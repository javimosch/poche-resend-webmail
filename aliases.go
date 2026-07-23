package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ─── aliases schema + CRUD ──────────────────────────────────────────────

// aliases collection: one row per alias address on a mailbox.
// Login + ingest routing check both mailboxes.address and aliases.address.
const aliasesSchema = "mailbox_id:string!required!ref=mailboxes,address:string!required!unique"

func ensureAliasesSchema(p *Poche) error {
	if err := p.AdminSchema("aliases", aliasesSchema); err != nil {
		return fmt.Errorf("aliases: %w", err)
	}
	if err := p.AdminExpose("aliases", "read,create,delete"); err != nil {
		return err
	}
	_ = p.AdminIndex("aliases", "mailbox_id", "")
	_ = p.AdminIndex("aliases", "address", "")
	return nil
}

// listAliases returns all alias addresses for a mailbox.
func listAliases(p *Poche, mailboxID string) ([]string, error) {
	data, err := p.List("aliases", "mailbox_id="+mailboxID, 1000, 0, "", false)
	if err != nil {
		return nil, err
	}
	var page struct {
		Items []struct {
			Doc json.RawMessage `json:"doc"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(page.Items))
	for _, it := range page.Items {
		doc := map[string]any{}
		_ = json.Unmarshal(it.Doc, &doc)
		addr := stringField(doc, "address")
		if addr != "" {
			out = append(out, addr)
		}
	}
	return out, nil
}

// findMailboxByAlias looks up a mailbox via an alias address.
func findMailboxByAlias(p *Poche, addr string) (*mailboxRecord, error) {
	data, err := p.List("aliases", "address="+addr, 1, 0, "", false)
	if err != nil {
		return nil, err
	}
	var page struct {
		Items []struct {
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
	doc := map[string]any{}
	_ = json.Unmarshal(page.Items[0].Doc, &doc)
	mbID := stringField(doc, "mailbox_id")
	if mbID == "" {
		return nil, nil
	}
	return getMailboxByID(p, mbID)
}

// addAlias creates an alias for a mailbox.
func addAlias(p *Poche, mailboxID, addr string) error {
	addr = strings.ToLower(strings.TrimSpace(addr))
	_, err := p.Create("aliases", map[string]any{
		"mailbox_id": mailboxID,
		"address":    addr,
	})
	return err
}

// removeAlias deletes an alias by address.
func removeAlias(p *Poche, addr string) error {
	addr = strings.ToLower(strings.TrimSpace(addr))
	data, err := p.List("aliases", "address="+addr, 1, 0, "", false)
	if err != nil {
		return err
	}
	var page struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		return err
	}
	if page.Total == 0 {
		return fmt.Errorf("alias not found: %s", addr)
	}
	return p.Delete("aliases", page.Items[0].ID)
}

// deleteAliasesByMailbox removes all aliases for a mailbox (used on delete).
func deleteAliasesByMailbox(p *Poche, mailboxID string) error {
	data, err := p.List("aliases", "mailbox_id="+mailboxID, 1000, 0, "", false)
	if err != nil {
		return err
	}
	var page struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	_ = json.Unmarshal(data, &page)
	for _, it := range page.Items {
		_ = p.Delete("aliases", it.ID)
	}
	return nil
}

// mailboxAllAddresses returns the primary address + all aliases.
func mailboxAllAddresses(p *Poche, mb *mailboxRecord) []string {
	out := []string{mb.Address}
	aliases, _ := listAliases(p, mb.ID)
	out = append(out, aliases...)
	return out
}

// ─── CLI subcommands ────────────────────────────────────────────────────

func handleAliasCmd() {
	if len(os.Args) < 4 {
		fail(80, "input", "usage: mailbox alias add|remove|list", "")
	}
	sub := os.Args[3]
	switch sub {
	case "add":
		aliasAddCmd()
	case "remove":
		aliasRemoveCmd()
	case "list":
		aliasListCmd()
	default:
		fail(80, "input", "unknown alias subcommand: "+sub, "mailbox alias add|remove|list")
	}
}

func aliasAddCmd() {
	fs := newFlagSet("mailbox alias add")
	mbAddr := fs.String("mailbox", "", "primary address of the mailbox")
	alias := fs.String("alias", "", "alias address to add")
	_ = fs.Parse(os.Args[4:])
	if *mbAddr == "" || *alias == "" {
		fail(80, "input", "--mailbox and --alias required", "")
	}
	p := newPocheFromEnv()
	if p.Token == "" {
		fail(80, "input", "set POCHE_TOKEN", "")
	}
	_ = ensureAliasesSchema(p)
	mb, err := findMailboxByAddress(p, *mbAddr)
	if err != nil || mb == nil {
		fail(90, "not_found", "mailbox not found: "+*mbAddr, "")
	}
	if err := addAlias(p, mb.ID, *alias); err != nil {
		fail(100, "integration", "add alias: "+err.Error(), "")
	}
	outOK(map[string]any{"added": true, "mailbox": *mbAddr, "alias": *alias})
}

func aliasRemoveCmd() {
	fs := newFlagSet("mailbox alias remove")
	alias := fs.String("alias", "", "alias address to remove")
	_ = fs.Parse(os.Args[4:])
	if *alias == "" {
		fail(80, "input", "--alias required", "")
	}
	p := newPocheFromEnv()
	if p.Token == "" {
		fail(80, "input", "set POCHE_TOKEN", "")
	}
	_ = ensureAliasesSchema(p)
	if err := removeAlias(p, *alias); err != nil {
		fail(100, "integration", "remove alias: "+err.Error(), "")
	}
	outOK(map[string]any{"removed": true, "alias": *alias})
}

func aliasListCmd() {
	fs := newFlagSet("mailbox alias list")
	mbAddr := fs.String("mailbox", "", "primary address of the mailbox")
	_ = fs.Parse(os.Args[4:])
	if *mbAddr == "" {
		fail(80, "input", "--mailbox required", "")
	}
	p := newPocheFromEnv()
	if p.Token == "" {
		fail(80, "input", "set POCHE_TOKEN", "")
	}
	mb, err := findMailboxByAddress(p, *mbAddr)
	if err != nil || mb == nil {
		fail(90, "not_found", "mailbox not found: "+*mbAddr, "")
	}
	aliases, err := listAliases(p, mb.ID)
	if err != nil {
		fail(100, "integration", "list aliases: "+err.Error(), "")
	}
	outOK(map[string]any{"mailbox": *mbAddr, "primary": mb.Address, "aliases": aliases, "count": len(aliases)})
}
