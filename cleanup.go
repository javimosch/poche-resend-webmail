package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"
)

const (
	defaultRetentionMonths = 3.0
	defaultMaxMessages     = 1000
	defaultMaxBytes        = 100 * 1024 * 1024
)

type cleanupResult struct {
	Deleted    int                `json:"deleted"`
	ByMailbox []mailboxCleanup  `json:"by_mailbox"`
}

type mailboxCleanup struct {
	Address string `json:"address"`
	Deleted int    `json:"deleted"`
	Reason  string `json:"reason"`
}

func cleanup() (*cleanupResult, error) {
	p := newPocheFromEnv()
	if p.Token == "" {
		return nil, fmt.Errorf("POCHE_TOKEN missing")
	}

	mboxes, err := listMailboxes(p)
	if err != nil {
		return nil, err
	}

	res := &cleanupResult{ByMailbox: []mailboxCleanup{}}
	for _, mb := range mboxes {
		n, reason, err := cleanupMailbox(p, mb)
		if err != nil {
			logCleanupEvent("mailbox_err", map[string]any{"address": mb.Address, "err": err.Error()})
			continue
		}
		res.Deleted += n
		res.ByMailbox = append(res.ByMailbox, mailboxCleanup{
			Address: mb.Address,
			Deleted: n,
			Reason:  reason,
		})
	}
	return res, nil
}

type mailbox struct {
	ID      string
	Address string
	Doc     map[string]any
}

func listMailboxes(p *Poche) ([]mailbox, error) {
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
	out := make([]mailbox, 0, len(page.Items))
	for _, it := range page.Items {
		doc := map[string]any{}
		_ = json.Unmarshal(it.Doc, &doc)
		addr, _ := doc["address"].(string)
		if addr == "" {
			addr = it.ID
		}
		out = append(out, mailbox{ID: it.ID, Address: addr, Doc: doc})
	}
	return out, nil
}

func cleanupMailbox(p *Poche, mb mailbox) (int, string, error) {
	retention := mailboxFloat(mb.Doc, "retention_months", envFloat("MAILBOX_RETENTION_MONTHS", defaultRetentionMonths))
	maxMsg := mailboxInt(mb.Doc, "max_messages", envInt("MAILBOX_MAX_MESSAGES", defaultMaxMessages))
	maxBytes := mailboxInt64(mb.Doc, "max_bytes", envInt64("MAILBOX_MAX_BYTES", defaultMaxBytes))

	cutoff := time.Now().UnixMilli() - int64(retention*30*24*60*60*1000)

	msgs, totalCount, totalBytes, err := loadMailboxMessages(p, mb.ID)
	if err != nil {
		return 0, "", err
	}

	deleted := 0
	reason := ""
	for _, m := range msgs {
		del := false
		r := ""
		if m.createdAt < cutoff {
			del = true
			r = "retention"
		} else if totalCount > maxMsg {
			del = true
			r = "count"
		} else if totalBytes > maxBytes {
			del = true
			r = "bytes"
		}
		if !del {
			break
		}
		if err := deleteMessageWithLinks(p, m.id); err != nil {
			logCleanupEvent("delete_err", map[string]any{"message_id": m.id, "err": err.Error()})
			continue
		}
		totalCount--
		totalBytes -= m.size
		deleted++
		if reason == "" {
			reason = r
		}
	}
	return deleted, reason, nil
}

type msgSum struct {
	id        string
	createdAt int64
	size      int64
}

func loadMailboxMessages(p *Poche, mailboxID string) ([]msgSum, int, int64, error) {
	var all []msgSum
	totalCount := 0
	totalBytes := int64(0)
	offset := 0
	for {
		data, err := p.List("messages", "mailbox_id="+mailboxID, 10000, offset, "created_at", false)
		if err != nil {
			return nil, 0, 0, err
		}
		var page struct {
			Items []struct {
				ID  string          `json:"id"`
				Doc json.RawMessage `json:"doc"`
			} `json:"items"`
			Total int `json:"total"`
		}
		if err := json.Unmarshal(data, &page); err != nil {
			return nil, 0, 0, err
		}
		for _, it := range page.Items {
			doc := map[string]any{}
			_ = json.Unmarshal(it.Doc, &doc)
			created := int64Field(doc, "created_at")
			if created == 0 {
				created = time.Now().UnixMilli()
			}
			size := int64(len(stringField(doc, "body_text")) + len(stringField(doc, "body_html")))
			totalCount++
			totalBytes += size
			if !boolField(doc, "starred") {
				all = append(all, msgSum{id: it.ID, createdAt: created, size: size})
			}
		}
		offset += len(page.Items)
		if offset >= page.Total || len(page.Items) == 0 {
			break
		}
	}
	return all, totalCount, totalBytes, nil
}

func deleteMessageWithLinks(p *Poche, messageID string) error {
	// Load the message before deleting it so we can update the mailbox usage
	// counters by the exact byte delta.
	var mailboxID string
	var deltaBytes int64
	if raw, err := p.Get("messages", messageID); err == nil {
		doc := map[string]any{}
		_ = json.Unmarshal(raw, &doc)
		if s, ok := doc["mailbox_id"].(string); ok {
			mailboxID = s
		}
		deltaBytes = messageSizeBytes(doc)
	}
	for _, coll := range []string{"message_tags", "attachments"} {
		if err := deleteByMessageID(p, coll, messageID); err != nil {
			logCleanupEvent("link_delete_err", map[string]any{"collection": coll, "message_id": messageID, "err": err.Error()})
		}
	}
	if err := p.Delete("messages", messageID); err != nil {
		return err
	}
	if mailboxID != "" {
		updateMailboxUsage(p, mailboxID, -1, -deltaBytes)
	}
	return nil
}

func deleteByMessageID(p *Poche, coll, messageID string) error {
	data, err := p.List(coll, "message_id="+messageID, 1000, 0, "", false)
	if err != nil {
		return err
	}
	var page struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		return err
	}
	for _, it := range page.Items {
		if err := p.Delete(coll, it.ID); err != nil {
			return err
		}
	}
	return nil
}

func logCleanupEvent(event string, fields map[string]any) {
	rec := map[string]any{"event": event}
	for k, v := range fields {
		rec[k] = v
	}
	b, _ := json.Marshal(rec)
	fmt.Fprintln(os.Stderr, string(b))
}

func mailboxFloat(doc map[string]any, key string, fallback float64) float64 {
	v := numField(doc, key)
	if v != 0 {
		return v
	}
	return fallback
}

func mailboxInt(doc map[string]any, key string, fallback int) int {
	v := int(numField(doc, key))
	if v != 0 {
		return v
	}
	return fallback
}

func mailboxInt64(doc map[string]any, key string, fallback int64) int64 {
	v := int64(numField(doc, key))
	if v != 0 {
		return v
	}
	return fallback
}

func numField(m map[string]any, k string) float64 {
	switch v := m[k].(type) {
	case float64:
		return v
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	}
	return 0
}

func int64Field(m map[string]any, k string) int64 {
	switch v := m[k].(type) {
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case int64:
		return v
	case json.Number:
		i, _ := v.Int64()
		return i
	}
	return 0
}

func boolField(m map[string]any, k string) bool {
	v, _ := m[k].(bool)
	return v
}

func stringField(m map[string]any, k string) string {
	v, _ := m[k].(string)
	return v
}

func envFloat(k string, def float64) float64 {
	s := os.Getenv(k)
	if s == "" {
		return def
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return def
	}
	return v
}

func envInt(k string, def int) int {
	s := os.Getenv(k)
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

func envInt64(k string, def int64) int64 {
	s := os.Getenv(k)
	if s == "" {
		return def
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return def
	}
	return v
}
