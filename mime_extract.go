package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

var rawEmailClient = &http.Client{Timeout: 30 * time.Second}

// fetchRawEmail downloads a Resend raw.download_url. Go's http.Client is
// already binary-safe, so no curl-style workaround is needed on this side
// (unlike machin-resend-inbox's MFL client, whose http_get/https_get return
// a string that truncates at an embedded NUL byte).
func fetchRawEmail(url string) ([]byte, error) {
	resp, err := rawEmailClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("raw email fetch: %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// mime_extract.go — pull one named attachment's bytes out of a raw RFC822
// email. Needed because Resend's inbound attachment objects carry NO
// per-attachment download link (confirmed against a real message with a
// genuine read-capable key, not assumed from docs — see the machin-resend-inbox
// sibling project's issue #1, which hit and fixed the identical wrong
// assumption). What Resend DOES give is `.raw.download_url`, a link to the
// entire raw email; getting one attachment means downloading that and
// pulling the matching MIME part out. Go's mime/multipart + net/mail do this
// natively — no hand-rolled parser needed here, unlike machin-resend-inbox's
// MFL implementation (mime.src), which had no such stdlib to reach for.
func extractAttachmentFromRawEmail(raw []byte, filename string) ([]byte, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("parse raw email: %w", err)
	}
	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		return nil, fmt.Errorf("parse content-type: %w", err)
	}
	if len(mediaType) < 10 || mediaType[:10] != "multipart/" {
		return nil, fmt.Errorf("not a multipart message: %s", mediaType)
	}
	boundary := params["boundary"]
	if boundary == "" {
		return nil, fmt.Errorf("multipart message has no boundary")
	}
	mr := multipart.NewReader(msg.Body, boundary)
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read mime part: %w", err)
		}
		if part.FileName() == filename {
			raw, err := io.ReadAll(part)
			part.Close()
			if err != nil {
				return nil, fmt.Errorf("read part body: %w", err)
			}
			// mime/multipart targets HTTP form-data and does NOT decode
			// Content-Transfer-Encoding for RFC822 email MIME — a real
			// stdlib gap for this use case, not an oversight here. base64 is
			// what Resend/SES uses for binary attachments; strip the MIME
			// line-wrapping (76 chars, CRLF) before decoding.
			cte := strings.ToLower(strings.TrimSpace(part.Header.Get("Content-Transfer-Encoding")))
			if cte == "base64" {
				clean := strings.NewReplacer("\r", "", "\n", "").Replace(string(raw))
				decoded, err := base64.StdEncoding.DecodeString(clean)
				if err != nil {
					return nil, fmt.Errorf("base64-decode attachment part: %w", err)
				}
				return decoded, nil
			}
			return raw, nil
		}
		part.Close()
	}
	return nil, fmt.Errorf("attachment %q not found in the raw email's MIME parts", filename)
}
