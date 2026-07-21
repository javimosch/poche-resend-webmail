package main

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
	"strings"
)

func ensureWebmailToken() string {
	t := os.Getenv("WEBMAIL_TOKEN")
	if t != "" {
		return t
	}
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	t = hex.EncodeToString(b)
	_ = os.Setenv("WEBMAIL_TOKEN", t)
	return t
}

func webmailToken() string {
	return os.Getenv("WEBMAIL_TOKEN")
}

func requestToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(h), "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return r.URL.Query().Get("token")
}

func authed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		want := webmailToken()
		if want == "" {
			writeJSON(w, 500, map[string]any{"ok": false, "error": "WEBMAIL_TOKEN not set"})
			return
		}
		got := requestToken(r)
		if got == "" || got != want {
			writeJSON(w, 401, map[string]any{"ok": false, "error": "unauthorized"})
			return
		}
		next(w, r)
	}
}
