package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// Tenant Resend credentials live in poche, and poche has no encryption at
// rest — the store writes append-only WAL chunks in the clear. Anything that
// copies the data directory (a backup, a snapshot, the tarball taken before a
// deploy) therefore carries live client credentials.
//
// These fields are encrypted with AES-256-GCM before they are stored, keyed by
// CREDENTIALS_KEY from the environment file. The key sits on the same host as
// the data, so this does not hide anything from root here; it makes copies of
// the database useless on their own, which is the leak path that actually
// happens.

const secretPrefix = "enc:v1:"

// credentialsKey returns the 32-byte key, or nil when none is configured.
func credentialsKey() ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv("CREDENTIALS_KEY"))
	if raw == "" {
		return nil, nil
	}
	var key []byte
	var err error
	if k, e := hex.DecodeString(raw); e == nil && len(k) == 32 {
		key = k
	} else if k, e := base64.StdEncoding.DecodeString(raw); e == nil && len(k) == 32 {
		key = k
	} else {
		err = fmt.Errorf("CREDENTIALS_KEY must be 32 bytes as hex or base64")
	}
	return key, err
}

func isEncrypted(s string) bool { return strings.HasPrefix(s, secretPrefix) }

// encryptSecret returns the value in stored form. Without a configured key the
// value is stored as-is, so the feature can be adopted without a flag day.
func encryptSecret(plain string) (string, error) {
	if plain == "" || isEncrypted(plain) {
		return plain, nil
	}
	key, err := credentialsKey()
	if err != nil {
		return "", err
	}
	if key == nil {
		fmt.Fprintf(os.Stderr, "{\"event\":\"credential_stored_plaintext\",\"hint\":%q}\n",
			"set CREDENTIALS_KEY to encrypt tenant credentials at rest")
		return plain, nil
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return secretPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// decryptSecret reverses encryptSecret. Values that were stored before a key
// existed are returned unchanged, so old rows keep working.
func decryptSecret(stored string) (string, error) {
	if stored == "" || !isEncrypted(stored) {
		return stored, nil
	}
	key, err := credentialsKey()
	if err != nil {
		return "", err
	}
	if key == nil {
		return "", fmt.Errorf("value is encrypted but CREDENTIALS_KEY is not set")
	}
	blob, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, secretPrefix))
	if err != nil {
		return "", fmt.Errorf("bad ciphertext encoding: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(blob) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext too short")
	}
	plain, err := gcm.Open(nil, blob[:gcm.NonceSize()], blob[gcm.NonceSize():], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt failed (wrong CREDENTIALS_KEY?)")
	}
	return string(plain), nil
}

// decryptOrWarn is for read paths that cannot return an error. A failure here
// means sending or webhook verification will fall back to the env credentials,
// which is confusing unless it is said out loud.
func decryptOrWarn(stored, field, mailbox string) string {
	v, err := decryptSecret(stored)
	if err != nil {
		fmt.Fprintf(os.Stderr, "{\"event\":\"credential_decrypt_err\",\"field\":%q,\"mailbox\":%q,\"err\":%q}\n",
			field, mailbox, err.Error())
		return ""
	}
	return v
}

// ─── CLI ────────────────────────────────────────────────────────────────

// handleSecretKeyCmd prints a fresh key for CREDENTIALS_KEY.
func handleSecretKeyCmd() {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		fail(110, "internal", "rand: "+err.Error(), "")
	}
	outOK(map[string]any{
		"credentials_key": hex.EncodeToString(b),
		"note":            "put this in CREDENTIALS_KEY (env file, mode 600), then run 'mailbox encrypt-secrets --apply'",
		"warning":         "losing this key makes stored tenant credentials unrecoverable — they must be re-entered",
	})
}

// handleEncryptSecretsCmd migrates credentials stored before a key existed.
func handleEncryptSecretsCmd() {
	fs := newFlagSet("mailbox encrypt-secrets")
	apply := fs.Bool("apply", false, "write changes (default is a dry run)")
	_ = fs.Parse(os.Args[3:])

	key, err := credentialsKey()
	if err != nil {
		fail(80, "input", err.Error(), "")
	}
	if key == nil {
		fail(80, "input", "CREDENTIALS_KEY is not set", "poche-resend-webmail secret-key")
	}
	p := newPocheFromEnv()
	if p.Token == "" {
		fail(80, "input", "set POCHE_TOKEN", "")
	}
	if err := ensureMailboxSchema(p); err != nil {
		fail(100, "integration", "schema: "+err.Error(), "")
	}
	mboxes, err := listAllMailboxes(p)
	if err != nil {
		fail(100, "integration", "list: "+err.Error(), "")
	}
	scanned, changed := 0, 0
	for i := range mboxes {
		mb := &mboxes[i]
		scanned++
		doc := mb.Doc
		touched := false
		for _, field := range []string{"resend_api_key", "resend_webhook_secret"} {
			cur := stringField(doc, field)
			if cur == "" || isEncrypted(cur) {
				continue
			}
			sealed, err := encryptSecret(cur)
			if err != nil {
				fail(110, "internal", field+": "+err.Error(), "")
			}
			doc[field] = sealed
			touched = true
		}
		if !touched {
			continue
		}
		changed++
		if !*apply {
			continue
		}
		if _, err := p.Update("mailboxes", mb.ID, doc); err != nil {
			fail(100, "integration", "update "+mb.Address+": "+err.Error(), "")
		}
	}
	outOK(map[string]any{
		"scanned": scanned,
		"changed": changed,
		"applied": *apply,
		"note":    "old plaintext values remain in previous WAL chunks until poche compacts — rotate the credentials to be safe",
	})
}
