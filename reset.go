package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ─── reset tokens ───────────────────────────────────────────────────────

// reset_tokens collection: single-use tokens for password reset.
const resetTokensSchema = "token:string!required!unique,mailbox_id:string!required!ref=mailboxes,expires_at:int"

func ensureResetSchema(p *Poche) error {
	if err := p.AdminSchema("reset_tokens", resetTokensSchema); err != nil {
		return fmt.Errorf("reset_tokens: %w", err)
	}
	if err := p.AdminExpose("reset_tokens", "read,create,delete"); err != nil {
		return err
	}
	_ = p.AdminIndex("reset_tokens", "token", "")
	return nil
}

func createResetToken(p *Poche, mailboxID string) (string, error) {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)
	expires := time.Now().Add(1 * time.Hour).UnixMilli()
	_, err := p.Create("reset_tokens", map[string]any{
		"token":      token,
		"mailbox_id": mailboxID,
		"expires_at": expires,
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

func lookupResetToken(p *Poche, token string) (string, error) {
	data, err := p.List("reset_tokens", "token="+token, 1, 0, "", false)
	if err != nil {
		return "", err
	}
	var page struct {
		Items []struct {
			ID  string          `json:"id"`
			Doc json.RawMessage `json:"doc"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		return "", err
	}
	if page.Total == 0 || len(page.Items) == 0 {
		return "", nil
	}
	doc := map[string]any{}
	_ = json.Unmarshal(page.Items[0].Doc, &doc)
	mbID := stringField(doc, "mailbox_id")
	expires := int64Field(doc, "expires_at")
	if expires > 0 && expires < time.Now().UnixMilli() {
		_ = p.Delete("reset_tokens", page.Items[0].ID)
		return "", nil
	}
	// delete the token (single-use)
	_ = p.Delete("reset_tokens", page.Items[0].ID)
	return mbID, nil
}

// ─── CLI: reset-password ────────────────────────────────────────────────

func handleResetPasswordCmd() {
	fs := newFlagSet("reset-password")
	addr := fs.String("address", "", "mailbox address (primary or alias)")
	_ = fs.Parse(os.Args[3:])
	if *addr == "" {
		fail(80, "input", "--address required", "reset-password --address contact@lacure.enbauges.fr")
	}
	p := newPocheFromEnv()
	if p.Token == "" {
		fail(80, "input", "set POCHE_TOKEN", "")
	}
	_ = ensureMailboxSchema(p)
	_ = ensureResetSchema(p)
	mb, err := findMailboxByAddress(p, *addr)
	if err != nil || mb == nil {
		mb, err = findMailboxByAlias(p, *addr)
		if err != nil || mb == nil {
			fail(90, "not_found", "mailbox not found: "+*addr, "")
		}
	}
	recovery := stringField(mb.Doc, "recovery_email")
	if recovery == "" {
		fail(80, "input", "no recovery_email set on mailbox", "mailbox update --address "+*addr+" --recovery-email emergency@x.fr")
	}
	token, err := createResetToken(p, mb.ID)
	if err != nil {
		fail(100, "integration", "create token: "+err.Error(), "")
	}
	// send the reset email via Resend
	re := newResendFromEnv()
	if !re.enabled() {
		// if Resend is not configured, output the token for manual delivery
		outOK(map[string]any{
			"sent":          false,
			"token":         token,
			"recovery_email": recovery,
			"note":          "RESEND_API_KEY not set — deliver this token manually",
		})
		return
	}
	resetURL := envOr("WEBMAIL_URL", "http://localhost:3090") + "/?reset_token=" + token
	payload := map[string]any{
		"from":    envOr("RESET_FROM", "noreply@intrane.fr"),
		"to":      []string{recovery},
		"subject": "Password reset — poche webmail",
		"text": fmt.Sprintf(
			"A password reset was requested for %s.\n\n"+
				"Click the link below to set a new password (valid 1 hour):\n%s\n\n"+
				"If you did not request this, ignore this email.",
			mb.Address, resetURL,
		),
	}
	if _, err := re.sendEmail(payload); err != nil {
		fail(100, "integration", "send reset email: "+err.Error(), "")
	}
	outOK(map[string]any{"sent": true, "recovery_email": recovery, "mailbox": mb.Address})
}

// ─── API: POST /api/reset-password ──────────────────────────────────────

func handleResetPasswordAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"ok": false, "error": "POST only"})
		return
	}
	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "bad json"})
		return
	}
	if req.Token == "" || req.NewPassword == "" {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "token and new_password required"})
		return
	}
	p := newPocheFromEnv()
	if p.Token == "" {
		writeJSON(w, 500, map[string]any{"ok": false, "error": "POCHE_TOKEN missing"})
		return
	}
	_ = ensureMailboxSchema(p)
	_ = ensureResetSchema(p)
	mbID, err := lookupResetToken(p, req.Token)
	if err != nil || mbID == "" {
		writeJSON(w, 401, map[string]any{"ok": false, "error": "invalid or expired token"})
		return
	}
	mb, err := getMailboxByID(p, mbID)
	if err != nil || mb == nil {
		writeJSON(w, 404, map[string]any{"ok": false, "error": "mailbox not found"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": "bcrypt: " + err.Error()})
		return
	}
	doc := mb.Doc
	doc["password_hash"] = string(hash)
	if _, err := p.Update("mailboxes", mbID, doc); err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": "update: " + err.Error()})
		return
	}
	// invalidate all existing sessions for this mailbox
	_ = deleteSessionsByMailbox(p, mbID)
	writeJSON(w, 200, map[string]any{"ok": true, "data": map[string]any{"reset": true, "address": mb.Address}})
}

// ─── API: POST /api/forgot-password ─────────────────────────────────────

func handleForgotPasswordAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"ok": false, "error": "POST only"})
		return
	}
	var req struct {
		Address string `json:"address"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "bad json"})
		return
	}
	if req.Address == "" {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "address required"})
		return
	}
	p := newPocheFromEnv()
	if p.Token == "" {
		writeJSON(w, 500, map[string]any{"ok": false, "error": "POCHE_TOKEN missing"})
		return
	}
	_ = ensureMailboxSchema(p)
	_ = ensureResetSchema(p)
	mb, err := findMailboxByAddress(p, req.Address)
	if err != nil || mb == nil {
		mb, err = findMailboxByAlias(p, req.Address)
		if err != nil || mb == nil {
			// don't reveal whether the address exists
			writeJSON(w, 200, map[string]any{"ok": true, "data": map[string]any{"sent": "if the address exists, a reset email was sent"}})
			return
		}
	}
	recovery := stringField(mb.Doc, "recovery_email")
	if recovery == "" {
		writeJSON(w, 200, map[string]any{"ok": true, "data": map[string]any{"sent": "if the address exists, a reset email was sent"}})
		return
	}
	token, err := createResetToken(p, mb.ID)
	if err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": "token: " + err.Error()})
		return
	}
	re := newResendFromEnv()
	if re.enabled() {
		resetURL := envOr("WEBMAIL_URL", "http://localhost:3090") + "/?reset_token=" + token
		payload := map[string]any{
			"from":    envOr("RESET_FROM", "noreply@intrane.fr"),
			"to":      []string{recovery},
			"subject": "Password reset — poche webmail",
			"text": fmt.Sprintf(
				"A password reset was requested for %s.\n\n"+
					"Click the link below to set a new password (valid 1 hour):\n%s\n\n"+
					"If you did not request this, ignore this email.",
				mb.Address, resetURL,
			),
		}
		_, _ = re.sendEmail(payload)
	}
	// always return the same response regardless of whether the address exists
	writeJSON(w, 200, map[string]any{"ok": true, "data": map[string]any{"sent": "if the address exists, a reset email was sent"}})
}
