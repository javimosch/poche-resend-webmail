package main

import (
	"encoding/base64"
	"strings"
	"testing"
)

// syntheticRawEmail builds a minimal multipart/mixed RFC822 message with one
// base64-encoded attachment part, shaped like what SES/Resend actually
// produce (verified against a real capture in mime_extract_manual_test.go —
// this is the permanent, network-independent regression test built from
// that same real-world shape).
func syntheticRawEmail(attachmentName string, attachmentBytes []byte) string {
	b64 := base64.StdEncoding.EncodeToString(attachmentBytes)
	var wrapped strings.Builder
	for i := 0; i < len(b64); i += 76 {
		end := i + 76
		if end > len(b64) {
			end = len(b64)
		}
		wrapped.WriteString(b64[i:end])
		wrapped.WriteString("\r\n")
	}
	return "From: sender@example.com\r\n" +
		"To: recipient@example.com\r\n" +
		"Subject: test\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=\"BOUNDARY123\"\r\n" +
		"\r\n" +
		"--BOUNDARY123\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"\r\n" +
		"body text\r\n" +
		"--BOUNDARY123\r\n" +
		"Content-Type: application/octet-stream; name=" + attachmentName + "\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"Content-Disposition: attachment; filename=" + attachmentName + "\r\n" +
		"\r\n" +
		wrapped.String() +
		"--BOUNDARY123--\r\n"
}

func TestExtractAttachmentFromRawEmail(t *testing.T) {
	want := []byte("hello attachment bytes, including \x00 a NUL and \xff a high byte")
	raw := syntheticRawEmail("test.bin", want)

	got, err := extractAttachmentFromRawEmail([]byte(raw), "test.bin")
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestExtractAttachmentFromRawEmail_NotFound(t *testing.T) {
	raw := syntheticRawEmail("test.bin", []byte("data"))
	_, err := extractAttachmentFromRawEmail([]byte(raw), "nosuchfile.xyz")
	if err == nil {
		t.Fatal("expected an error for a nonexistent filename")
	}
}

func TestExtractAttachmentFromRawEmail_NotMultipart(t *testing.T) {
	raw := "From: a@b.com\r\nTo: c@d.com\r\nSubject: x\r\nContent-Type: text/plain\r\n\r\nhi\r\n"
	_, err := extractAttachmentFromRawEmail([]byte(raw), "test.bin")
	if err == nil {
		t.Fatal("expected an error for a non-multipart message")
	}
}
