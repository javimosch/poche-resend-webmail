package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type Resend struct {
	Key    string
	Base   string
	Client *http.Client
}

func newResendFromEnv() *Resend {
	return &Resend{
		Key:    os.Getenv("RESEND_API_KEY"),
		Base:   envOr("RESEND_BASE_URL", "https://api.resend.com"),
		Client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (r *Resend) enabled() bool { return r.Key != "" }

func (r *Resend) do(method, path string, body any) (int, []byte, error) {
	if !r.enabled() {
		return 0, nil, fmt.Errorf("RESEND_API_KEY not set")
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		rdr = bytes.NewReader(b)
	}
	base := r.Base
	if base == "" {
		base = "https://api.resend.com"
	}
	req, err := http.NewRequest(method, base+path, rdr)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.Key)
	req.Header.Set("Content-Type", "application/json")
	res, err := r.Client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	return res.StatusCode, raw, err
}

// Receiving list — try /emails/receiving then legacy /emails/inbound.
func (r *Resend) listReceiving() ([]map[string]any, error) {
	for _, path := range []string{"/emails/receiving", "/emails/inbound"} {
		code, raw, err := r.do("GET", path, nil)
		if err != nil {
			return nil, err
		}
		if code == 404 {
			continue
		}
		if code < 200 || code >= 300 {
			return nil, fmt.Errorf("resend list %s: %d %s", path, code, truncate(string(raw), 200))
		}
		var wrap struct {
			Data []map[string]any `json:"data"`
		}
		if err := json.Unmarshal(raw, &wrap); err != nil {
			return nil, err
		}
		return wrap.Data, nil
	}
	return nil, fmt.Errorf("no receiving list endpoint")
}

func (r *Resend) getReceiving(id string) (map[string]any, error) {
	var lastErr error
	for _, path := range []string{"/emails/receiving/" + id, "/emails/inbound/" + id} {
		code, raw, err := r.do("GET", path, nil)
		if err != nil {
			return nil, err
		}
		if code == 404 {
			lastErr = fmt.Errorf("not found")
			continue
		}
		if code < 200 || code >= 300 {
			return nil, fmt.Errorf("resend get: %d %s", code, truncate(string(raw), 200))
		}
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, err
		}
		// some APIs wrap in data
		if data, ok := doc["data"].(map[string]any); ok {
			return data, nil
		}
		return doc, nil
	}
	return nil, lastErr
}

func (r *Resend) sendEmail(payload map[string]any) (map[string]any, error) {
	code, raw, err := r.do("POST", "/emails", payload)
	if err != nil {
		return nil, err
	}
	if code < 200 || code >= 300 {
		return nil, fmt.Errorf("resend send: %d %s", code, truncate(string(raw), 300))
	}
	var doc map[string]any
	_ = json.Unmarshal(raw, &doc)
	return doc, nil
}

func mailFromAllowlist() []string {
	raw := os.Getenv("MAIL_FROM_ALLOWLIST")
	if raw == "" {
		raw = "@intrane.fr"
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func fromAllowed(addr string) bool {
	a := strings.ToLower(strings.TrimSpace(addr))
	if i := strings.LastIndex(a, "<"); i >= 0 {
		if j := strings.LastIndex(a, ">"); j > i {
			a = a[i+1 : j]
		}
	}
	for _, suf := range mailFromAllowlist() {
		if strings.HasSuffix(a, suf) {
			return true
		}
	}
	return false
}

func emailOf(addr string) string {
	a := strings.TrimSpace(addr)
	if i := strings.LastIndex(a, "<"); i >= 0 {
		if j := strings.LastIndex(a, ">"); j > i {
			return strings.TrimSpace(a[i+1 : j])
		}
	}
	return a
}
