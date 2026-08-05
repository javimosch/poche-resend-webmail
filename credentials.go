package main

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Per-mailbox Resend credentials.
//
// Each tenant can own a separate Resend account: its own verified domain,
// its own API key, its own webhook signing secret. When a mailbox carries a
// key we use it; otherwise we fall back to the process-wide RESEND_API_KEY.
// This is what lets one deployment serve several client domains.

// resendForMailbox returns a Resend client bound to the mailbox's own key when
// it has one, else the environment key.
func resendForMailbox(mb *mailboxRecord) *Resend {
	re := newResendFromEnv()
	if mb != nil && mb.ResendAPIKey != "" {
		re.Key = mb.ResendAPIKey
	}
	return re
}

// webhookSecretForMailbox returns the signing secret to verify an inbound
// webhook against: the mailbox's own secret when set, else the env secret.
func webhookSecretForMailbox(mb *mailboxRecord) string {
	if mb != nil && mb.ResendWebhookSecret != "" {
		return mb.ResendWebhookSecret
	}
	return os.Getenv("RESEND_WEBHOOK_SECRET")
}

// maskSecret renders a credential for human/agent output without disclosing it.
func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "…"
	}
	return s[:3] + "…" + s[len(s)-4:]
}

// readSecretFlag resolves a secret supplied on the command line.
// "-" reads one line from stdin and "env:NAME" reads an environment variable,
// so keys never have to appear in shell history or in the process list.
func readSecretFlag(v string) (string, error) {
	switch {
	case v == "":
		return "", nil
	case v == "-":
		raw, err := io.ReadAll(io.LimitReader(os.Stdin, 4096))
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		return strings.TrimSpace(string(raw)), nil
	case strings.HasPrefix(v, "env:"):
		name := strings.TrimPrefix(v, "env:")
		got := strings.TrimSpace(os.Getenv(name))
		if got == "" {
			return "", fmt.Errorf("env %s is empty", name)
		}
		return got, nil
	default:
		return strings.TrimSpace(v), nil
	}
}
