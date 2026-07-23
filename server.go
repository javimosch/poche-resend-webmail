package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"time"
)

//go:embed ui/*
var uiFiles embed.FS

func startServer(port int) {
	started := time.Now()
	token := ensureWebmailToken()
	poche := newPocheFromEnv()

	// ensure schema is set up before serving
	if poche.Token != "" {
		_ = ensureSchema(poche)
		_ = ensureMailboxSchema(poche)
		_ = ensureTags(poche)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"ok": true, "status": "healthy", "version": Version})
	})
	mux.HandleFunc("/webhooks/resend", handleWebhook)
	mux.HandleFunc("/api/login", handleLoginAPI)
	mux.HandleFunc("/api/forgot-password", handleForgotPasswordAPI)
	mux.HandleFunc("/api/reset-password", handleResetPasswordAPI)
	mux.HandleFunc("/api/mailbox/usage", authMiddleware(handleMailboxUsageAPI))

	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		authMode := "session"
		if os.Getenv("WEBMAIL_TOKEN") != "" && os.Getenv("ADMIN_TOKEN") == "" {
			authMode = "token"
		}
		writeJSON(w, 200, map[string]any{
			"ok":      true,
			"version": Version,
			"data": map[string]any{
				"ui_port":        port,
				"auth":           authMode,
				"resend_enabled": newResendFromEnv().enabled(),
				"note":           "Login with address+password, or Bearer token.",
			},
		})
	})
	mux.HandleFunc("/api/status", authed(func(w http.ResponseWriter, r *http.Request) {
		pocheOK := poche.Health() == nil
		count := 0
		if pocheOK && poche.Token != "" {
			if n, err := poche.Count("messages", ""); err == nil {
				count = n
			}
		}
		writeJSON(w, 200, map[string]any{
			"ok": true,
			"data": map[string]any{
				"status":         "running",
				"port":           port,
				"uptime":         time.Since(started).String(),
				"version":        Version,
				"poche_ok":       pocheOK,
				"message_n":      count,
				"resend_enabled": newResendFromEnv().enabled(),
			},
		})
	}))

	mux.HandleFunc("/api/messages", authed(handleMessagesAPI))
	mux.HandleFunc("/api/messages/", authed(handleMessagesAPI))
	mux.HandleFunc("/api/bulk", authed(handleBulkAPI))
	mux.HandleFunc("/api/tags", authed(handleTagsAPI))
	mux.HandleFunc("/api/message-tags", authed(handleMessageTagsAPI))
	mux.HandleFunc("/api/reply", authed(handleReplyAPI))
	mux.HandleFunc("/api/cleanup", authed(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, 405, map[string]any{"ok": false, "error": "POST only"})
			return
		}
		summary, err := cleanup()
		if err != nil {
			writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "data": summary})
	}))
	mux.HandleFunc("/api/sync", authed(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, 405, map[string]any{"ok": false, "error": "POST only"})
			return
		}
		n, err := syncInbound(0)
		if err != nil {
			writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "data": map[string]any{"synced": n}})
	}))
	mux.HandleFunc("/api/attachments/", authed(handleAttachmentOpen))

	uiSub, err := fs.Sub(uiFiles, "ui")
	if err != nil {
		fail(110, "internal", "embed ui: "+err.Error(), "")
	}
	mux.Handle("/", http.FileServer(http.FS(uiSub)))

	fmt.Fprintf(os.Stderr, "{\"event\":\"serve\",\"url\":\"http://127.0.0.1:%d\",\"poche\":%q,\"token_hint\":%q}\n",
		port, poche.Base, token[:8]+"…")
	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), withCORS(mux)); err != nil {
		fail(110, "internal", "server error: "+err.Error(), "")
	}
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func handleGuide() {
	outOK(map[string]any{
		"tool":        "poche-resend-webmail",
		"what":        "OSS self-hosted Resend webmail over poche",
		"license":     "MIT",
		"auth":        "session (address+password → POST /api/login → Bearer) or ADMIN_TOKEN/WEBMAIL_TOKEN (legacy admin)",
		"store":       "poche (tags, has_link/missing_link, search)",
		"transport":   "Resend receiving + send",
		"webhook":     "POST /webhooks/resend (email.received, routed by to_addr)",
		"attachments": "GET /api/attachments/:id/open → new tab",
		"cleanup":     "POST /api/cleanup · retention/quota purge (per-mailbox)",
		"star":        "PUT /api/messages/:id/star · toggle",
		"bulk":        "POST /api/bulk · mark_read|mark_unread|archive|unarchive|delete|star|unstar|tag|untag",
		"mailbox":     "CLI: mailbox create|list|update|delete|alias|reset-password — per-address auth + storage caps + aliases",
		"login":       "POST /api/login {address, password} → {token, address, max_bytes} (address can be primary or alias)",
		"forgot":      "POST /api/forgot-password {address} → sends reset link to recovery_email",
		"reset":       "POST /api/reset-password {token, new_password} → sets new password, invalidates sessions",
		"cli":         []string{"serve", "seed", "sync", "cleanup", "mailbox", "list", "read", "reply", "guide"},
	})
}
