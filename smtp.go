package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/mail"
	"os"
	"strings"
	"time"
)

// ─── minimal SMTP receiver ──────────────────────────────────────────────
//
// Listens on port 25 (or configurable), accepts emails for any address,
// routes them to the matching mailbox via the same upsertInbound path.
// No auth, no TLS — receiving only. Designed to sit behind an MX record.

type smtpServer struct {
	port int
}

func startSMTPServer(port int) {
	s := &smtpServer{port: port}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		fmt.Fprintf(os.Stderr, "{\"event\":\"smtp_listen_err\",\"port\":%d,\"err\":%q}\n", port, err.Error())
		fail(100, "integration", "smtp listen: "+err.Error(), "")
	}
	fmt.Fprintf(os.Stderr, "{\"event\":\"smtp_listening\",\"port\":%d}\n", port)
	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go s.handle(conn)
	}
}

func (s *smtpServer) handle(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(30 * time.Second))
	w := bufio.NewWriter(conn)
	r := bufio.NewReader(conn)

	send := func(code int, msg string) {
		fmt.Fprintf(w, "%d %s\r\n", code, msg)
		w.Flush()
	}

	send(220, "poche-resend-webmail ESMTP ready")

	var from string
	var rcpts []string

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		upper := strings.ToUpper(line)

		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			send(250, "poche-resend-webmail")
		case strings.HasPrefix(upper, "MAIL FROM:"):
			from = parseAddr(line[10:])
			send(250, "OK")
		case strings.HasPrefix(upper, "RCPT TO:"):
			rcpts = append(rcpts, parseAddr(line[8:]))
			send(250, "OK")
		case strings.HasPrefix(upper, "NOOP"):
			send(250, "OK")
		case strings.HasPrefix(upper, "RSET"):
			from = ""
			rcpts = nil
			send(250, "OK")
		case strings.HasPrefix(upper, "QUIT"):
			send(221, "Bye")
			return
		case strings.HasPrefix(upper, "DATA"):
			send(354, "Start mail input; end with <CRLF>.<CRLF>")
			// read until \r\n.\r\n
			var raw strings.Builder
			for {
				dline, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimRight(dline, "\r\n") == "." {
					break
				}
				// handle dot-stuffing
				if strings.HasPrefix(dline, "..") {
					dline = dline[1:]
				}
				raw.WriteString(dline)
			}
			// process the email
			for _, rcpt := range rcpts {
				s.processEmail(from, rcpt, raw.String())
			}
			send(250, "OK queued")
			from = ""
			rcpts = nil
		default:
			send(500, "Command not recognized")
		}
	}
}

func (s *smtpServer) processEmail(from, rcpt, rawEmail string) {
	// parse headers
	msg, err := mail.ReadMessage(strings.NewReader(rawEmail))
	if err != nil {
		fmt.Fprintf(os.Stderr, "{\"event\":\"smtp_parse_err\",\"from\":%q,\"rcpt\":%q,\"err\":%q}\n", from, rcpt, err.Error())
		return
	}
	subject := msg.Header.Get("Subject")
	toHeader := msg.Header.Get("To")
	ccHeader := msg.Header.Get("Cc")
	dateStr := msg.Header.Get("Date")
	messageID := msg.Header.Get("Message-ID")
	if messageID == "" {
		messageID = fmt.Sprintf("%d.smtp@poche", time.Now().UnixNano())
	}
	// read body
	body, _ := io.ReadAll(msg.Body)
	bodyStr := string(body)
	// determine text vs html
	textPart := ""
	htmlPart := ""
	if strings.Contains(strings.ToLower(msg.Header.Get("Content-Type")), "text/html") {
		htmlPart = bodyStr
	} else {
		textPart = bodyStr
	}

	// build the payload (same shape as Resend webhook)
	payload := map[string]any{
		"id":         messageID,
		"from":       from,
		"to":         []string{rcpt},
		"subject":    subject,
		"text":       textPart,
		"html":       htmlPart,
		"received_at": time.Now().UTC().Format(time.RFC3339),
	}
	if toHeader != "" {
		payload["to_header"] = toHeader
	}
	if ccHeader != "" {
		payload["cc"] = ccHeader
	}
	if dateStr != "" {
		payload["date"] = dateStr
	}

	// route to mailbox
	p := newPocheFromEnv()
	if p.Token == "" {
		fmt.Fprintf(os.Stderr, "{\"event\":\"smtp_no_poche\",\"rcpt\":%q}\n", rcpt)
		return
	}
	_ = ensureSchema(p)
	_ = ensureMailboxSchema(p)
	_ = ensureTags(p)

	mbID, err := findOrCreateMailboxForAddress(p, rcpt)
	if err != nil || mbID == "" {
		fmt.Fprintf(os.Stderr, "{\"event\":\"smtp_no_mailbox\",\"rcpt\":%q}\n", rcpt)
		return
	}

	// quota check
	msgSize := int64(len(bodyStr))
	if err := mailboxAllowsIngest(p, mbID, msgSize); err != nil {
		fmt.Fprintf(os.Stderr, "{\"event\":\"smtp_quota\",\"rcpt\":%q,\"err\":%q}\n", rcpt, err.Error())
		return
	}

	created, err := upsertInbound(p, mbID, payload)
	if err != nil {
		fmt.Fprintf(os.Stderr, "{\"event\":\"smtp_store_err\",\"rcpt\":%q,\"err\":%q}\n", rcpt, err.Error())
		return
	}
	fmt.Fprintf(os.Stderr, "{\"event\":\"smtp_received\",\"from\":%q,\"rcpt\":%q,\"created\":%v,\"size\":%d}\n", from, rcpt, created, msgSize)
}

func parseAddr(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "<") && strings.HasSuffix(s, ">") {
		return s[1 : len(s)-1]
	}
	return s
}

// ─── CLI: smtp subcommand ───────────────────────────────────────────────

func handleSMTPCmd() {
	fs := newFlagSet("smtp")
	port := fs.Int("port", 25, "SMTP port to listen on")
	_ = fs.Parse(os.Args[2:])
	// ensure schema before starting
	p := newPocheFromEnv()
	if p.Token != "" {
		_ = ensureSchema(p)
		_ = ensureMailboxSchema(p)
		_ = ensureTags(p)
	}
	startSMTPServer(*port)
}

// ─── API: GET /api/mailbox/usage ────────────────────────────────────────

func handleMailboxUsageAPI(w http.ResponseWriter, r *http.Request) {
	mbID := authMailboxID(r)
	isAdmin := authIsAdmin(r)
	p := newPocheFromEnv()
	if p.Token == "" {
		writeJSON(w, 500, map[string]any{"ok": false, "error": "POCHE_TOKEN missing"})
		return
	}
	if mbID == "" && !isAdmin {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "no mailbox context"})
		return
	}
	// for admin without mailbox, aggregate all mailboxes
	if isAdmin && mbID == "" {
		mboxes, _ := listAllMailboxes(p)
		type usageOut struct {
			Address      string `json:"address"`
			UsedBytes    int64  `json:"used_bytes"`
			MaxBytes     int64  `json:"max_bytes"`
			Percent      int    `json:"percent"`
			MessageCount int    `json:"message_count"`
		}
		out := make([]usageOut, 0, len(mboxes))
		var totalUsed, totalMax int64
		for _, mb := range mboxes {
			count, used, _ := mailboxUsage(p, mb.ID)
			maxB := mb.MaxBytes
			if maxB == 0 {
				maxB = envInt64("MAILBOX_MAX_BYTES", defaultMaxBytes)
			}
			pct := 0
			if maxB > 0 {
				pct = int(used * 100 / maxB)
			}
			out = append(out, usageOut{mb.Address, used, maxB, pct, count})
			totalUsed += used
			totalMax += maxB
		}
		writeJSON(w, 200, map[string]any{"ok": true, "data": map[string]any{"mailboxes": out, "total_used": totalUsed, "total_max": totalMax}})
		return
	}
	// single mailbox
	count, used, err := mailboxUsage(p, mbID)
	if err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	mb, _ := getMailboxByID(p, mbID)
	maxB := int64(0)
	if mb != nil {
		maxB = mb.MaxBytes
	}
	if maxB == 0 {
		maxB = envInt64("MAILBOX_MAX_BYTES", defaultMaxBytes)
	}
	pct := 0.0
	if maxB > 0 {
		pct = float64(used) * 100.0 / float64(maxB)
	}
	writeJSON(w, 200, map[string]any{
		"ok": true,
		"data": map[string]any{
			"used_bytes":    used,
			"max_bytes":     maxB,
			"percent":       pct,
			"message_count": count,
		},
	})
}

// ─── CLI: seed-mailbox ──────────────────────────────────────────────────

func handleSeedMailboxCmd() {
	fs := newFlagSet("seed-mailbox")
	addr := fs.String("address", "", "mailbox address to seed")
	count := fs.Int("count", 10, "number of emails to seed")
	sizeKB := fs.Int("size-kb", 200, "approx size of each email body in KB")
	_ = fs.Parse(os.Args[2:])
	if *addr == "" {
		fail(80, "input", "--address required", "seed-mailbox --address contact@x.fr --count 10 --size-kb 200")
	}
	p := newPocheFromEnv()
	if p.Token == "" {
		fail(80, "input", "set POCHE_TOKEN", "")
	}
	_ = ensureSchema(p)
	_ = ensureMailboxSchema(p)
	_ = ensureTags(p)
	mb, err := findMailboxByAddress(p, *addr)
	if err != nil || mb == nil {
		mb, err = findMailboxByAlias(p, *addr)
		if err != nil || mb == nil {
			fail(90, "not_found", "mailbox not found: "+*addr, "")
		}
	}
	// generate padding content
	padding := strings.Repeat("La Cure en Bauges — email de test. ", (*sizeKB*1024)/40)
	totalBytes := int64(0)
	for i := 1; i <= *count; i++ {
		subject := fmt.Sprintf("Email de test #%d — La Cure en Bauges", i)
		from := []string{"noreply@expediteur.fr", "contact@fournisseur.com", "hello@partenaire.fr"}[i%3]
		body := fmt.Sprintf("Bonjour,\n\nCeci est l'email de test #%d pour le mailbox %s.\n\n%s\n\nCordialement,\nService test", i, mb.Address, padding)
		payload := map[string]any{
			"id":         fmt.Sprintf("seed-%d-%d@poche", i, time.Now().UnixNano()),
			"from":       from,
			"to":         []string{mb.Address},
			"subject":    subject,
			"text":       body,
			"html":       "",
			"received_at": time.Now().UTC().Format(time.RFC3339),
		}
		_, err := upsertInbound(p, mb.ID, payload)
		if err != nil {
			fmt.Fprintf(os.Stderr, "{\"event\":\"seed_err\",\"i\":%d,\"err\":%q}\n", i, err.Error())
			continue
		}
		totalBytes += int64(len(body))
		fmt.Fprintf(os.Stderr, "{\"event\":\"seeded\",\"i\":%d,\"size\":%d}\n", i, len(body))
	}
	// verify usage
	count2, used, _ := mailboxUsage(p, mb.ID)
	_ = json.Marshal // keep import
	outOK(map[string]any{
		"seeded":        *count,
		"address":       mb.Address,
		"total_bytes":   totalBytes,
		"actual_count":  count2,
		"actual_bytes":  used,
		"max_bytes":     mb.MaxBytes,
		"percent":       int(used * 100 / mb.MaxBytes),
	})
}
