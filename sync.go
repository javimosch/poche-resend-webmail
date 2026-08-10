package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// syncInbound pulls recent receiving emails into poche. limit=0 ⇒ all returned page.
func syncInbound(limit int) (int, error) {
	p := newPocheFromEnv()
	if p.Token == "" {
		return 0, fmt.Errorf("POCHE_TOKEN missing")
	}
	if err := ensureSchema(p); err != nil {
		return 0, err
	}
	if err := ensureMailboxSchema(p); err != nil {
		return 0, err
	}
	_ = ensureTags(p)

	// Each tenant may own a separate Resend account, so poll every distinct
	// key: the process-wide one plus any per-mailbox keys.
	accounts := resendAccounts(p)
	if len(accounts) == 0 {
		return 0, fmt.Errorf("no Resend credentials (set RESEND_API_KEY or a per-mailbox key)")
	}
	total := 0
	var lastErr error
	failures := 0
	for _, re := range accounts {
		n, err := syncFromAccount(p, re, limit)
		if err != nil {
			failures++
			lastErr = err
			fmt.Fprintf(os.Stderr, "{\"event\":\"sync_account_err\",\"key\":%q,\"err\":%q}\n", maskSecret(re.Key), err.Error())
			continue
		}
		total += n
	}
	if os.Getenv("AUTO_CLEANUP") == "1" {
		if _, cErr := cleanup(); cErr != nil {
			fmt.Fprintf(os.Stderr, "{\"event\":\"auto_cleanup_err\",\"err\":%q}\n", cErr.Error())
		}
	}
	if failures == len(accounts) {
		return 0, lastErr
	}
	return total, nil
}

// resendAccounts returns one client per distinct Resend key in play.
func resendAccounts(p *Poche) []*Resend {
	seen := map[string]bool{}
	out := []*Resend{}
	if env := newResendFromEnv(); env.enabled() {
		seen[env.Key] = true
		out = append(out, env)
	}
	mboxes, err := listAllMailboxes(p)
	if err != nil {
		return out
	}
	for i := range mboxes {
		key := mboxes[i].ResendAPIKey
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, resendForMailbox(&mboxes[i]))
	}
	return out
}

func syncFromAccount(p *Poche, re *Resend, limit int) (int, error) {
	items, err := re.listReceiving()
	if err != nil {
		return 0, err
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	n := 0
	dropped := 0
	for _, it := range items {
		id, _ := it["id"].(string)
		if id == "" {
			continue
		}
		full, err := re.getReceiving(id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "{\"event\":\"sync_skip\",\"id\":%q,\"err\":%q}\n", id, err.Error())
			continue
		}
		// route to the right mailbox by recipient address
		toAddr := emailField(full, "to")
		if toAddr == "" {
			toAddr = emailField(full, "received_for")
		}
		mbID, err := findOrCreateMailboxForAddress(p, toAddr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "{\"event\":\"sync_err\",\"id\":%q,\"err\":%q}\n", id, err.Error())
			continue
		}
		if mbID == "" {
			dropped++
			continue
		}
		// quota check
		msgSize := int64(len(strField(full, "text")) + len(strField(full, "html")))
		if err := mailboxAllowsIngest(p, mbID, msgSize); err != nil {
			fmt.Fprintf(os.Stderr, "{\"event\":\"sync_quota\",\"id\":%q,\"err\":%q}\n", id, err.Error())
			dropped++
			continue
		}
		created, err := upsertInbound(p, mbID, full)
		if err != nil {
			fmt.Fprintf(os.Stderr, "{\"event\":\"sync_err\",\"id\":%q,\"err\":%q}\n", id, err.Error())
			continue
		}
		if created {
			n++
		}
	}
	if dropped > 0 {
		fmt.Fprintf(os.Stderr, "{\"event\":\"sync_dropped\",\"count\":%d}\n", dropped)
	}
	return n, nil
}

func upsertInbound(p *Poche, mailboxID string, doc map[string]any) (created bool, err error) {
	resendID := strField(doc, "id")
	if resendID == "" {
		return false, fmt.Errorf("missing id")
	}
	existing, err := findByResendID(p, resendID)
	if err != nil {
		return false, err
	}
	if existing != "" {
		return false, nil
	}
	from := emailField(doc, "from")
	if from == "" {
		from = "unknown@resend.local"
	}
	to := emailField(doc, "to")
	if to == "" {
		to = emailField(doc, "received_for")
	}
	if to == "" {
		to = "inbox@local"
	}
	rf := emailField(doc, "received_for")
	if rf == "" {
		rf = to
	}
	subj := strField(doc, "subject")
	if subj == "" {
		subj = "(no subject)"
	}
	text := strField(doc, "text")
	// Sender-controlled markup is sanitized before it is ever stored, so no
	// render path has to trust it.
	html := sanitizeEmailHTML(strField(doc, "html"))
	mid := strField(doc, "message_id")
	// Thread continuity: Resend's inbound payload carries the raw email's
	// In-Reply-To/References headers, but until now nothing read them, so
	// every inbound message (including a reply-back to something WE sent)
	// got thread_id = its own message_id — never joining the conversation
	// it was actually part of. If the referenced message is one we already
	// have, adopt ITS thread_id (checking In-Reply-To first — the direct
	// parent — then References, most recent first, for the case where the
	// direct parent was never stored but an earlier ancestor was).
	inReplyTo := headerVal(doc, "in-reply-to")
	references := headerVal(doc, "references")
	threadID := mid
	candidates := extractMessageIDs(inReplyTo)
	refIDs := extractMessageIDs(references)
	for i := len(refIDs) - 1; i >= 0; i-- {
		candidates = append(candidates, refIDs[i])
	}
	for _, cand := range candidates {
		if tid, ok := findThreadIDByMessageID(p, mailboxID, cand); ok {
			threadID = tid
			break
		}
	}
	preview := text
	if len(preview) > 160 {
		preview = preview[:160]
	}
	if preview == "" && html != "" {
		preview = stripTags(html)
		if len(preview) > 160 {
			preview = preview[:160]
		}
	}
	search := strings.ToLower(subj + " " + from + " " + to + " " + text)
	createdAt := time.Now().UnixMilli()
	msg := map[string]any{
		"mailbox_id":     mailboxID,
		"from_addr":      from,
		"to_addr":        to,
		"subject":        subj,
		"preview":        preview,
		"body_text":      text,
		"body_html":      html,
		"html_sanitized": true,
		"search_text":    search,
		"thread_id":      threadID,
		"unread":         true,
		"starred":        false,
		"resend_id":      resendID,
		"message_id":     mid,
		"received_for":   rf,
		"direction":      "in",
		"in_reply_to":    inReplyTo,
		"references":     references,
		"created_at":     createdAt,
	}
	raw, err := p.Create("messages", msg)
	if err != nil {
		return false, err
	}
	updateMailboxUsage(p, mailboxID, 1, messageSizeBytes(msg))
	var wrap map[string]any
	_ = json.Unmarshal(raw, &wrap)
	localID, _ := wrap["_id"].(string)
	_ = upsertAttachments(p, localID, doc)
	return true, nil
}

func findByResendID(p *Poche, resendID string) (string, error) {
	data, err := p.List("messages", "resend_id="+resendID, 1, 0, "", false)
	if err != nil {
		return "", err
	}
	var page struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		return "", err
	}
	if page.Total == 0 || len(page.Items) == 0 {
		return "", nil
	}
	return page.Items[0].ID, nil
}

// upsertAttachments stores attachment metadata, and — when machin-esetres is
// configured — the actual content. Resend's attachment objects carry no
// per-attachment download link (verified against a real message, not
// assumed: see mime_extract.go's header comment); what's fetchable is
// doc["raw"]["download_url"], the entire raw email, which this downloads
// ONCE per message and extracts every attachment from, rather than
// re-fetching per attachment.
func upsertAttachments(p *Poche, messageID string, doc map[string]any) error {
	rawAttachments, ok := doc["attachments"]
	if !ok || messageID == "" {
		return nil
	}
	arr, ok := rawAttachments.([]any)
	if !ok || len(arr) == 0 {
		return nil
	}

	rawURL := ""
	if rawObj, ok := doc["raw"].(map[string]any); ok {
		rawURL = strField(rawObj, "download_url")
	}
	var rawEmail []byte
	if esetresEnabled() && rawURL != "" {
		var err error
		rawEmail, err = fetchRawEmail(rawURL)
		if err != nil {
			fmt.Fprintf(os.Stderr, "{\"event\":\"attachment_raw_fetch_err\",\"message_id\":%q,\"err\":%q}\n", messageID, err.Error())
		}
	}

	for _, a := range arr {
		m, ok := a.(map[string]any)
		if !ok {
			continue
		}
		aid := strField(m, "id")
		filename := strField(m, "filename")
		contentType := strField(m, "content_type")
		esetresKey := ""
		stored := false

		if len(rawEmail) > 0 && filename != "" {
			data, err := extractAttachmentFromRawEmail(rawEmail, filename)
			if err != nil {
				fmt.Fprintf(os.Stderr, "{\"event\":\"attachment_extract_err\",\"message_id\":%q,\"filename\":%q,\"err\":%q}\n", messageID, filename, err.Error())
			} else {
				key := messageID + "/" + aid + "/" + safeFilename(filename)
				if err := esetresPut(key, data, contentType); err != nil {
					fmt.Fprintf(os.Stderr, "{\"event\":\"attachment_esetres_put_err\",\"message_id\":%q,\"filename\":%q,\"err\":%q}\n", messageID, filename, err.Error())
				} else {
					esetresKey = key
					stored = true
				}
			}
		}

		_, _ = p.Create("attachments", map[string]any{
			"message_id":       messageID,
			"filename":         filename,
			"content_type":     contentType,
			"resend_attach_id": aid,
			"download_url":     "",
			"file_id":          "",
			"esetres_key":      esetresKey,
			"bytes":            m["size"],
			"stored":           stored,
		})
	}
	return nil
}

func strField(m map[string]any, k string) string {
	v, _ := m[k].(string)
	return v
}

// headerVal reads a raw email header from Resend's inbound payload
// (doc["headers"] is a flat map of lowercased header names to string
// values — confirmed against a real captured payload, not assumed).
func headerVal(doc map[string]any, key string) string {
	h, _ := doc["headers"].(map[string]any)
	if h == nil {
		return ""
	}
	v, _ := h[key].(string)
	return v
}

// extractMessageIDs pulls every "<...>"-wrapped Message-ID out of a raw
// In-Reply-To or References header value. RFC 5322 says these are
// space-separated, but scanning for "<...>" pairs directly (rather than
// splitting on whitespace) also copes with servers that comma-join them or
// omit separators entirely — both seen in the wild.
func extractMessageIDs(s string) []string {
	var out []string
	for {
		start := strings.IndexByte(s, '<')
		if start < 0 {
			break
		}
		end := strings.IndexByte(s[start:], '>')
		if end < 0 {
			break
		}
		out = append(out, s[start:start+end+1])
		s = s[start+end+1:]
	}
	return out
}

// findThreadIDByMessageID looks up the thread_id of an existing message by
// its RFC822 message_id, scoped to a mailbox (a reply should only ever join
// a thread already owned by the same mailbox). Returns ok=false if nothing
// matches — the caller then starts a new thread rather than guessing.
func findThreadIDByMessageID(p *Poche, mailboxID, msgID string) (string, bool) {
	if msgID == "" {
		return "", false
	}
	// A "<...>"-wrapped Message-ID breaks poche's `=` equality matching
	// entirely (confirmed against a real instance — 0 results even for a
	// literal exact value, no other special chars involved). `~=` (substring)
	// works fine with the same characters, so this uses that plus an exact
	// check in Go once the candidates are back, rather than trusting the
	// substring match alone.
	data, err := p.List("messages", "mailbox_id="+mailboxID+",message_id~="+msgID, 5, 0, "", false)
	if err != nil {
		return "", false
	}
	var page struct {
		Items []struct {
			Doc json.RawMessage `json:"doc"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		return "", false
	}
	for _, it := range page.Items {
		doc := map[string]any{}
		_ = json.Unmarshal(it.Doc, &doc)
		if strField(doc, "message_id") != msgID {
			continue
		}
		tid := strField(doc, "thread_id")
		if tid == "" {
			tid = msgID
		}
		return tid, true
	}
	return "", false
}

// emailFromValue extracts an email address from Resend's object/array format:
// string, {"email":"..."}, or [{"email":"..."}]. Also strips "Name <email>".
func emailFromValue(v any) string {
	switch t := v.(type) {
	case string:
		return emailOf(t)
	case map[string]any:
		if e, ok := t["email"].(string); ok {
			return emailOf(e)
		}
	case []any:
		if len(t) > 0 {
			return emailFromValue(t[0])
		}
	}
	return ""
}

func emailField(m map[string]any, k string) string {
	return emailFromValue(m[k])
}

func firstString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		if len(t) > 0 {
			if s, ok := t[0].(string); ok {
				return s
			}
		}
	}
	return ""
}

func stripTags(s string) string {
	out := strings.Builder{}
	in := false
	for _, r := range s {
		if r == '<' {
			in = true
			continue
		}
		if r == '>' {
			in = false
			continue
		}
		if !in {
			out.WriteRune(r)
		}
	}
	return strings.TrimSpace(out.String())
}
