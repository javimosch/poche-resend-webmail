package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

type bulkReq struct {
	Action string   `json:"action"` // mark_read|mark_unread|archive|unarchive|delete|mark_read_all|tag|untag
	IDs    []string `json:"ids"`
	Tag    string   `json:"tag"`
	// View context for mark_read_all / all_pages
	View     string `json:"view"` // inbox|archive|tag
	TagView  string `json:"tag_view"`
	Q        string `json:"q"`
	AllPages bool   `json:"all_pages"`
}

func handleBulkAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, 405, map[string]any{"ok": false, "error": "POST only"})
		return
	}
	var req bulkReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "bad json"})
		return
	}
	p := newPocheFromEnv()
	if p.Token == "" {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "POCHE_TOKEN missing"})
		return
	}
	mbID := authMailboxID(r)
	isAdmin := authIsAdmin(r)

	ids := req.IDs
	if req.Action == "mark_read_all" || req.AllPages {
		var err error
		ids, err = collectIDsLinked(p, req.View, req.TagView, req.Q, mbID, isAdmin)
		if err != nil {
			writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if req.Action == "mark_read_all" {
			req.Action = "mark_read"
		}
	} else if !isAdmin {
		// req.IDs here are client-supplied (the checked rows in the UI) and
		// were NOT derived from a mailbox-scoped query like collectIDsLinked
		// above — without this filter, any signed-in tenant could
		// star/archive/tag/delete another tenant's mail by sending its ids
		// directly, no guessing required once any two ids were ever visible
		// in the same browser (e.g. via the account switcher).
		ids = filterIDsOwnedByMailbox(p, ids, mbID)
	}

	// Destructive bulk actions are logged with who and how many: after a
	// mailbox was emptied twice with no record of it, "we cannot tell what
	// deleted this" was the expensive part, not the deletion itself.
	if req.Action == "delete" || req.AllPages {
		fmt.Fprintf(os.Stderr,
			"{\"event\":\"bulk_action\",\"action\":%q,\"count\":%d,\"all_pages\":%v,\"view\":%q,\"tag_view\":%q,\"mailbox\":%q,\"admin\":%v}\n",
			req.Action, len(ids), req.AllPages, req.View, req.TagView, mbID, isAdmin)
	}

	okN := 0
	failN := 0
	for _, id := range ids {
		if id == "" {
			continue
		}
		var err error
		switch req.Action {
		case "mark_read", "mark_unread":
			err = patchBool(p, id, "unread", req.Action == "mark_unread")
		case "star", "unstar":
			err = patchBool(p, id, "starred", req.Action == "star")
		case "archive":
			err = ensureTag(p, id, tagArchive)
		case "unarchive":
			err = removeTag(p, id, tagArchive)
		case "tag":
			if req.Tag == "" {
				writeJSON(w, 400, map[string]any{"ok": false, "error": "tag required"})
				return
			}
			err = ensureTag(p, id, req.Tag)
		case "untag":
			if req.Tag == "" {
				writeJSON(w, 400, map[string]any{"ok": false, "error": "tag required"})
				return
			}
			err = removeTag(p, id, req.Tag)
		case "delete":
			err = deleteMessageWithLinks(p, id)
		default:
			writeJSON(w, 400, map[string]any{"ok": false, "error": "unknown action"})
			return
		}
		if err != nil {
			failN++
		} else {
			okN++
		}
	}
	writeJSON(w, 200, map[string]any{"ok": true, "data": map[string]any{"action": req.Action, "ok": okN, "failed": failN}})
}

func handleTagsAPI(w http.ResponseWriter, r *http.Request) {
	p := newPocheFromEnv()
	if p.Token == "" {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "POCHE_TOKEN missing"})
		return
	}
	if r.Method == http.MethodGet {
		data, err := p.List("tags", "", 200, 0, "", false)
		if err != nil {
			writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "data": json.RawMessage(data)})
		return
	}
	if r.Method == http.MethodPost {
		var body struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, 400, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		name := sanitizeTagName(body.Name)
		if name == "" {
			writeJSON(w, 400, map[string]any{"ok": false, "error": "name required"})
			return
		}
		created, err := p.Create("tags", map[string]any{"name": name})
		if err != nil {
			writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, 201, map[string]any{"ok": true, "data": json.RawMessage(created)})
		return
	}
	// PUT {"name":"old","new_name":"new"} — rename everywhere the tag is used.
	if r.Method == http.MethodPut {
		var body struct {
			Name    string `json:"name"`
			NewName string `json:"new_name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, 400, map[string]any{"ok": false, "error": "bad json"})
			return
		}
		if isArchiveName(body.Name) || isArchiveName(body.NewName) {
			writeJSON(w, 400, map[string]any{"ok": false, "error": "archive is a system tag and cannot be renamed"})
			return
		}
		from := sanitizeTagName(body.Name)
		to := sanitizeTagName(body.NewName)
		if from == "" || to == "" {
			writeJSON(w, 400, map[string]any{"ok": false, "error": "name and new_name required"})
			return
		}
		if from == to {
			writeJSON(w, 200, map[string]any{"ok": true, "data": map[string]any{"renamed": 0, "name": to}})
			return
		}
		n, err := renameTag(p, from, to)
		if err != nil {
			writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "data": map[string]any{"renamed": n, "from": from, "name": to}})
		return
	}
	// DELETE ?name=x — drop the tag and every message link to it.
	if r.Method == http.MethodDelete {
		raw := r.URL.Query().Get("name")
		if isArchiveName(raw) {
			writeJSON(w, 400, map[string]any{"ok": false, "error": "archive is a system tag and cannot be deleted"})
			return
		}
		name := sanitizeTagName(raw)
		if name == "" {
			writeJSON(w, 400, map[string]any{"ok": false, "error": "name required"})
			return
		}
		n, err := deleteTagEverywhere(p, name)
		if err != nil {
			writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "data": map[string]any{"deleted": true, "name": name, "untagged": n}})
		return
	}
	writeJSON(w, 405, map[string]any{"ok": false, "error": "GET, POST, PUT or DELETE"})
}

// isArchiveName spots the system tag before sanitizeTagName blanks it out.
func isArchiveName(s string) bool {
	return strings.EqualFold(strings.TrimSpace(s), tagArchive)
}

type tagLink struct {
	id        string
	messageID string
}

// listTagLinks returns the message_tags rows carrying a tag.
func listTagLinks(p *Poche, tag string) ([]tagLink, error) {
	data, err := p.List("message_tags", "tag="+tag, 10000, 0, "", false)
	if err != nil {
		return nil, err
	}
	var page struct {
		Items []struct {
			ID  string `json:"id"`
			Doc struct {
				MessageID string `json:"message_id"`
			} `json:"doc"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		return nil, err
	}
	out := make([]tagLink, 0, len(page.Items))
	for _, it := range page.Items {
		out = append(out, tagLink{id: it.ID, messageID: it.Doc.MessageID})
	}
	return out, nil
}

// renameTag rewrites the tag row and every message link, so messages keep
// their labels instead of silently losing them.
func renameTag(p *Poche, from, to string) (int, error) {
	links, err := listTagLinks(p, from)
	if err != nil {
		return 0, err
	}
	// Create the destination tag first; a partial rename that lost the tag row
	// would leave links pointing at a tag the sidebar never lists.
	if err := ensureTagRow(p, to); err != nil {
		return 0, err
	}
	// message_tags is exposed for create+delete but not update, so a relabel is
	// "attach the new tag, then drop the old" — the same path tagging uses.
	moved := 0
	for _, l := range links {
		if l.messageID == "" {
			continue
		}
		if err := ensureTag(p, l.messageID, to); err != nil {
			return moved, fmt.Errorf("relabel %s: %w", l.messageID, err)
		}
		if err := removeTag(p, l.messageID, from); err != nil {
			return moved, fmt.Errorf("drop old label %s: %w", l.messageID, err)
		}
		moved++
	}
	if err := deleteTagRow(p, from); err != nil {
		return moved, err
	}
	return moved, nil
}

func deleteTagEverywhere(p *Poche, name string) (int, error) {
	links, err := listTagLinks(p, name)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, l := range links {
		if err := p.Delete("message_tags", l.id); err != nil {
			return removed, fmt.Errorf("untag %s: %w", l.id, err)
		}
		removed++
	}
	if err := deleteTagRow(p, name); err != nil {
		return removed, err
	}
	return removed, nil
}

func ensureTagRow(p *Poche, name string) error {
	data, err := p.List("tags", "name="+name, 1, 0, "", false)
	if err != nil {
		return err
	}
	var page struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal(data, &page)
	if page.Total > 0 {
		return nil
	}
	_, err = p.Create("tags", map[string]any{"name": name})
	return err
}

func deleteTagRow(p *Poche, name string) error {
	data, err := p.List("tags", "name="+name, 10, 0, "", false)
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
		if err := p.Delete("tags", it.ID); err != nil {
			return fmt.Errorf("delete tag row: %w", err)
		}
	}
	return nil
}

func handleMessageTagsAPI(w http.ResponseWriter, r *http.Request) {
	p := newPocheFromEnv()
	id := r.URL.Query().Get("message_id")
	if id == "" {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "message_id required"})
		return
	}
	data, err := p.List("message_tags", "message_id="+id, 100, 0, "", false)
	if err != nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "data": json.RawMessage(data)})
}

func viewLinks(view, tagView string) (has, missing []string) {
	switch view {
	case "archive":
		has = []string{linkArchive}
	case "tag":
		if tagView != "" {
			has = []string{"message_tags.message_id:tag=" + tagView}
		}
	default: // inbox
		missing = []string{linkArchive}
	}
	return
}

// filterIDsOwnedByMailbox drops any id whose message doesn't belong to mbID
// (or that doesn't exist) — silently, so a caller's action just applies to
// fewer ids rather than erroring, matching handleBulkAPI's existing
// okN/failN counting for individual failures.
func filterIDsOwnedByMailbox(p *Poche, ids []string, mbID string) []string {
	if mbID == "" {
		return nil
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		doc, err := loadDoc(p, id)
		if err != nil {
			continue
		}
		if strField(doc, "mailbox_id") == mbID {
			out = append(out, id)
		}
	}
	return out
}

func collectIDsLinked(p *Poche, view, tagView, q string, mbID string, isAdmin bool) ([]string, error) {
	has, missing := viewLinks(view, tagView)
	where := ""
	if needle := sanitizeQ(q); needle != "" {
		where = "search_text~=" + needle
	}
	if !isAdmin && mbID != "" {
		if where != "" {
			where += ",mailbox_id=" + mbID
		} else {
			where = "mailbox_id=" + mbID
		}
	}
	out := []string{}
	offset := 0
	for {
		data, err := p.ListLinked("messages", where, has, missing, 200, offset, "created_at", true)
		if err != nil {
			return out, err
		}
		var page struct {
			Items []struct {
				ID string `json:"id"`
			} `json:"items"`
			Total int `json:"total"`
		}
		if err := json.Unmarshal(data, &page); err != nil {
			return out, err
		}
		if len(page.Items) == 0 {
			break
		}
		for _, it := range page.Items {
			out = append(out, it.ID)
		}
		offset += len(page.Items)
		if offset >= page.Total {
			break
		}
	}
	return out, nil
}

func sanitizeQ(q string) string {
	out := ""
	for _, r := range q {
		if r == ',' {
			out += " "
			continue
		}
		out += string(r)
	}
	return out
}

func sanitizeTagName(name string) string {
	out := ""
	for _, r := range name {
		if r >= 'A' && r <= 'Z' {
			out += string(r + ('a' - 'A'))
			continue
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			out += string(r)
		}
	}
	if out == "archive" {
		return ""
	}
	return out
}

func ensureTag(p *Poche, messageID, tag string) error {
	data, err := p.List("message_tags", "message_id="+messageID+",tag="+tag, 1, 0, "", false)
	if err == nil {
		var page struct {
			Total int `json:"total"`
		}
		if json.Unmarshal(data, &page) == nil && page.Total > 0 {
			return nil
		}
	}
	_, err = p.Create("message_tags", map[string]any{"message_id": messageID, "tag": tag})
	return err
}

func removeTag(p *Poche, messageID, tag string) error {
	data, err := p.List("message_tags", "message_id="+messageID+",tag="+tag, 50, 0, "", false)
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
		if err := p.Delete("message_tags", it.ID); err != nil {
			return err
		}
	}
	return nil
}

func removeAllTags(p *Poche, messageID string) error {
	data, err := p.List("message_tags", "message_id="+messageID, 200, 0, "", false)
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
		_ = p.Delete("message_tags", it.ID)
	}
	return nil
}

func loadDoc(p *Poche, id string) (map[string]any, error) {
	raw, err := p.Get("messages", id)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Doc json.RawMessage `json:"doc"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, err
	}
	doc := map[string]any{}
	if err := json.Unmarshal(wrap.Doc, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func patchBool(p *Poche, id, field string, val bool) error {
	doc, err := loadDoc(p, id)
	if err != nil {
		return err
	}
	doc[field] = val
	_, err = p.Update("messages", id, doc)
	return err
}
