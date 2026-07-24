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
	re := newResendFromEnv()
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
	if os.Getenv("AUTO_CLEANUP") == "1" {
		if _, cErr := cleanup(); cErr != nil {
			fmt.Fprintf(os.Stderr, "{\"event\":\"auto_cleanup_err\",\"err\":%q}\n", cErr.Error())
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
	html := strField(doc, "html")
	mid := strField(doc, "message_id")
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
		"mailbox_id":    mailboxID,
		"from_addr":     from,
		"to_addr":       to,
		"subject":       subj,
		"preview":       preview,
		"body_text":     text,
		"body_html":     html,
		"search_text":   search,
		"thread_id":     mid,
		"unread":        true,
		"starred":       false,
		"resend_id":     resendID,
		"message_id":    mid,
		"received_for":  rf,
		"direction":     "in",
		"in_reply_to":   "",
		"references":    "",
		"created_at":    createdAt,
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

func upsertAttachments(p *Poche, messageID string, doc map[string]any) error {
	raw, ok := doc["attachments"]
	if !ok || messageID == "" {
		return nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil
	}
	for _, a := range arr {
		m, ok := a.(map[string]any)
		if !ok {
			continue
		}
		aid := strField(m, "id")
		_, _ = p.Create("attachments", map[string]any{
			"message_id":       messageID,
			"filename":         strField(m, "filename"),
			"content_type":     strField(m, "content_type"),
			"resend_attach_id": aid,
			"download_url":     strField(m, "download_url"),
			"file_id":          "",
		})
	}
	return nil
}

func strField(m map[string]any, k string) string {
	v, _ := m[k].(string)
	return v
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
