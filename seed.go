package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"
)

const (
	linkArchive = "message_tags.message_id:tag=archive"
	tagArchive  = "archive"
)

var seedSubjects = []string{
	"Welcome to poche webmail",
	"Invoice #{{n}} ready",
	"Re: project kickoff",
	"Your weekly digest",
	"Security alert — new login",
	"Meeting notes {{n}}",
	"Shipment update {{n}}",
	"Can we reschedule?",
	"Fwd: design review",
	"Action required: approve PR",
}

var seedSenders = []string{
	"ada@example.com",
	"billing@acme.test",
	"noreply@github.com",
	"support@resend.dev",
	"javi@intrane.fr",
	"ops@peage.test",
	"bot@ci.example",
	"hello@customers.test",
}

var seedUserTags = []string{"billing", "urgent", "personal", "ops"}

func handleSeed(count int) {
	p := newPocheFromEnv()
	if p.Token == "" {
		fail(80, "input", "set POCHE_TOKEN (admin token from poche init)", "export POCHE_TOKEN=…")
	}
	fmt.Fprintf(os.Stderr, "{\"event\":\"seed_start\",\"count\":%d,\"poche\":%q}\n", count, p.Base)
	if err := p.Health(); err != nil {
		fail(100, "integration", "poche unreachable: "+err.Error(), "start: POCHE_DB=./mail.data poche serve 17780")
	}
	if err := ensureSchema(p); err != nil {
		fail(100, "integration", "schema setup failed: "+err.Error(), "")
	}
	if err := ensureMailboxSchema(p); err != nil {
		fail(100, "integration", "mailbox schema failed: "+err.Error(), "")
	}
	if err := ensureTags(p); err != nil {
		fail(100, "integration", "tags: "+err.Error(), "")
	}

	existing, err := p.Count("messages", "")
	if err != nil {
		fail(100, "integration", "count failed: "+err.Error(), "")
	}
	if existing >= count {
		outOK(map[string]any{"seeded": false, "existing": existing, "requested": count})
	}

	mb, err := ensureMailbox(p)
	if err != nil {
		fail(100, "integration", "mailbox: "+err.Error(), "")
	}

	need := count - existing
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	batch := 100
	created := 0
	for created < need {
		n := batch
		if need-created < n {
			n = need - created
		}
		for i := 0; i < n; i++ {
			idx := existing + created + i + 1
			doc := mockMessage(mb, idx, rng)
			raw, err := p.Create("messages", doc)
			if err != nil {
				fail(100, "integration", fmt.Sprintf("create #%d: %v", idx, err), "")
			}
			var createdDoc map[string]any
			_ = json.Unmarshal(raw, &createdDoc)
			mid, _ := createdDoc["_id"].(string)
			if mid == "" {
				fail(100, "integration", "no message id", "")
			}
			// ~8% archived via system tag; ~15% get a user tag
			if rng.Intn(12) == 0 {
				_, _ = p.Create("message_tags", map[string]any{"message_id": mid, "tag": tagArchive})
			}
			if rng.Intn(7) == 0 {
				t := seedUserTags[rng.Intn(len(seedUserTags))]
				_, _ = p.Create("message_tags", map[string]any{"message_id": mid, "tag": t})
			}
		}
		created += n
		fmt.Fprintf(os.Stderr, "{\"event\":\"seed_progress\",\"created\":%d,\"target\":%d}\n", existing+created, count)
	}
	// Recalc and persist mailbox usage counters once after bulk seeding.
	_, _, _ = mailboxUsage(p, mb)
	outOK(map[string]any{"seeded": true, "created": created, "total": existing + created, "mailbox": mb})
}

func ensureSchema(p *Poche) error {
	if err := p.AdminSchema("mailboxes", "name:string!required!unique,address:string!required,retention_months:float,max_messages:int,max_bytes:int"); err != nil {
		return fmt.Errorf("mailboxes: %w", err)
	}
	if err := p.AdminSchema("messages",
		"mailbox_id:string!required!ref=mailboxes,from_addr:string!required,to_addr:string!required,cc_addr:string,subject:string!required,preview:string,body_text:string,body_html:string,html_sanitized:bool,search_text:string,thread_id:string,unread:bool,starred:bool,resend_id:string,message_id:string,received_for:string,direction:string,in_reply_to:string,references:string,created_at:int!now"); err != nil {
		return fmt.Errorf("messages: %w", err)
	}
	if err := p.AdminSchema("tags", "name:string!required!unique"); err != nil {
		return fmt.Errorf("tags: %w", err)
	}
	if err := p.AdminSchema("message_tags",
		"message_id:string!required!ref=messages,tag:string!required"); err != nil {
		return fmt.Errorf("message_tags: %w", err)
	}
	if err := p.AdminSchema("attachments",
		"message_id:string!required!ref=messages,filename:string,content_type:string,resend_attach_id:string,download_url:string,file_id:string,bytes:int,stored:bool"); err != nil {
		return fmt.Errorf("attachments: %w", err)
	}
	_ = p.AdminIndex("messages", "created_at", "range")
	_ = p.AdminIndex("messages", "mailbox_id", "")
	_ = p.AdminIndex("messages", "resend_id", "")
	_ = p.AdminIndex("message_tags", "tag", "")
	_ = p.AdminIndex("message_tags", "message_id", "")
	_ = p.AdminIndex("tags", "name", "")
	_ = p.AdminIndex("attachments", "message_id", "")
	if err := p.AdminExpose("messages", "read,create,update,delete"); err != nil {
		return err
	}
	if err := p.AdminExpose("tags", "read,create,delete"); err != nil {
		return err
	}
	if err := p.AdminExpose("message_tags", "read,create,delete"); err != nil {
		return err
	}
	if err := p.AdminExpose("attachments", "read,create,delete"); err != nil {
		return err
	}
	return nil
}

func ensureTags(p *Poche) error {
	names := append([]string{tagArchive}, seedUserTags...)
	for _, name := range names {
		data, err := p.List("tags", "name="+name, 1, 0, "", false)
		if err == nil {
			var page struct {
				Total int `json:"total"`
			}
			if json.Unmarshal(data, &page) == nil && page.Total > 0 {
				continue
			}
		}
		if _, err := p.Create("tags", map[string]any{"name": name}); err != nil {
			// unique conflict on re-seed is fine
			fmt.Fprintf(os.Stderr, "{\"event\":\"tag_warn\",\"tag\":%q,\"err\":%q}\n", name, err.Error())
		}
	}
	return nil
}

func ensureMailbox(p *Poche) (string, error) {
	data, err := p.List("mailboxes", "", 1, 0, "", false)
	if err == nil && len(data) > 0 && string(data) != "null" {
		var page struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
		}
		if json.Unmarshal(data, &page) == nil && len(page.Items) > 0 && page.Items[0].ID != "" {
			return page.Items[0].ID, nil
		}
	}
	created, err := p.Create("mailboxes", map[string]any{
		"name":    "Inbox",
		"address": "demo@poche.local",
	})
	if err != nil {
		return "", err
	}
	var doc map[string]any
	if err := json.Unmarshal(created, &doc); err != nil {
		return "", err
	}
	id, _ := doc["_id"].(string)
	if id == "" {
		return "", fmt.Errorf("no _id in mailbox create")
	}
	return id, nil
}

func mockMessage(mailboxID string, n int, rng *rand.Rand) map[string]any {
	subjTpl := seedSubjects[rng.Intn(len(seedSubjects))]
	subj := strings.ReplaceAll(subjTpl, "{{n}}", fmt.Sprintf("%d", n))
	from := seedSenders[rng.Intn(len(seedSenders))]
	preview := fmt.Sprintf("Mock message #%d from %s — used to stress poche list/sort/paging.", n, from)
	body := fmt.Sprintf("Hello,\n\nThis is mock email #%d.\n\n— poche-webmail-demo\n", n)
	html := fmt.Sprintf("<p>Hello,</p><p>This is mock email <strong>#%d</strong>.</p><p>— poche-webmail-demo</p>", n)
	search := strings.ToLower(subj + " " + from + " " + preview + " " + body)
	return map[string]any{
		"mailbox_id":   mailboxID,
		"from_addr":    from,
		"to_addr":      "demo@poche.local",
		"subject":      subj,
		"preview":      preview,
		"body_text":    body,
		"body_html":    html,
		"search_text":  search,
		"thread_id":    fmt.Sprintf("t-%d", n/3),
		"unread":       rng.Intn(3) != 0,
		"starred":      rng.Intn(10) == 0,
		"resend_id":    fmt.Sprintf("seed-%d", n),
		"message_id":   fmt.Sprintf("<seed-%d@poche.local>", n),
		"received_for": "demo@poche.local",
		"direction":    "in",
		"in_reply_to":  "",
		"references":   "",
	}
}
