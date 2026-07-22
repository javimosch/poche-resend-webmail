package main

import (
	"encoding/json"
	"net/http"
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

	ids := req.IDs
	if req.Action == "mark_read_all" || req.AllPages {
		var err error
		ids, err = collectIDsLinked(p, req.View, req.TagView, req.Q)
		if err != nil {
			writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		if req.Action == "mark_read_all" {
			req.Action = "mark_read"
		}
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
			_ = removeAllTags(p, id)
			err = p.Delete("messages", id)
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
	writeJSON(w, 405, map[string]any{"ok": false, "error": "GET or POST"})
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

func collectIDsLinked(p *Poche, view, tagView, q string) ([]string, error) {
	has, missing := viewLinks(view, tagView)
	where := ""
	if needle := sanitizeQ(q); needle != "" {
		where = "search_text~=" + needle
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
