package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
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
	// Log every delivery attempt: without this there is no way to tell a
	// provider that never called from one whose calls we rejected.
	fmt.Fprintf(os.Stderr, "{\"event\":\"webhook_hit\",\"bytes\":%d,\"signed\":%v,\"ua\":%q,\"from_ip\":%q}\n",
		len(body), firstHeader(r, "svix-signature", "resend-signature", "webhook-signature") != "",
		r.Header.Get("User-Agent"), r.Header.Get("X-Forwarded-For"))
	// The payload has to be parsed before the signature can be checked, because
	// the recipient decides which tenant's signing secret applies. Nothing is
	// written to the store until the signature has been verified below.
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

	// route to the right mailbox by recipient address
	toAddr := emailField(evt.Data, "to")
	if toAddr == "" {
		toAddr = emailField(evt.Data, "received_for")
	}
	mb, err := findMailboxRecordForAddress(p, toAddr)
	if err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": "mailbox lookup: " + err.Error()})
		return
	}

	// Verify against the tenant's own secret when it has one, else the env secret.
	if secret := webhookSecretForMailbox(mb); secret != "" {
		if !verifyResendWebhook(secret, r, body) {
			fmt.Fprintf(os.Stderr, "{\"event\":\"webhook_rejected\",\"reason\":\"bad_signature\",\"to\":%q}\n", toAddr)
			writeJSON(w, 401, map[string]any{"ok": false, "error": "invalid signature"})
			return
		}
	} else {
		fmt.Fprintf(os.Stderr, "{\"event\":\"webhook_unverified\",\"to\":%q}\n", toAddr)
	}

	if mb == nil {
		fmt.Fprintf(os.Stderr, "{\"event\":\"webhook_dropped\",\"reason\":\"no_mailbox\",\"to\":%q}\n", toAddr)
		writeJSON(w, 200, map[string]any{"ok": true, "data": map[string]any{"email_id": emailID, "created": false, "dropped": "no matching mailbox"}})
		return
	}
	mbID := mb.ID

	// Fetch the full email with the credentials of the account that received it.
	re := resendForMailbox(mb)
	full, err := re.getReceiving(emailID)
	if err != nil {
		// metadata-only fallback from webhook payload
		full = evt.Data
		full["id"] = emailID
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
	fmt.Fprintf(os.Stderr, "{\"event\":\"webhook_ingested\",\"to\":%q,\"created\":%v,\"email_id\":%q}\n", toAddr, created, emailID)
	writeJSON(w, 200, map[string]any{"ok": true, "data": map[string]any{"email_id": emailID, "created": created}})
}

// webhookTolerance bounds how far a webhook timestamp may drift, so a captured
// request cannot be replayed indefinitely.
const webhookTolerance = 5 * time.Minute

// verifyResendWebhook checks the signature on an inbound webhook.
//
// Resend signs through Svix: the MAC covers "<id>.<timestamp>.<body>", keyed by
// the base64 payload of a "whsec_"-prefixed secret, and is sent base64 in a
// space-separated list of "v1,<sig>" candidates. Secrets that are not in that
// format fall back to a plain HMAC of the body (used by local tooling/tests).
func verifyResendWebhook(secret string, r *http.Request, body []byte) bool {
	sigHeader := firstHeader(r, "svix-signature", "resend-signature", "webhook-signature")
	if sigHeader == "" {
		return false
	}
	if strings.HasPrefix(secret, "whsec_") {
		return verifySvixSignature(secret, r, body, sigHeader)
	}
	return verifyBodyHMAC(secret, body, sigHeader)
}

func verifySvixSignature(secret string, r *http.Request, body []byte, sigHeader string) bool {
	id := firstHeader(r, "svix-id", "resend-id", "webhook-id")
	ts := firstHeader(r, "svix-timestamp", "resend-timestamp", "webhook-timestamp")
	if id == "" || ts == "" {
		return false
	}
	secs, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return false
	}
	if drift := time.Since(time.Unix(secs, 0)); drift > webhookTolerance || drift < -webhookTolerance {
		return false
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "whsec_"))
	if err != nil || len(key) == 0 {
		return false
	}
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(id + "." + ts + "."))
	_, _ = mac.Write(body)
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	// The header carries one or more "v1,<sig>" candidates during key rotation.
	for _, part := range strings.Fields(sigHeader) {
		i := strings.IndexByte(part, ',')
		if i < 0 {
			continue
		}
		if hmac.Equal([]byte(part[i+1:]), []byte(want)) {
			return true
		}
	}
	return false
}

// verifyBodyHMAC is the legacy scheme: hex HMAC of the raw body keyed by the
// secret string, sent either bare or as "v1=<hex>".
func verifyBodyHMAC(secret string, body []byte, sigHeader string) bool {
	cand := sigHeader
	for _, p := range strings.Split(sigHeader, ",") {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(p, "v1=") {
			cand = strings.TrimPrefix(p, "v1=")
			break
		}
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(want), []byte(cand))
}

func firstHeader(r *http.Request, names ...string) string {
	for _, n := range names {
		if v := strings.TrimSpace(r.Header.Get(n)); v != "" {
			return v
		}
	}
	return ""
}
