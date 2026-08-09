package main

import (
	"os"
	"strings"
	"testing"
)

// TestExtractAttachmentFromRawEmail_RealCapture verifies against a REAL
// captured raw email (same probe message used to fix the identical bug in
// machin-resend-inbox), not a synthetic fixture. Skips if the file isn't
// present — this is a manual verification aid, not a permanent CI fixture
// (the raw file isn't committed).
func TestExtractAttachmentFromRawEmail_RealCapture(t *testing.T) {
	raw, err := os.ReadFile("/tmp/lacure-raw-email-go.eml")
	if err != nil {
		t.Skip("no real capture at /tmp/lacure-raw-email-go.eml, skipping")
	}
	data, err := extractAttachmentFromRawEmail(raw, "devis-probe.pdf")
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	if len(data) != 240 {
		t.Fatalf("expected 240 bytes (Resend's own reported size), got %d", len(data))
	}
	if !strings.HasPrefix(string(data), "%PDF") {
		t.Fatalf("expected a PDF header, got: %q", string(data[:20]))
	}
	if !strings.Contains(string(data), "fake devis for attachment probe") {
		t.Fatalf("expected the known probe content, got: %q", string(data))
	}
	t.Logf("extracted %d bytes: %s", len(data), string(data))

	_, err = extractAttachmentFromRawEmail(raw, "nosuchfile.xyz")
	if err == nil {
		t.Fatal("expected an error for a nonexistent filename")
	}
}
