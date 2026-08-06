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

// webmailURLForMailbox is the base URL a tenant's users log in at — it decides
// the host in password-reset links, so a client never sees another tenant's
// hostname.
func webmailURLForMailbox(mb *mailboxRecord) string {
	if mb != nil && mb.WebmailURL != "" {
		return strings.TrimRight(mb.WebmailURL, "/")
	}
	return strings.TrimRight(envOr("WEBMAIL_URL", "http://localhost:3090"), "/")
}

// resetFromForMailbox picks the sender for a reset email. A mailbox on its own
// Resend account cannot send as the shared RESET_FROM address (that domain is
// verified in a different account), so it falls back to its own address.
func resetFromForMailbox(mb *mailboxRecord) string {
	if mb != nil {
		if mb.ResetFrom != "" {
			return mb.ResetFrom
		}
		if mb.ResendAPIKey != "" {
			return mb.Address
		}
	}
	return envOr("RESET_FROM", "noreply@intrane.fr")
}

// sendResetEmail delivers a reset link through the mailbox's own transport.
func sendResetEmail(mb *mailboxRecord, recovery, token string) error {
	re := resendForMailbox(mb)
	if !re.enabled() {
		return fmt.Errorf("RESEND_API_KEY not set")
	}
	resetURL := webmailURLForMailbox(mb) + "/?reset_token=" + token
	_, err := re.sendEmail(map[string]any{
		"from":    resetFromForMailbox(mb),
		"to":      []string{recovery},
		"subject": "Password reset — poche webmail",
		"text": fmt.Sprintf(
			"A password reset was requested for %s.\n\n"+
				"Click the link below to set a new password (valid 1 hour):\n%s\n\n"+
				"If you did not request this, ignore this email.",
			mb.Address, resetURL,
		),
	})
	return err
}

// brandFor is the name shown in a tenant's sidebar: the mailbox's own brand,
// then the deployment default, then the product name. One deployment serves
// several clients, so this cannot be a single global string.
func brandFor(mb *mailboxRecord) string {
	if mb != nil && mb.Brand != "" {
		return mb.Brand
	}
	return envOr("BRAND_NAME", "poche")
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
