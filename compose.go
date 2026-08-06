package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// composeReq is the payload for a brand-new outbound email (not a reply).
// to/cc/bcc accept either a JSON array of strings or a comma-separated string.
type composeReq struct {
	From    string `json:"from"`
	To      any    `json:"to"`
	Cc      any    `json:"cc"`
	Bcc     any    `json:"bcc"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
	// "text" (default), "html", or "markdown" — markdown is converted and
	// sanitized server-side so every caller gets the same output.
	Format string `json:"format"`
}

func handleComposeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"ok": false, "error": "POST only"})
		return
	}
	var req composeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "bad json"})
		return
	}
	data, err := composeMessage(authMailboxID(r), authIsAdmin(r), req)
	if err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "data": data})
}

// composeMessage sends a new email through Resend and stores it as an
// outbound message on the sender's mailbox. mbID is the session mailbox
// ("" for admin tokens, which must then supply a From we can resolve).
func composeMessage(mbID string, isAdmin bool, req composeReq) (map[string]any, error) {
	to := parseAddrList(req.To)
	cc := parseAddrList(req.Cc)
	bcc := parseAddrList(req.Bcc)
	if len(to) == 0 {
		return nil, fmt.Errorf("at least one recipient is required")
	}
	for _, a := range append(append(append([]string{}, to...), cc...), bcc...) {
		if !looksLikeEmail(a) {
			return nil, fmt.Errorf("invalid recipient: %s", a)
		}
	}
	subject := strings.TrimSpace(req.Subject)
	if subject == "" {
		return nil, fmt.Errorf("subject is required")
	}
	if strings.TrimSpace(req.Text) == "" {
		return nil, fmt.Errorf("body is required")
	}

	p := newPocheFromEnv()
	if p.Token == "" {
		return nil, fmt.Errorf("POCHE_TOKEN missing")
	}

	from := emailOf(strings.TrimSpace(req.From))
	var mb *mailboxRecord
	var err error
	if mbID != "" {
		mb, err = getMailboxByID(p, mbID)
		if err != nil || mb == nil {
			return nil, fmt.Errorf("mailbox not found")
		}
	} else if isAdmin {
		// Admin tokens have no mailbox context — derive it from the From address
		// so the sent copy still lands in the right mailbox.
		if from == "" {
			return nil, fmt.Errorf("from is required for admin sends")
		}
		if mb, err = findMailboxByAddress(p, from); err != nil || mb == nil {
			mb, _ = findMailboxByAlias(p, from)
		}
		if mb != nil {
			mbID = mb.ID
		}
	} else {
		return nil, fmt.Errorf("no mailbox context")
	}

	// Default the sender to the mailbox primary address, then verify the
	// chosen address actually belongs to this mailbox (primary or alias).
	if mb != nil {
		addrs := mailboxAllAddresses(p, mb)
		if from == "" {
			from = mb.Address
		}
		owned := false
		for _, a := range addrs {
			if strings.EqualFold(a, from) {
				owned = true
				break
			}
		}
		if !owned {
			return nil, fmt.Errorf("from address %s does not belong to this mailbox", from)
		}
		if !mb.IsActive {
			return nil, fmt.Errorf("mailbox suspended")
		}
	}
	if from == "" {
		return nil, fmt.Errorf("no from address")
	}
	if !fromAllowed(from) {
		return nil, fmt.Errorf("from not allowed (set MAIL_FROM_ALLOWLIST): %s", from)
	}

	textPart, htmlPart, err := renderBody(req.Text, req.Format)
	if err != nil {
		return nil, fmt.Errorf("render %s body: %w", normalizeFormat(req.Format), err)
	}
	if strings.TrimSpace(stripTags(htmlPart)) == "" && strings.TrimSpace(textPart) == "" {
		return nil, fmt.Errorf("body is empty after rendering")
	}
	payload := map[string]any{
		"from":    from,
		"to":      to,
		"subject": subject,
		"text":    textPart,
	}
	if htmlPart != "" {
		payload["html"] = htmlPart
	}
	if len(cc) > 0 {
		payload["cc"] = cc
	}
	if len(bcc) > 0 {
		payload["bcc"] = bcc
	}

	re := resendForMailbox(mb)
	sent, err := re.sendEmail(payload)
	if err != nil {
		return nil, err
	}
	sentID, _ := sent["id"].(string)

	toLine := strings.Join(to, ", ")
	outDoc := map[string]any{
		"mailbox_id":     mbID,
		"from_addr":      from,
		"to_addr":        toLine,
		"cc_addr":        strings.Join(cc, ", "),
		"subject":        subject,
		"preview":        truncate(textPart, 200),
		"body_text":      textPart,
		"body_html":      htmlPart,
		"html_sanitized": true,
		"search_text":    strings.ToLower(subject + " " + from + " " + toLine + " " + textPart),
		"thread_id":      "",
		"unread":         false,
		"starred":        false,
		"resend_id":      sentID,
		"message_id":     "",
		"received_for":   from,
		"direction":      "out",
		"in_reply_to":    "",
		"references":     "",
		"created_at":     time.Now().UnixMilli(),
	}
	if _, err := p.Create("messages", outDoc); err != nil {
		// The mail is already out the door — report the send as successful but
		// surface the storage failure so the Sent copy isn't silently missing.
		fmt.Fprintf(os.Stderr, "{\"event\":\"compose_store_err\",\"resend_id\":%q,\"err\":%q}\n", sentID, err.Error())
		return map[string]any{
			"sent_id": sentID, "from": from, "to": to, "subject": subject,
			"stored": false, "store_error": err.Error(),
		}, nil
	}
	if mbID != "" {
		updateMailboxUsage(p, mbID, 1, messageSizeBytes(outDoc))
	}
	return map[string]any{
		"sent_id": sentID,
		"from":    from,
		"to":      to,
		"cc":      cc,
		"subject": subject,
		"format":  normalizeFormat(req.Format),
		"html":    htmlPart != "",
		"stored":  true,
	}, nil
}

// handleMailboxAddressesAPI lists the addresses the caller may send as, so the
// UI can offer a From selector (primary address first, then aliases).
func handleMailboxAddressesAPI(w http.ResponseWriter, r *http.Request) {
	p := newPocheFromEnv()
	if p.Token == "" {
		writeJSON(w, 500, map[string]any{"ok": false, "error": "POCHE_TOKEN missing"})
		return
	}
	mbID := authMailboxID(r)
	if mbID == "" {
		if !authIsAdmin(r) {
			writeJSON(w, 400, map[string]any{"ok": false, "error": "no mailbox context"})
			return
		}
		// Admin: every address across every mailbox.
		mboxes, _ := listAllMailboxes(p)
		all := []string{}
		for i := range mboxes {
			all = append(all, mailboxAllAddresses(p, &mboxes[i])...)
		}
		writeJSON(w, 200, map[string]any{"ok": true, "data": map[string]any{"addresses": all}})
		return
	}
	mb, err := getMailboxByID(p, mbID)
	if err != nil || mb == nil {
		writeJSON(w, 404, map[string]any{"ok": false, "error": "mailbox not found"})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "data": map[string]any{
		"addresses": mailboxAllAddresses(p, mb),
		"primary":   mb.Address,
	}})
}

// parseAddrList accepts []string, a comma/semicolon separated string, or nil.
func parseAddrList(v any) []string {
	out := []string{}
	switch t := v.(type) {
	case nil:
		return out
	case string:
		for _, part := range strings.FieldsFunc(t, func(r rune) bool { return r == ',' || r == ';' }) {
			if a := emailOf(strings.TrimSpace(part)); a != "" {
				out = append(out, a)
			}
		}
	case []any:
		for _, item := range t {
			s, ok := item.(string)
			if !ok {
				continue
			}
			if a := emailOf(strings.TrimSpace(s)); a != "" {
				out = append(out, a)
			}
		}
	}
	return out
}

func looksLikeEmail(a string) bool {
	at := strings.Index(a, "@")
	if at <= 0 || at == len(a)-1 {
		return false
	}
	if strings.Count(a, "@") != 1 {
		return false
	}
	domain := a[at+1:]
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return false
	}
	return !strings.ContainsAny(a, " \t\r\n<>,;")
}

// ─── CLI ────────────────────────────────────────────────────────────────

func handleComposeCmd() {
	fs := newFlagSet("send")
	to := fs.String("to", "", "recipient(s), comma-separated")
	cc := fs.String("cc", "", "cc recipient(s), comma-separated")
	bcc := fs.String("bcc", "", "bcc recipient(s), comma-separated")
	from := fs.String("from", "", "sender address (defaults to mailbox primary)")
	subject := fs.String("subject", "", "subject")
	text := fs.String("text", "", "body text")
	format := fs.String("format", "text", "body format: text|html|markdown")
	_ = fs.Parse(os.Args[2:])
	if *to == "" || *subject == "" || *text == "" {
		fail(80, "input", "usage: send --to a@b.fr --subject S --text T [--from x] [--cc] [--bcc]", "")
	}
	data, err := composeMessage("", true, composeReq{
		From: *from, To: *to, Cc: *cc, Bcc: *bcc, Subject: *subject, Text: *text, Format: *format,
	})
	if err != nil {
		fail(100, "integration", err.Error(), "set RESEND_API_KEY, POCHE_TOKEN and MAIL_FROM_ALLOWLIST")
	}
	outOK(data)
}

// handleRenderAPI previews a body exactly as it would be sent: same Markdown
// converter, same sanitizer. A preview rendered separately in the browser
// would be a different pipeline, and would lie about what actually goes out.
func handleRenderAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"ok": false, "error": "POST only"})
		return
	}
	var req struct {
		Text   string `json:"text"`
		Format string `json:"format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "bad json"})
		return
	}
	text, html, err := renderBody(req.Text, req.Format)
	if err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "data": map[string]any{
		"format": normalizeFormat(req.Format),
		"html":   html,
		"text":   text,
	}})
}
