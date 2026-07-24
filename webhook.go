package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
)

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"ok": false, "error": "POST only"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "read body"})
		return
	}
	secret := os.Getenv("RESEND_WEBHOOK_SECRET")
	if secret != "" {
		if !verifyResendWebhook(secret, r, body) {
			writeJSON(w, 401, map[string]any{"ok": false, "error": "invalid signature"})
			return
		}
	}
	var evt struct {
		Type string         `json:"type"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &evt); err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "bad json"})
		return
	}
	if evt.Type != "" && evt.Type != "email.received" {
		writeJSON(w, 200, map[string]any{"ok": true, "ignored": evt.Type})
		return
	}
	emailID := strField(evt.Data, "email_id")
	if emailID == "" {
		emailID = strField(evt.Data, "id")
	}
	if emailID == "" {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "missing email_id"})
		return
	}
	p := newPocheFromEnv()
	if p.Token == "" {
		writeJSON(w, 500, map[string]any{"ok": false, "error": "POCHE_TOKEN missing"})
		return
	}
	_ = ensureSchema(p)
	_ = ensureMailboxSchema(p)
	_ = ensureTags(p)
	re := newResendFromEnv()
	full, err := re.getReceiving(emailID)
	if err != nil {
		// metadata-only fallback from webhook payload
		full = evt.Data
		full["id"] = emailID
	}
	// route to the right mailbox by recipient address
	toAddr := emailField(full, "to")
	if toAddr == "" {
		toAddr = emailField(full, "received_for")
	}
	mbID, err := findOrCreateMailboxForAddress(p, toAddr)
	if err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": "mailbox lookup: " + err.Error()})
		return
	}
	if mbID == "" {
		writeJSON(w, 200, map[string]any{"ok": true, "data": map[string]any{"email_id": emailID, "created": false, "dropped": "no matching mailbox"}})
		return
	}
	// quota check
	msgSize := int64(len(strField(full, "text")) + len(strField(full, "html")))
	if err := mailboxAllowsIngest(p, mbID, msgSize); err != nil {
		writeJSON(w, 200, map[string]any{"ok": true, "data": map[string]any{"email_id": emailID, "created": false, "dropped": "quota: " + err.Error()}})
		return
	}
	created, err := upsertInbound(p, mbID, full)
	if err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "data": map[string]any{"email_id": emailID, "created": created}})
}

// verifyResendWebhook — lightweight HMAC check when secret is set.
// Full Svix multi-header verify can replace this later.
func verifyResendWebhook(secret string, r *http.Request, body []byte) bool {
	sig := r.Header.Get("resend-signature")
	if sig == "" {
		sig = r.Header.Get("svix-signature")
	}
	if sig == "" {
		return false
	}
	// accept raw hex hmac of body, or svix-style "v1,<hex>"
	parts := strings.Split(sig, ",")
	cand := sig
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(p, "v1=") {
			cand = strings.TrimPrefix(p, "v1=")
			break
		}
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(want), []byte(cand)) || strings.Contains(sig, want)
}
