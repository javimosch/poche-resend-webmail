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

// authed is kept for backwards compatibility but now delegates to authMiddleware.
// New code should use authMiddleware directly (it supports per-mailbox sessions).
func authed(next http.HandlerFunc) http.HandlerFunc {
	return authMiddleware(next)
}
