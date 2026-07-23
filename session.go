package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ─── sessions ───────────────────────────────────────────────────────────

func createSession(p *Poche, mailboxID string) (string, error) {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)
	expires := time.Now().Add(7 * 24 * time.Hour).UnixMilli()
	_, err := p.Create("sessions", map[string]any{
		"token":      token,
		"mailbox_id": mailboxID,
		"expires_at": expires,
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

func lookupSession(p *Poche, token string) (*mailboxRecord, error) {
	data, err := p.List("sessions", "token="+token, 1, 0, "", false)
	if err != nil {
		return nil, err
	}
	var page struct {
		Items []struct {
			ID  string          `json:"id"`
			Doc json.RawMessage `json:"doc"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		return nil, err
	}
	if page.Total == 0 || len(page.Items) == 0 {
		return nil, nil
	}
	doc := map[string]any{}
	_ = json.Unmarshal(page.Items[0].Doc, &doc)
	mbID := stringField(doc, "mailbox_id")
	expires := int64Field(doc, "expires_at")
	if expires > 0 && expires < time.Now().UnixMilli() {
		_ = p.Delete("sessions", page.Items[0].ID)
		return nil, nil
	}
	// load the mailbox
	raw, err := p.Get("mailboxes", mbID)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Doc json.RawMessage `json:"doc"`
	}
	_ = json.Unmarshal(raw, &wrap)
	return parseMailbox(mbID, wrap.Doc), nil
}

func deleteSessionsByMailbox(p *Poche, mailboxID string) error {
	data, err := p.List("sessions", "mailbox_id="+mailboxID, 1000, 0, "", false)
	if err != nil {
		return err
	}
	var page struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	_ = json.Unmarshal(data, &page)
	for _, it := range page.Items {
		_ = p.Delete("sessions", it.ID)
	}
	return nil
}

// ─── login API ──────────────────────────────────────────────────────────

func handleLoginAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"ok": false, "error": "POST only"})
		return
	}
	var req struct {
		Address  string `json:"address"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "bad json"})
		return
	}
	if req.Address == "" || req.Password == "" {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "address and password required"})
		return
	}
	p := newPocheFromEnv()
	if p.Token == "" {
		writeJSON(w, 500, map[string]any{"ok": false, "error": "POCHE_TOKEN missing"})
		return
	}
	_ = ensureMailboxSchema(p)
	mb, err := findMailboxByAddress(p, req.Address)
	if err != nil || mb == nil {
		writeJSON(w, 401, map[string]any{"ok": false, "error": "invalid credentials"})
		return
	}
	if !mb.IsActive {
		writeJSON(w, 403, map[string]any{"ok": false, "error": "mailbox suspended"})
		return
	}
	if mb.PasswordHash == "" {
		writeJSON(w, 401, map[string]any{"ok": false, "error": "no password set on mailbox"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(mb.PasswordHash), []byte(req.Password)); err != nil {
		writeJSON(w, 401, map[string]any{"ok": false, "error": "invalid credentials"})
		return
	}
	token, err := createSession(p, mb.ID)
	if err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": "session: " + err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{
		"ok": true,
		"data": map[string]any{
			"token":      token,
			"address":    mb.Address,
			"name":       mb.Name,
			"max_bytes":  mb.MaxBytes,
			"expires_in": "7d",
		},
	})
}

// ─── auth context ───────────────────────────────────────────────────────

type authCtx struct {
	IsAdmin bool
	Mailbox *mailboxRecord
}

// authMiddleware replaces the old authed() — supports both admin token and
// per-mailbox session tokens. The mailbox_id is passed via request headers.
func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := resolveAuth(r)
		if ctx == nil {
			writeJSON(w, 401, map[string]any{"ok": false, "error": "unauthorized"})
			return
		}
		if !ctx.IsAdmin && ctx.Mailbox != nil && !ctx.Mailbox.IsActive {
			writeJSON(w, 403, map[string]any{"ok": false, "error": "mailbox suspended"})
			return
		}
		r.Header.Set("X-Auth-Mailbox-ID", ctx.MailboxID())
		r.Header.Set("X-Auth-Is-Admin", fmt.Sprintf("%v", ctx.IsAdmin))
		next(w, r)
	}
}

func resolveAuth(r *http.Request) *authCtx {
	tok := requestToken(r)
	if tok == "" {
		return nil
	}
	// check admin token first
	adminTok := os.Getenv("ADMIN_TOKEN")
	if adminTok != "" && tok == adminTok {
		return &authCtx{IsAdmin: true}
	}
	// check legacy global WEBMAIL_TOKEN (backwards compat)
	wmTok := os.Getenv("WEBMAIL_TOKEN")
	if wmTok != "" && tok == wmTok {
		return &authCtx{IsAdmin: true}
	}
	// check session token
	p := newPocheFromEnv()
	if p.Token == "" {
		return nil
	}
	mb, err := lookupSession(p, tok)
	if err != nil || mb == nil {
		return nil
	}
	return &authCtx{Mailbox: mb}
}

func (c *authCtx) MailboxID() string {
	if c == nil {
		return ""
	}
	if c.IsAdmin {
		return ""
	}
	if c.Mailbox != nil {
		return c.Mailbox.ID
	}
	return ""
}

func authMailboxID(r *http.Request) string {
	return r.Header.Get("X-Auth-Mailbox-ID")
}

func authIsAdmin(r *http.Request) bool {
	return r.Header.Get("X-Auth-Is-Admin") == "true"
}

// ─── mailbox quota helpers ──────────────────────────────────────────────

func mailboxUsage(p *Poche, mailboxID string) (count int, bytes int64, err error) {
	_, count, bytes, err = loadMailboxMessages(p, mailboxID)
	return
}

func mailboxAllowsIngest(p *Poche, mailboxID string, newSize int64) error {
	mb, err := getMailboxByID(p, mailboxID)
	if err != nil || mb == nil {
		return fmt.Errorf("mailbox not found")
	}
	if !mb.IsActive {
		return fmt.Errorf("mailbox suspended")
	}
	_, used, err := mailboxUsage(p, mailboxID)
	if err != nil {
		return err
	}
	maxBytes := mb.MaxBytes
	if maxBytes == 0 {
		maxBytes = envInt64("MAILBOX_MAX_BYTES", defaultMaxBytes)
	}
	if used+newSize > maxBytes {
		return fmt.Errorf("quota exceeded: %d + %d > %d", used, newSize, maxBytes)
	}
	return nil
}

func getMailboxByID(p *Poche, id string) (*mailboxRecord, error) {
	raw, err := p.Get("mailboxes", id)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Doc json.RawMessage `json:"doc"`
	}
	_ = json.Unmarshal(raw, &wrap)
	return parseMailbox(id, wrap.Doc), nil
}

// findOrCreateMailboxForAddress routes inbound email to the right mailbox.
// Returns empty string if no mailbox matches (email is dropped).
func findOrCreateMailboxForAddress(p *Poche, addr string) (string, error) {
	addr = strings.ToLower(strings.Trim(addr, "<>"))
	mb, err := findMailboxByAddress(p, addr)
	if err != nil {
		return "", err
	}
	if mb != nil {
		return mb.ID, nil
	}
	return "", nil
}
