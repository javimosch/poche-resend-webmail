package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
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
	// Bytes are forwarded to Resend and never persisted; see below.
	Attachments []composeAttachment `json:"attachments"`
}

func handleComposeAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"ok": false, "error": "POST only"})
		return
	}
	// base64 inflates by ~4/3, plus JSON overhead — cap the request rather than
	// letting an oversized upload buffer into memory before validation runs.
	_, totalCap, _ := attachmentLimits()
	r.Body = http.MaxBytesReader(w, r.Body, totalCap*2+1<<20)
	var req composeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 413, map[string]any{"ok": false, "error": "request too large or malformed"})
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
		if from == "" {
			from = mb.Address
		}
		if !mailboxOwnsAddress(p, mb, from) {
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

	files, fileMeta, err := prepareAttachments(req.Attachments)
	if err != nil {
		return nil, err
	}
	if len(files) > 0 {
		payload["attachments"] = files
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
	stored, err := p.Create("messages", outDoc)
	var attachmentBytes int64
	if err == nil && len(fileMeta) > 0 {
		var wrap map[string]any
		_ = json.Unmarshal(stored, &wrap)
		if id, _ := wrap["_id"].(string); id != "" {
			attachmentBytes = recordSentAttachments(p, mbID, id, fileMeta)
			fmt.Fprintf(os.Stderr, "{\"event\":\"attachments_stored\",\"files\":%d,\"bytes\":%d,\"mailbox\":%q}\n",
				len(fileMeta), attachmentBytes, mbID)
		}
	}
	if err != nil {
		// The mail is already out the door — report the send as successful but
		// surface the storage failure so the Sent copy isn't silently missing.
		fmt.Fprintf(os.Stderr, "{\"event\":\"compose_store_err\",\"resend_id\":%q,\"err\":%q}\n", sentID, err.Error())
		return map[string]any{
			"sent_id": sentID, "from": from, "to": to, "subject": subject,
			"stored": false, "store_error": err.Error(),
		}, nil
	}
	if mbID != "" {
		// One write, not two: poche's read-after-write is not immediate, so a
		// second update moments later reads a stale counter and clobbers the
		// first. Attachments count against the quota like message bodies.
		updateMailboxUsage(p, mbID, 1, messageSizeBytes(outDoc)+attachmentBytes)
	}
	return map[string]any{
		"sent_id":     sentID,
		"from":        from,
		"to":          to,
		"cc":          cc,
		"subject":     subject,
		"format":      normalizeFormat(req.Format),
		"html":        htmlPart != "",
		"attachments": len(files),
		"stored":      true,
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
	resp := map[string]any{
		"addresses": mailboxAllAddresses(p, mb),
		"primary":   mb.Address,
	}
	// A catch-all mailbox (see mailboxOwnsAddress) may send/reply as ANY
	// address at its domain, not just its own primary+aliases — that set is
	// unbounded, so the UI can't offer a fixed dropdown of it. Tell it the
	// domain (so it can offer free-text entry) plus, as a convenience, the
	// addresses this mailbox has actually received mail at so far — a
	// starting point, not the full set of what's allowed.
	if mb.CatchallDomain != "" {
		resp["catchall_domain"] = mb.CatchallDomain
		if seen, _, err := messageFieldFacets(p, mb.ID, "to_addr"); err == nil {
			addrs := make([]string, 0, len(seen))
			for _, f := range seen {
				addrs = append(addrs, f.Value)
			}
			resp["seen_addresses"] = addrs
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true, "data": resp})
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
	attach := fs.String("attach", "", "comma-separated file paths to attach (streamed to Resend, not stored)")
	_ = fs.Parse(os.Args[2:])
	if *to == "" || *subject == "" || *text == "" {
		fail(80, "input", "usage: send --to a@b.fr --subject S --text T [--from x] [--cc] [--bcc]", "")
	}
	var atts []composeAttachment
	for _, path := range splitCSV(*attach) {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			fail(80, "input", "--attach: "+err.Error(), "")
		}
		atts = append(atts, composeAttachment{
			Filename:    filepath.Base(path),
			ContentType: mime.TypeByExtension(filepath.Ext(path)),
			Content:     base64.StdEncoding.EncodeToString(raw),
		})
	}
	data, err := composeMessage("", true, composeReq{
		From: *from, To: *to, Cc: *cc, Bcc: *bcc, Subject: *subject, Text: *text, Format: *format,
		Attachments: atts,
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

// ─── attachments (send-only) ────────────────────────────────────────────
//
// Attachment bytes are streamed straight through to Resend and never stored:
// poche keeps documents in memory (6 MB of blobs took the store from 4 MB to
// 315 MB RSS) and dk1 has little disk to spare. The Sent copy therefore keeps
// the file names and sizes, but not the contents.

// storedAttachment carries the decoded bytes so the Sent copy can keep them.
type storedAttachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

type composeAttachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Content     string `json:"content"` // base64, as Resend expects
}

func attachmentLimits() (perFile int64, total int64, count int) {
	return envInt64("ATTACHMENT_MAX_BYTES", 10*1024*1024),
		envInt64("ATTACHMENTS_MAX_TOTAL_BYTES", 20*1024*1024),
		envInt("ATTACHMENTS_MAX_COUNT", 10)
}

// safeFilename strips any path and control characters a sender could use to
// escape a download directory or spoof an extension.
func safeFilename(name string) string {
	name = strings.TrimSpace(name)
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, name)
	name = strings.TrimLeft(name, ".")
	if name == "" {
		name = "attachment"
	}
	if len(name) > 200 {
		name = name[:200]
	}
	return name
}

// prepareAttachments validates the payload and returns the Resend-shaped list
// plus a metadata summary to store on the sent message.
func prepareAttachments(in []composeAttachment) ([]map[string]any, []storedAttachment, error) {
	perFile, totalCap, maxCount := attachmentLimits()
	if len(in) == 0 {
		return nil, nil, nil
	}
	if len(in) > maxCount {
		return nil, nil, fmt.Errorf("too many attachments: %d (max %d)", len(in), maxCount)
	}
	out := make([]map[string]any, 0, len(in))
	meta := make([]storedAttachment, 0, len(in))
	var running int64
	for _, a := range in {
		name := safeFilename(a.Filename)
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(a.Content))
		if err != nil {
			return nil, nil, fmt.Errorf("%s: content must be base64", name)
		}
		size := int64(len(raw))
		if size == 0 {
			return nil, nil, fmt.Errorf("%s: empty file", name)
		}
		if size > perFile {
			return nil, nil, fmt.Errorf("%s is %s, over the %s per-file limit", name, humanBytes(size), humanBytes(perFile))
		}
		running += size
		if running > totalCap {
			return nil, nil, fmt.Errorf("attachments total %s, over the %s limit", humanBytes(running), humanBytes(totalCap))
		}
		att := map[string]any{
			"filename": name,
			"content":  base64.StdEncoding.EncodeToString(raw),
		}
		if ct := strings.TrimSpace(a.ContentType); ct != "" {
			att["content_type"] = ct
		}
		out = append(out, att)
		meta = append(meta, storedAttachment{Filename: name, ContentType: a.ContentType, Data: raw})
	}
	return out, meta, nil
}

func humanBytes(b int64) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// recordSentAttachments keeps the bytes so a sent message can be re-opened.
// Storage failures are logged and recorded as stored:false rather than failing
// the send — the mail is already delivered by this point.
func recordSentAttachments(p *Poche, mailboxID, messageID string, atts []storedAttachment) int64 {
	var kept int64
	for _, a := range atts {
		blobID, err := putBlob(a.Data)
		if err != nil {
			fmt.Fprintf(os.Stderr, "{\"event\":\"attachment_store_err\",\"file\":%q,\"err\":%q}\n", a.Filename, err.Error())
			blobID = ""
		}
		doc := map[string]any{
			"message_id":       messageID,
			"filename":         a.Filename,
			"content_type":     a.ContentType,
			"resend_attach_id": "",
			"download_url":     "",
			"file_id":          blobID,
			"bytes":            len(a.Data),
			"stored":           blobID != "",
		}
		if _, err := p.Create("attachments", doc); err != nil {
			fmt.Fprintf(os.Stderr, "{\"event\":\"sent_attachment_meta_err\",\"err\":%q}\n", err.Error())
			if blobID != "" {
				deleteBlob(blobID) // no row to find it by — do not orphan it
			}
			continue
		}
		if blobID != "" {
			kept += int64(len(a.Data))
		}
	}
	return kept
}
