package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

func handleMessagesAPI(w http.ResponseWriter, r *http.Request) {
	p := newPocheFromEnv()
	if p.Token == "" {
		writeJSON(w, 500, map[string]any{"ok": false, "error": "POCHE_TOKEN missing"})
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/messages")
	path = strings.Trim(path, "/")

	if path == "count" && r.Method == http.MethodGet {
		has := r.URL.Query()["has_link"]
		miss := r.URL.Query()["missing_link"]
		where := r.URL.Query().Get("where")
		n, err := p.CountLinked("messages", where, has, miss)
		if err != nil {
			writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "data": map[string]any{"count": n}})
		return
	}

	if path == "" && r.Method == http.MethodGet {
		q := r.URL.Query()
		limit := atoiDef(q.Get("limit"), 50)
		offset := atoiDef(q.Get("offset"), 0)
		sort := q.Get("sort")
		if sort == "" {
			sort = "created_at"
		}
		desc := q.Get("order") != "asc"
		data, err := p.ListLinked("messages", q.Get("where"), q["has_link"], q["missing_link"], limit, offset, sort, desc)
		if err != nil {
			writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "data": json.RawMessage(data)})
		return
	}

	// /api/messages/:id[/attachments]
	parts := strings.Split(path, "/")
	if len(parts) >= 1 && parts[0] != "" && r.Method == http.MethodGet {
		id := parts[0]
		if len(parts) == 1 {
			data, err := p.Get("messages", id)
			if err != nil {
				writeJSON(w, 404, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			writeJSON(w, 200, map[string]any{"ok": true, "data": json.RawMessage(data)})
			return
		}
		if len(parts) == 2 && parts[1] == "attachments" {
			data, err := p.List("attachments", "message_id="+id, 100, 0, "", false)
			if err != nil {
				writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
				return
			}
			writeJSON(w, 200, map[string]any{"ok": true, "data": json.RawMessage(data)})
			return
		}
	}
	writeJSON(w, 405, map[string]any{"ok": false, "error": "method/path"})
}

func handleAttachmentOpen(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/attachments/")
	id = strings.TrimSuffix(id, "/open")
	id = strings.Trim(id, "/")
	if id == "" {
		writeJSON(w, 400, map[string]any{"ok": false, "error": "id required"})
		return
	}
	p := newPocheFromEnv()
	raw, err := p.Get("attachments", id)
	if err != nil {
		writeJSON(w, 404, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	var wrap struct {
		Doc json.RawMessage `json:"doc"`
	}
	_ = json.Unmarshal(raw, &wrap)
	doc := map[string]any{}
	_ = json.Unmarshal(wrap.Doc, &doc)
	url, _ := doc["download_url"].(string)
	if url == "" {
		writeJSON(w, 404, map[string]any{"ok": false, "error": "no download_url"})
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}

func atoiDef(s string, def int) int {
	if s == "" {
		return def
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	return n
}
