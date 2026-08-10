package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func splitCSV(s string) []string {
	return strings.Split(s, ",")
}

// ─── schema ─────────────────────────────────────────────────────────────

// mailboxSchemaExtra are the fields added to the existing mailboxes collection
// for multi-tenant auth + provisioning. The base fields (name, address,
// retention_months, max_messages, max_bytes) stay as-is.
const mailboxSchemaExtra = "password_hash:string,is_active:bool,created_at:int"

// sessions collection: per-login session tokens scoped to a mailbox.
const sessionsSchema = "token:string!required!unique,mailbox_id:string!required!ref=mailboxes,expires_at:int"

func ensureMailboxSchema(p *Poche) error {
	if err := p.AdminSchema("mailboxes", "name:string!required!unique,address:string!required!unique,retention_months:float,max_messages:int,max_bytes:int,message_count:int,used_bytes:int,password_hash:string,recovery_email:string,resend_api_key:string,resend_webhook_secret:string,webmail_url:string,reset_from:string,brand:string,is_active:bool,created_at:int,catchall_domain:string"); err != nil {
		return fmt.Errorf("mailboxes: %w", err)
	}
	_ = p.AdminIndex("mailboxes", "catchall_domain", "")
	if err := p.AdminSchema("sessions", sessionsSchema); err != nil {
		return fmt.Errorf("sessions: %w", err)
	}
	if err := p.AdminExpose("mailboxes", "read,create,update,delete"); err != nil {
		return err
	}
	if err := p.AdminExpose("sessions", "read,create,delete"); err != nil {
		return err
	}
	_ = p.AdminIndex("sessions", "token", "")
	_ = p.AdminIndex("sessions", "mailbox_id", "")
	_ = ensureAliasesSchema(p)
	_ = ensureResetSchema(p)
	return nil
}

// ─── mailbox CRUD ───────────────────────────────────────────────────────

type mailboxRecord struct {
	ID            string
	Address       string
	Name          string
	PasswordHash  string
	RecoveryEmail string
	// Per-tenant Resend credentials; empty ⇒ fall back to the env key/secret.
	ResendAPIKey        string
	ResendWebhookSecret string
	// Tenant-facing identity: login host used in reset links, and the sender
	// those reset emails come from.
	WebmailURL      string
	ResetFrom       string
	Brand           string
	// CatchallDomain, when set, makes this mailbox the inbound target for any
	// address at that domain that doesn't match a real mailbox or alias
	// first — e.g. "intrane.fr" catches mail to any-name@intrane.fr. At most
	// one mailbox may claim a given domain (enforced in mailboxUpdateCmd).
	CatchallDomain  string
	IsActive        bool
	RetentionMonths float64
	MaxMessages     int
	MaxBytes        int64
	MessageCount    int
	UsedBytes       int64
	CreatedAt       int64
	Doc             map[string]any
}

func findMailboxByAddress(p *Poche, addr string) (*mailboxRecord, error) {
	data, err := p.List("mailboxes", "address="+addr, 1, 0, "", false)
	if err != nil {
		return nil, err
	}
	var page struct {
		Items []struct {
			ID  string          `json:"id"`
			Doc json.RawMessage `json:"doc"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		return nil, err
	}
	if page.Total == 0 || len(page.Items) == 0 {
		return nil, nil
	}
	return parseMailbox(page.Items[0].ID, page.Items[0].Doc), nil
}

// findMailboxByCatchallDomain looks up the mailbox (if any) configured to
// catch mail for an entire domain. addrDomain must already be the lowercase
// part after '@' — callers derive it from the inbound recipient address.
func findMailboxByCatchallDomain(p *Poche, addrDomain string) (*mailboxRecord, error) {
	if addrDomain == "" {
		return nil, nil
	}
	data, err := p.List("mailboxes", "catchall_domain="+addrDomain, 1, 0, "", false)
	if err != nil {
		return nil, err
	}
	var page struct {
		Items []struct {
			ID  string          `json:"id"`
			Doc json.RawMessage `json:"doc"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		return nil, err
	}
	if page.Total == 0 || len(page.Items) == 0 {
		return nil, nil
	}
	return parseMailbox(page.Items[0].ID, page.Items[0].Doc), nil
}

// emailDomain returns the lowercase domain part of an address, or "" if the
// address has no '@'. Shared by the catch-all resolver and its CLI validation.
func emailDomain(addr string) string {
	addr = strings.ToLower(strings.TrimSpace(addr))
	_, domain, ok := strings.Cut(addr, "@")
	if !ok {
		return ""
	}
	return domain
}

func listAllMailboxes(p *Poche) ([]mailboxRecord, error) {
	data, err := p.List("mailboxes", "", 10000, 0, "", false)
	if err != nil {
		return nil, err
	}
	var page struct {
		Items []struct {
			ID  string          `json:"id"`
			Doc json.RawMessage `json:"doc"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		return nil, err
	}
	out := make([]mailboxRecord, 0, len(page.Items))
	for _, it := range page.Items {
		out = append(out, *parseMailbox(it.ID, it.Doc))
	}
	return out, nil
}

func parseMailbox(id string, raw json.RawMessage) *mailboxRecord {
	doc := map[string]any{}
	_ = json.Unmarshal(raw, &doc)
	mb := &mailboxRecord{
		ID:                  id,
		Doc:                 doc,
		Address:             stringField(doc, "address"),
		Name:                stringField(doc, "name"),
		PasswordHash:        stringField(doc, "password_hash"),
		RecoveryEmail:       stringField(doc, "recovery_email"),
		ResendAPIKey:        decryptOrWarn(stringField(doc, "resend_api_key"), "resend_api_key", stringField(doc, "address")),
		ResendWebhookSecret: decryptOrWarn(stringField(doc, "resend_webhook_secret"), "resend_webhook_secret", stringField(doc, "address")),
		WebmailURL:          stringField(doc, "webmail_url"),
		ResetFrom:           stringField(doc, "reset_from"),
		Brand:               stringField(doc, "brand"),
		CatchallDomain:      stringField(doc, "catchall_domain"),
		IsActive:            boolField(doc, "is_active"),
		RetentionMonths:     numField(doc, "retention_months"),
		MaxMessages:         int(numField(doc, "max_messages")),
		MaxBytes:            int64Field(doc, "max_bytes"),
		MessageCount:        int(numField(doc, "message_count")),
		UsedBytes:           int64Field(doc, "used_bytes"),
		CreatedAt:           int64Field(doc, "created_at"),
	}
	return mb
}

// ─── CLI commands ───────────────────────────────────────────────────────

func handleMailboxCmd() {
	if len(os.Args) < 3 {
		fail(80, "input", "usage: mailbox create|list|update|delete|alias|reset-password|encrypt-secrets", "")
	}
	sub := os.Args[2]
	switch sub {
	case "create":
		mailboxCreateCmd()
	case "list":
		mailboxListCmd()
	case "update":
		mailboxUpdateCmd()
	case "delete":
		mailboxDeleteCmd()
	case "alias":
		handleAliasCmd()
	case "reset-password":
		handleResetPasswordCmd()
	case "encrypt-secrets":
		handleEncryptSecretsCmd()
	default:
		fail(80, "input", "unknown mailbox subcommand: "+sub, "mailbox create|list|update|delete|alias|reset-password|encrypt-secrets")
	}
}

func mailboxCreateCmd() {
	fs := newFlagSet("mailbox create")
	addr := fs.String("address", "", "email address for this mailbox")
	password := fs.String("password", "", "login password ('-' = read stdin, 'env:NAME' = read env)")
	name := fs.String("name", "", "display name (defaults to address)")
	maxBytes := fs.Int64("max-bytes", 100*1024*1024, "storage cap in bytes")
	maxMessages := fs.Int("max-messages", 1000, "max message count")
	retention := fs.Float64("retention-months", 3, "months to keep unstarred mail (0 inherits MAILBOX_RETENTION_MONTHS, default 3 — it does NOT mean forever)")
	recoveryEmail := fs.String("recovery-email", "", "emergency address for password resets")
	aliasCSV := fs.String("alias", "", "comma-separated alias addresses")
	resendKey := fs.String("resend-key", "", "per-mailbox Resend API key ('-' = read stdin, 'env:NAME' = read env)")
	resendSecret := fs.String("resend-webhook-secret", "", "per-mailbox Resend webhook signing secret ('-' / 'env:NAME')")
	webmailURL := fs.String("webmail-url", "", "login URL for this tenant, used in password-reset links")
	resetFrom := fs.String("reset-from", "", "sender for reset emails (defaults to the mailbox address when it has its own Resend key)")
	brand := fs.String("brand", "", "name shown in the sidebar for this tenant (defaults to BRAND_NAME)")
	_ = fs.Parse(os.Args[3:])
	if *addr == "" || *password == "" {
		fail(80, "input", "--address and --password required", "mailbox create --address x@y.fr --password secret --max-bytes 524288000")
	}
	if *recoveryEmail == "" {
		fail(80, "input", "--recovery-email required (for password resets)", "mailbox create --address x@y.fr --password secret --recovery-email emergency@x.fr")
	}
	if *name == "" {
		*name = *addr
	}
	p := newPocheFromEnv()
	if p.Token == "" {
		fail(80, "input", "set POCHE_TOKEN", "")
	}
	if err := ensureMailboxSchema(p); err != nil {
		fail(100, "integration", "schema: "+err.Error(), "")
	}
	// check for existing
	existing, err := findMailboxByAddress(p, *addr)
	if err != nil {
		fail(100, "integration", "lookup: "+err.Error(), "")
	}
	if existing != nil {
		fail(80, "input", "mailbox already exists: "+*addr, "mailbox update "+*addr+" --max-bytes ...")
	}
	pw, err := readSecretFlag(*password)
	if err != nil {
		fail(80, "input", "--password: "+err.Error(), "")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		fail(110, "internal", "bcrypt: "+err.Error(), "")
	}
	key, err := readSecretFlag(*resendKey)
	if err != nil {
		fail(80, "input", "--resend-key: "+err.Error(), "")
	}
	secret, err := readSecretFlag(*resendSecret)
	if err != nil {
		fail(80, "input", "--resend-webhook-secret: "+err.Error(), "")
	}
	sealedKey, err := encryptSecret(key)
	if err != nil {
		fail(80, "input", "--resend-key: "+err.Error(), "")
	}
	sealedSecret, err := encryptSecret(secret)
	if err != nil {
		fail(80, "input", "--resend-webhook-secret: "+err.Error(), "")
	}
	doc := map[string]any{
		"name":                  *name,
		"address":               *addr,
		"password_hash":         string(hash),
		"recovery_email":        *recoveryEmail,
		"resend_api_key":        sealedKey,
		"resend_webhook_secret": sealedSecret,
		"webmail_url":           *webmailURL,
		"reset_from":            *resetFrom,
		"brand":                 *brand,
		"is_active":             true,
		"retention_months":      *retention,
		"max_messages":          *maxMessages,
		"max_bytes":             *maxBytes,
		"created_at":            time.Now().UnixMilli(),
	}
	raw, err := p.Create("mailboxes", doc)
	if err != nil {
		fail(100, "integration", "create: "+err.Error(), "")
	}
	var wrap map[string]any
	_ = json.Unmarshal(raw, &wrap)
	id, _ := wrap["_id"].(string)
	// add aliases
	var aliases []string
	if *aliasCSV != "" {
		for _, a := range splitCSV(*aliasCSV) {
			a = strings.ToLower(strings.TrimSpace(a))
			if a == "" || a == *addr {
				continue
			}
			if err := addAlias(p, id, a); err != nil {
				fmt.Fprintf(os.Stderr, "{\"event\":\"alias_err\",\"alias\":%q,\"err\":%q}\n", a, err.Error())
				continue
			}
			aliases = append(aliases, a)
		}
	}
	outOK(map[string]any{
		"created":          true,
		"id":               id,
		"address":          *addr,
		"name":             *name,
		"max_bytes":        *maxBytes,
		"max_messages":     *maxMessages,
		"retention_months": *retention,
		"recovery_email":   *recoveryEmail,
		"aliases":          aliases,
		"resend_key":       maskSecret(key),
		"has_resend_key":   key != "",
	})
}

func mailboxListCmd() {
	p := newPocheFromEnv()
	if p.Token == "" {
		fail(80, "input", "set POCHE_TOKEN", "")
	}
	_ = ensureAliasesSchema(p)
	mboxes, err := listAllMailboxes(p)
	if err != nil {
		fail(100, "integration", "list: "+err.Error(), "")
	}
	type mbOut struct {
		ID               string   `json:"id"`
		Address          string   `json:"address"`
		Name             string   `json:"name"`
		IsActive         bool     `json:"is_active"`
		RetentionMonths  float64  `json:"retention_months"`
		MaxMessages      int      `json:"max_messages"`
		MaxBytes         int64    `json:"max_bytes"`
		HasPassword      bool     `json:"has_password"`
		RecoveryEmail    string   `json:"recovery_email"`
		Aliases          []string `json:"aliases"`
		ResendKey        string   `json:"resend_key"`
		HasResendKey     bool     `json:"has_resend_key"`
		HasWebhookSecret bool     `json:"has_webhook_secret"`
		CatchallDomain   string   `json:"catchall_domain,omitempty"`
		CreatedAt        int64    `json:"created_at"`
	}
	out := make([]mbOut, 0, len(mboxes))
	for _, mb := range mboxes {
		aliases, _ := listAliases(p, mb.ID)
		out = append(out, mbOut{
			ID:               mb.ID,
			Address:          mb.Address,
			Name:             mb.Name,
			IsActive:         mb.IsActive,
			RetentionMonths:  mb.RetentionMonths,
			MaxMessages:      mb.MaxMessages,
			MaxBytes:         mb.MaxBytes,
			HasPassword:      mb.PasswordHash != "",
			RecoveryEmail:    mb.RecoveryEmail,
			Aliases:          aliases,
			ResendKey:        maskSecret(mb.ResendAPIKey),
			HasResendKey:     mb.ResendAPIKey != "",
			HasWebhookSecret: mb.ResendWebhookSecret != "",
			CatchallDomain:   mb.CatchallDomain,
			CreatedAt:        mb.CreatedAt,
		})
	}
	outOK(map[string]any{"mailboxes": out, "count": len(out)})
}

func mailboxUpdateCmd() {
	fs := newFlagSet("mailbox update")
	addr := fs.String("address", "", "mailbox address to update")
	password := fs.String("password", "", "new password ('-' = read stdin, 'env:NAME' = read env)")
	maxBytes := fs.Int64("max-bytes", 0, "storage cap in bytes (0 = unchanged)")
	maxMessages := fs.Int("max-messages", 0, "max message count (0 = unchanged)")
	retention := fs.Float64("retention-months", -1, "months to keep unstarred mail (negative = unchanged; 0 inherits the default, not forever)")
	active := fs.String("active", "", "set active state: true|false (empty = unchanged)")
	recoveryEmail := fs.String("recovery-email", "", "set recovery email (empty = unchanged)")
	resendKey := fs.String("resend-key", "", "set per-mailbox Resend API key ('-' = stdin, 'env:NAME' = env, 'none' = clear)")
	resendSecret := fs.String("resend-webhook-secret", "", "set per-mailbox webhook secret ('-' / 'env:NAME' / 'none')")
	webmailURL := fs.String("webmail-url", "", "set login URL used in reset links ('none' = clear)")
	resetFrom := fs.String("reset-from", "", "set sender for reset emails ('none' = clear)")
	brand := fs.String("brand", "", "set the sidebar brand for this tenant ('none' = clear)")
	catchallDomain := fs.String("catchall-domain", "", "receive mail for any address at this domain that doesn't match a real mailbox/alias ('none' = clear)")
	_ = fs.Parse(os.Args[3:])
	if *addr == "" {
		fail(80, "input", "--address required", "")
	}
	p := newPocheFromEnv()
	if p.Token == "" {
		fail(80, "input", "set POCHE_TOKEN", "")
	}
	// Ensure the schema carries every field we may write — an older store that
	// predates a field will otherwise drop it silently on update.
	if err := ensureMailboxSchema(p); err != nil {
		fail(100, "integration", "schema: "+err.Error(), "")
	}
	mb, err := findMailboxByAddress(p, *addr)
	if err != nil || mb == nil {
		// try alias lookup
		mb, err = findMailboxByAlias(p, *addr)
		if err != nil || mb == nil {
			fail(90, "not_found", "mailbox not found: "+*addr, "")
		}
	}
	doc := mb.Doc
	if *password != "" {
		pw, err := readSecretFlag(*password)
		if err != nil {
			fail(80, "input", "--password: "+err.Error(), "")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
		if err != nil {
			fail(110, "internal", "bcrypt: "+err.Error(), "")
		}
		doc["password_hash"] = string(hash)
	}
	if *maxBytes > 0 {
		doc["max_bytes"] = *maxBytes
	}
	if *maxMessages > 0 {
		doc["max_messages"] = *maxMessages
	}
	if *retention >= 0 {
		doc["retention_months"] = *retention
	}
	if *active == "true" {
		doc["is_active"] = true
	} else if *active == "false" {
		doc["is_active"] = false
	}
	if *recoveryEmail != "" {
		doc["recovery_email"] = *recoveryEmail
	}
	for _, f := range []struct {
		val, field string
	}{{*webmailURL, "webmail_url"}, {*resetFrom, "reset_from"}, {*brand, "brand"}} {
		if f.val == "none" {
			doc[f.field] = ""
		} else if f.val != "" {
			doc[f.field] = f.val
		}
	}
	if *resendKey == "none" {
		doc["resend_api_key"] = ""
	} else if *resendKey != "" {
		key, err := readSecretFlag(*resendKey)
		if err != nil {
			fail(80, "input", "--resend-key: "+err.Error(), "")
		}
		sealed, err := encryptSecret(key)
		if err != nil {
			fail(80, "input", "--resend-key: "+err.Error(), "")
		}
		doc["resend_api_key"] = sealed
	}
	if *resendSecret == "none" {
		doc["resend_webhook_secret"] = ""
	} else if *resendSecret != "" {
		secret, err := readSecretFlag(*resendSecret)
		if err != nil {
			fail(80, "input", "--resend-webhook-secret: "+err.Error(), "")
		}
		sealed, err := encryptSecret(secret)
		if err != nil {
			fail(80, "input", "--resend-webhook-secret: "+err.Error(), "")
		}
		doc["resend_webhook_secret"] = sealed
	}
	if *catchallDomain == "none" {
		doc["catchall_domain"] = ""
	} else if *catchallDomain != "" {
		domain := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(*catchallDomain), "@"))
		if strings.Contains(domain, "@") || domain == "" {
			fail(80, "input", "--catchall-domain must be a bare domain, e.g. intrane.fr (not an address)", "")
		}
		existing, err := findMailboxByCatchallDomain(p, domain)
		if err != nil {
			fail(100, "integration", "catchall-domain lookup: "+err.Error(), "")
		}
		if existing != nil && existing.ID != mb.ID {
			fail(80, "input", "domain already claimed by another mailbox: "+existing.Address, "mailbox update --address "+existing.Address+" --catchall-domain none")
		}
		doc["catchall_domain"] = domain
	}
	if _, err := p.Update("mailboxes", mb.ID, doc); err != nil {
		fail(100, "integration", "update: "+err.Error(), "")
	}
	outOK(map[string]any{
		"updated":            true,
		"address":            mb.Address,
		"has_resend_key":     stringField(doc, "resend_api_key") != "",
		"has_webhook_secret": stringField(doc, "resend_webhook_secret") != "",
		"catchall_domain":    stringField(doc, "catchall_domain"),
	})
}

func mailboxDeleteCmd() {
	fs := newFlagSet("mailbox delete")
	addr := fs.String("address", "", "mailbox address to delete")
	force := fs.Bool("force", false, "skip confirmation")
	_ = fs.Parse(os.Args[3:])
	if *addr == "" {
		fail(80, "input", "--address required", "")
	}
	if !*force {
		fail(80, "input", "use --force to confirm deletion", "mailbox delete --address "+*addr+" --force")
	}
	p := newPocheFromEnv()
	if p.Token == "" {
		fail(80, "input", "set POCHE_TOKEN", "")
	}
	mb, err := findMailboxByAddress(p, *addr)
	if err != nil || mb == nil {
		fail(90, "not_found", "mailbox not found: "+*addr, "")
	}
	// delete messages belonging to this mailbox
	msgs, _, _, _ := loadMailboxMessages(p, mb.ID)
	for _, m := range msgs {
		_ = deleteMessageWithLinks(p, m.id)
	}
	// delete sessions
	_ = deleteSessionsByMailbox(p, mb.ID)
	// delete aliases
	_ = deleteAliasesByMailbox(p, mb.ID)
	// delete mailbox
	if err := p.Delete("mailboxes", mb.ID); err != nil {
		fail(100, "integration", "delete: "+err.Error(), "")
	}
	outOK(map[string]any{"deleted": true, "address": *addr, "messages_removed": len(msgs)})
}

// messageSizeBytes returns the counted byte size for a message.
// It intentionally mirrors loadMailboxMessages: body_text + body_html.
// Attachments are not counted here (same as the previous implementation).
func messageSizeBytes(doc map[string]any) int64 {
	size := int64(0)
	if s, ok := doc["body_text"].(string); ok {
		size += int64(len(s))
	}
	if s, ok := doc["body_html"].(string); ok {
		size += int64(len(s))
	}
	return size
}

// updateMailboxUsage applies a delta to the stored message_count and used_bytes
// on the mailbox doc. It is a best-effort write; errors are logged to stderr
// but not returned, because quota/usage should not block message operations.
func updateMailboxUsage(p *Poche, mailboxID string, deltaCount int, deltaBytes int64) {
	raw, err := p.Get("mailboxes", mailboxID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "{\"event\":\"usage_update_err\",\"mailbox\":%q,\"err\":%q}\n", mailboxID, err.Error())
		return
	}
	// p.Get returns {"id":…,"doc":{…}} — parsing the envelope makes every
	// counter read as 0, so each update overwrote the total with its own
	// delta instead of adding to it.
	var wrap struct {
		Doc json.RawMessage `json:"doc"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil || len(wrap.Doc) == 0 {
		fmt.Fprintf(os.Stderr, "{\"event\":\"usage_update_err\",\"mailbox\":%q,\"err\":\"unreadable doc\"}\n", mailboxID)
		return
	}
	mb := parseMailbox(mailboxID, wrap.Doc)
	newCount := mb.MessageCount + deltaCount
	if newCount < 0 {
		newCount = 0
	}
	newBytes := mb.UsedBytes + deltaBytes
	if newBytes < 0 {
		newBytes = 0
	}
	doc := mb.Doc
	doc["message_count"] = newCount
	doc["used_bytes"] = newBytes
	if _, err := p.Update("mailboxes", mailboxID, doc); err != nil {
		fmt.Fprintf(os.Stderr, "{\"event\":\"usage_update_err\",\"mailbox\":%q,\"err\":%q}\n", mailboxID, err.Error())
	}
}
