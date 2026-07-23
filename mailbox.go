package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// ─── schema ─────────────────────────────────────────────────────────────

// mailboxSchemaExtra are the fields added to the existing mailboxes collection
// for multi-tenant auth + provisioning. The base fields (name, address,
// retention_months, max_messages, max_bytes) stay as-is.
const mailboxSchemaExtra = "password_hash:string,is_active:bool,created_at:int"

// sessions collection: per-login session tokens scoped to a mailbox.
const sessionsSchema = "token:string!required!unique,mailbox_id:string!required!ref=mailboxes,expires_at:int"

func ensureMailboxSchema(p *Poche) error {
	if err := p.AdminSchema("mailboxes", "name:string!required!unique,address:string!required!unique,retention_months:float,max_messages:int,max_bytes:int,password_hash:string,is_active:bool,created_at:int"); err != nil {
		return fmt.Errorf("mailboxes: %w", err)
	}
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
	return nil
}

// ─── mailbox CRUD ───────────────────────────────────────────────────────

type mailboxRecord struct {
	ID             string
	Address        string
	Name           string
	PasswordHash   string
	IsActive       bool
	RetentionMonths float64
	MaxMessages    int
	MaxBytes       int64
	CreatedAt      int64
	Doc            map[string]any
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
		ID:              id,
		Doc:             doc,
		Address:         stringField(doc, "address"),
		Name:            stringField(doc, "name"),
		PasswordHash:    stringField(doc, "password_hash"),
		IsActive:        boolField(doc, "is_active"),
		RetentionMonths: numField(doc, "retention_months"),
		MaxMessages:     int(numField(doc, "max_messages")),
		MaxBytes:        int64Field(doc, "max_bytes"),
		CreatedAt:       int64Field(doc, "created_at"),
	}
	return mb
}

// ─── CLI commands ───────────────────────────────────────────────────────

func handleMailboxCmd() {
	if len(os.Args) < 3 {
		fail(80, "input", "usage: mailbox create|list|update|delete", "")
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
	default:
		fail(80, "input", "unknown mailbox subcommand: "+sub, "mailbox create|list|update|delete")
	}
}

func mailboxCreateCmd() {
	fs := newFlagSet("mailbox create")
	addr := fs.String("address", "", "email address for this mailbox")
	password := fs.String("password", "", "login password")
	name := fs.String("name", "", "display name (defaults to address)")
	maxBytes := fs.Int64("max-bytes", 100*1024*1024, "storage cap in bytes")
	maxMessages := fs.Int("max-messages", 1000, "max message count")
	retention := fs.Float64("retention-months", 3, "retention (0 = keep forever)")
	_ = fs.Parse(os.Args[3:])
	if *addr == "" || *password == "" {
		fail(80, "input", "--address and --password required", "mailbox create --address x@y.fr --password secret --max-bytes 524288000")
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
	hash, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
	if err != nil {
		fail(110, "internal", "bcrypt: "+err.Error(), "")
	}
	doc := map[string]any{
		"name":             *name,
		"address":          *addr,
		"password_hash":    string(hash),
		"is_active":        true,
		"retention_months": *retention,
		"max_messages":     *maxMessages,
		"max_bytes":        *maxBytes,
		"created_at":       time.Now().UnixMilli(),
	}
	raw, err := p.Create("mailboxes", doc)
	if err != nil {
		fail(100, "integration", "create: "+err.Error(), "")
	}
	var wrap map[string]any
	_ = json.Unmarshal(raw, &wrap)
	id, _ := wrap["_id"].(string)
	outOK(map[string]any{
		"created":       true,
		"id":            id,
		"address":       *addr,
		"name":          *name,
		"max_bytes":     *maxBytes,
		"max_messages":  *maxMessages,
		"retention_months": *retention,
	})
}

func mailboxListCmd() {
	p := newPocheFromEnv()
	if p.Token == "" {
		fail(80, "input", "set POCHE_TOKEN", "")
	}
	mboxes, err := listAllMailboxes(p)
	if err != nil {
		fail(100, "integration", "list: "+err.Error(), "")
	}
	type mbOut struct {
		ID              string `json:"id"`
		Address         string `json:"address"`
		Name            string `json:"name"`
		IsActive        bool   `json:"is_active"`
		RetentionMonths float64 `json:"retention_months"`
		MaxMessages     int    `json:"max_messages"`
		MaxBytes        int64  `json:"max_bytes"`
		HasPassword     bool   `json:"has_password"`
		CreatedAt       int64  `json:"created_at"`
	}
	out := make([]mbOut, 0, len(mboxes))
	for _, mb := range mboxes {
		out = append(out, mbOut{
			ID:              mb.ID,
			Address:         mb.Address,
			Name:            mb.Name,
			IsActive:        mb.IsActive,
			RetentionMonths: mb.RetentionMonths,
			MaxMessages:     mb.MaxMessages,
			MaxBytes:        mb.MaxBytes,
			HasPassword:     mb.PasswordHash != "",
			CreatedAt:       mb.CreatedAt,
		})
	}
	outOK(map[string]any{"mailboxes": out, "count": len(out)})
}

func mailboxUpdateCmd() {
	fs := newFlagSet("mailbox update")
	addr := fs.String("address", "", "mailbox address to update")
	password := fs.String("password", "", "new password (if changing)")
	maxBytes := fs.Int64("max-bytes", 0, "storage cap in bytes (0 = unchanged)")
	maxMessages := fs.Int("max-messages", 0, "max message count (0 = unchanged)")
	retention := fs.Float64("retention-months", -1, "retention (negative = unchanged)")
	active := fs.String("active", "", "set active state: true|false (empty = unchanged)")
	_ = fs.Parse(os.Args[3:])
	if *addr == "" {
		fail(80, "input", "--address required", "")
	}
	p := newPocheFromEnv()
	if p.Token == "" {
		fail(80, "input", "set POCHE_TOKEN", "")
	}
	mb, err := findMailboxByAddress(p, *addr)
	if err != nil || mb == nil {
		fail(90, "not_found", "mailbox not found: "+*addr, "")
	}
	doc := mb.Doc
	if *password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(*password), bcrypt.DefaultCost)
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
	if _, err := p.Update("mailboxes", mb.ID, doc); err != nil {
		fail(100, "integration", "update: "+err.Error(), "")
	}
	outOK(map[string]any{"updated": true, "address": *addr})
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
	// delete mailbox
	if err := p.Delete("mailboxes", mb.ID); err != nil {
		fail(100, "integration", "delete: "+err.Error(), "")
	}
	outOK(map[string]any{"deleted": true, "address": *addr, "messages_removed": len(msgs)})
}
