package main

import (
	"fmt"
	"os"
)

const Version = "0.3.0"

func main() {
	if len(os.Args) < 2 {
		printHelp()
		os.Exit(80)
	}
	switch os.Args[1] {
	case "serve", "start":
		handleServe()
	case "seed":
		handleSeedCmd()
	case "sync":
		handleSyncCmd()
	case "cleanup":
		handleCleanupCmd()
	case "mailbox":
		handleMailboxCmd()
	case "smtp":
		handleSMTPCmd()
	case "seed-mailbox":
		handleSeedMailboxCmd()
	case "reply":
		handleReplyCmd()
	case "send", "compose":
		handleComposeCmd()
	case "list":
		handleListCmd()
	case "read":
		handleReadCmd()
	case "guide":
		handleGuide()
	case "stop":
		stopDaemon()
	case "status":
		checkDaemonStatus()
	case "version":
		outOK(map[string]any{"version": Version, "tool": "poche-resend-webmail"})
	case "help", "--help", "-h":
		printHelp()
	default:
		fail(80, "input", "unknown command: "+os.Args[1], "poche-resend-webmail help")
	}
}

func handleServe() {
	fs := newFlagSet("serve")
	port := fs.Int("port", 3090, "UI / BFF port")
	daemon := fs.Bool("daemon", false, "run in background")
	_ = fs.Parse(os.Args[2:])
	ensureWebmailToken()
	if *daemon {
		startDaemon(*port)
		return
	}
	startServer(*port)
}

func handleSeedCmd() {
	fs := newFlagSet("seed")
	count := fs.Int("count", 100, "mock messages for offline UI")
	_ = fs.Parse(os.Args[2:])
	handleSeed(*count)
}

func handleSyncCmd() {
	n, err := syncInbound(0)
	if err != nil {
		fail(100, "integration", err.Error(), "set RESEND_API_KEY and POCHE_TOKEN")
	}
	outOK(map[string]any{"synced": n})
}

func handleCleanupCmd() {
	summary, err := cleanup()
	if err != nil {
		fail(100, "integration", err.Error(), "set POCHE_TOKEN")
	}
	outOK(summary)
}

func handleReplyCmd() {
	if len(os.Args) < 4 {
		fail(80, "input", "usage: reply <message_id> <text>", "")
	}
	id, text := os.Args[2], os.Args[3]
	from := ""
	if len(os.Args) > 4 {
		from = os.Args[4]
	}
	data, err := replyMessage(id, text, from, "", "")
	if err != nil {
		fail(100, "integration", err.Error(), "")
	}
	outOK(data)
}

func handleListCmd() {
	fs := newFlagSet("list")
	limit := fs.Int("limit", 20, "page size")
	_ = fs.Parse(os.Args[2:])
	p := newPocheFromEnv()
	data, err := p.ListLinked("messages", "", nil, []string{linkArchive}, *limit, 0, "created_at", true)
	if err != nil {
		fail(100, "integration", err.Error(), "")
	}
	fmt.Println(string(data))
	os.Exit(0)
}

func handleReadCmd() {
	if len(os.Args) < 3 {
		fail(80, "input", "usage: read <message_id>", "")
	}
	p := newPocheFromEnv()
	data, err := p.Get("messages", os.Args[2])
	if err != nil {
		fail(100, "integration", err.Error(), "")
	}
	fmt.Println(string(data))
	os.Exit(0)
}

func printHelp() {
	fmt.Fprintln(os.Stderr, `poche-resend-webmail — OSS self-hosted Resend webmail over poche (MIT)

Usage:
  poche-resend-webmail serve [-port 3090] [-daemon]
  poche-resend-webmail seed    [-count 100]
  poche-resend-webmail sync
  poche-resend-webmail cleanup
  poche-resend-webmail mailbox create|list|update|delete|alias|reset-password
  poche-resend-webmail smtp    [-port 25]          (minimal SMTP receiver for inbound)
  poche-resend-webmail seed-mailbox --address X [--count 10 --size-kb 200]
  poche-resend-webmail list    [-limit 20]
  poche-resend-webmail read    <id>
  poche-resend-webmail reply   <id> <text> [from]
  poche-resend-webmail send    --to a@b.fr --subject S --text T [--from x] [--cc] [--bcc]
  poche-resend-webmail guide | stop | status | version | help

Env:
  WEBMAIL_TOKEN            Bearer for UI/API (generated if unset on serve; legacy admin)
  ADMIN_TOKEN              Bearer for admin API (mailbox management; separate from WEBMAIL_TOKEN)
  POCHE_URL                default http://127.0.0.1:17781
  POCHE_TOKEN              poche admin/user token
  RESEND_API_KEY           required for sync/reply/send/webhook fetch
  RESEND_BASE_URL          override Resend API host (default https://api.resend.com)
  RESEND_WEBHOOK_SECRET    verify webhooks (empty = accept, dev only)
  MAIL_FROM_ALLOWLIST      comma suffixes, default @intrane.fr
  MAILBOX_RETENTION_MONTHS default 3 (fallback if mailbox field missing)
  MAILBOX_MAX_MESSAGES     default 1000
  MAILBOX_MAX_BYTES        default 104857600 (100 MB)
  AUTO_CLEANUP             run cleanup after sync, default 0`)
}
