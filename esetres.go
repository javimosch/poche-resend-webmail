package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"
)

// esetres.go — a thin client for machin-esetres (self-hosted object store),
// used to persist inbound attachment bytes past Resend's expiring
// raw.download_url. Go's net/http is already binary-safe, unlike machin's
// own HTTP client builtins (see machin-resend-inbox's esetres.go/mime.src,
// which had to shell out to curl for exactly that reason) — no workaround
// needed on this side.

func esetresURL() string    { return envOr("ESETRES_URL", "") }
func esetresBucket() string { return envOr("ESETRES_BUCKET", "") }
func esetresToken() string  { return envOr("ESETRES_TOKEN", "") }

func esetresEnabled() bool {
	return esetresURL() != "" && esetresBucket() != "" && esetresToken() != ""
}

var esetresClient = &http.Client{Timeout: 30 * time.Second}

func esetresObjectURL(key string) string {
	return esetresURL() + "/b/" + esetresBucket() + "/o/" + key
}

// esetresExists checks a key without downloading it (Phase 1's HEAD returns
// JSON metadata, not a bodyless response — see machin-esetres's own docs for
// why; a 200 status is all this needs).
func esetresExists(key string) bool {
	if !esetresEnabled() {
		return false
	}
	req, err := http.NewRequest("HEAD", esetresObjectURL(key), nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+esetresToken())
	resp, err := esetresClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

// esetresPut uploads data under key.
func esetresPut(key string, data []byte, contentType string) error {
	if !esetresEnabled() {
		return fmt.Errorf("machin-esetres not configured")
	}
	req, err := http.NewRequest("PUT", esetresObjectURL(key), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+esetresToken())
	req.Header.Set("Content-Type", contentType)
	resp, err := esetresClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("esetres put %d: %s", resp.StatusCode, truncate(string(body), 200))
	}
	return nil
}

// esetresGet downloads key, returning its bytes and Content-Type.
func esetresGet(key string) ([]byte, string, error) {
	if !esetresEnabled() {
		return nil, "", fmt.Errorf("machin-esetres not configured")
	}
	req, err := http.NewRequest("GET", esetresObjectURL(key), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+esetresToken())
	resp, err := esetresClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("esetres get: %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	return data, resp.Header.Get("Content-Type"), nil
}
