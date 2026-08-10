package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

func handleReplyAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"ok": false, "error": "POST only"})
		return
	}
	var req struct {
		ID      string `json:"id"`
		Text    string `json:"text"`
		From    string `json:"from"`
		To      string `json:"to"`
		Subject string `json:"subject"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" || req.Text == "" {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "id and text required"})
		return
	}
	// A message id alone must not let one tenant reply as another: without
	// this, replyMessage derives its Resend credentials purely from the
	// TARGET message's own mailbox, so any signed-in tenant could send mail
	// through a different tenant's Resend account just by supplying its
	// message id. Same class of gap as handleAttachmentOpen already guards.
	if !authIsAdmin(r) {
		p := newPocheFromEnv()
		doc, err := loadDoc(p, req.ID)
		if err != nil {
			writeJSON(w, 404, map[string]any{"ok": false, "error": "message not found"})
			return
		}
		mbID := authMailboxID(r)
		if mbID == "" || strField(doc, "mailbox_id") != mbID {
			writeJSON(w, 403, map[string]any{"ok": false, "error": "forbidden"})
			return
		}
	}
	data, err := replyMessage(req.ID, req.Text, req.From, req.To, req.Subject)
	if err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "data": data})
}

func replyMessage(localID, text, fromOverride, toOverride, subjOverride string) (map[string]any, error) {
	p := newPocheFromEnv()
	doc, err := loadDoc(p, localID)
	if err != nil {
		return nil, err
	}
	from := fromOverride
	if from == "" {
		from, _ = doc["received_for"].(string)
	}
	if from == "" {
		from, _ = doc["to_addr"].(string)
	}
	// If the derived from address is missing, disallowed by domain allowlist, or
	// does not belong to the mailbox (e.g. stale 'inbox@local' from old payloads),
	// fall back to the mailbox primary address so replies can still be sent.
	mbID, _ := doc["mailbox_id"].(string)
	pForAddr := newPocheFromEnv()
	var mb *mailboxRecord
	if mbID != "" {
		mb, _ = getMailboxByID(pForAddr, mbID)
	}
	isMailboxAddr := mb != nil && mailboxOwnsAddress(pForAddr, mb, from)
	if from == "" || !fromAllowed(from) || (mb != nil && !isMailboxAddr) {
		if mb != nil && fromAllowed(mb.Address) {
			from = mb.Address
		}
	}
	if from == "" || !fromAllowed(from) {
		return nil, fmt.Errorf("from not allowed (set MAIL_FROM_ALLOWLIST): %s", from)
	}
	if mb != nil && !mailboxOwnsAddress(pForAddr, mb, from) {
		return nil, fmt.Errorf("from address %s does not belong to this mailbox", from)
	}
	to := toOverride
	if to == "" {
		to = emailOf(fmt.Sprint(doc["from_addr"]))
	}
	if to == "" {
		return nil, fmt.Errorf("no recipient")
	}
	subj := subjOverride
	if subj == "" {
		subj = reSubject(fmt.Sprint(doc["subject"]))
	}
	mid, _ := doc["message_id"].(string)
	payload := map[string]any{
		"from":    from,
		"to":      []string{to},
		"subject": subj,
		"text":    text,
	}
	if mid != "" {
		payload["headers"] = map[string]string{
			"In-Reply-To": mid,
			"References":  mid,
		}
	}
	re := resendForMailbox(mb)
	sent, err := re.sendEmail(payload)
	if err != nil {
		return nil, err
	}
	sentID, _ := sent["id"].(string)
	sentMbID, _ := doc["mailbox_id"].(string)
	outDoc := map[string]any{
		"mailbox_id":   sentMbID,
		"from_addr":    from,
		"to_addr":      to,
		"subject":      subj,
		"preview":      text,
		"body_text":    text,
		"body_html":    "",
		"search_text":  strings.ToLower(subj + " " + from + " " + to + " " + text),
		"thread_id":    mid,
		"unread":       false,
		"starred":      false,
		"resend_id":    sentID,
		"message_id":   "",
		"received_for": from,
		"direction":    "out",
		"in_reply_to":  mid,
		"references":   mid,
		"created_at":   time.Now().UnixMilli(),
	}
	_, err = p.Create("messages", outDoc)
	if err == nil {
		updateMailboxUsage(p, mbID, 1, messageSizeBytes(outDoc))
	}
	return map[string]any{
		"sent_id": sentID,
		"from":    from,
		"to":      to,
		"subject": subj,
	}, nil
}

func reSubject(subj string) string {
	l := strings.ToLower(strings.TrimSpace(subj))
	if strings.HasPrefix(l, "re:") {
		return subj
	}
	return "Re: " + subj
}
