package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

func handleMessagesAPI(w http.ResponseWriter, r *http.Request) {
	p := newPocheFromEnv()
	if p.Token == "" {
		writeJSON(w, 500, map[string]any{"ok": false, "error": "POCHE_TOKEN missing"})
		return
	}
	mbID := authMailboxID(r)
	isAdmin := authIsAdmin(r)
	path := strings.TrimPrefix(r.URL.Path, "/api/messages")
	path = strings.Trim(path, "/")

	// scopeWhere returns the where clause with mailbox_id filter for non-admin
	scopeWhere := func(base string) string {
		if isAdmin || mbID == "" {
			return base
		}
		if base != "" {
			return base + ",mailbox_id=" + mbID
		}
		return "mailbox_id=" + mbID
	}

	if path == "count" && r.Method == http.MethodGet {
		has := r.URL.Query()["has_link"]
		miss := r.URL.Query()["missing_link"]
		where := scopeWhere(r.URL.Query().Get("where"))
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
		data, err := p.ListLinked("messages", scopeWhere(q.Get("where")), q["has_link"], q["missing_link"], limit, offset, sort, desc)
		if err != nil {
			writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "data": json.RawMessage(data)})
		return
	}

	// /api/messages/:id[/attachments]
	parts := strings.Split(path, "/")
	if len(parts) == 2 && parts[1] == "star" && r.Method == http.MethodPut {
		id := parts[0]
		doc, err := loadDoc(p, id)
		if err != nil {
			writeJSON(w, 404, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		star, _ := doc["starred"].(bool)
		star = !star
		doc["starred"] = star
		if _, err := p.Update("messages", id, doc); err != nil {
			writeJSON(w, 500, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "starred": star})
		return
	}
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
		writeJSON(w, 404, map[string]any{"ok": false, "error": "not found"})
		return
	}
	var wrap struct {
		Doc json.RawMessage `json:"doc"`
	}
	_ = json.Unmarshal(raw, &wrap)
	doc := map[string]any{}
	_ = json.Unmarshal(wrap.Doc, &doc)

	// An attachment id alone must not grant access: without this any signed-in
	// tenant could read another tenant's files by guessing or replaying an id.
	if !authIsAdmin(r) {
		mbID := authMailboxID(r)
		msgID, _ := doc["message_id"].(string)
		if mbID == "" || msgID == "" {
			writeJSON(w, 403, map[string]any{"ok": false, "error": "forbidden"})
			return
		}
		msg, err := loadDoc(p, msgID)
		if err != nil {
			writeJSON(w, 404, map[string]any{"ok": false, "error": "not found"})
			return
		}
		if owner, _ := msg["mailbox_id"].(string); owner != mbID {
			writeJSON(w, 403, map[string]any{"ok": false, "error": "forbidden"})
			return
		}
	}

	filename := strField(doc, "filename")
	if filename == "" {
		filename = "attachment"
	}

	// Stored locally (outbound copies): serve the bytes ourselves.
	if fileID := strField(doc, "file_id"); fileID != "" {
		if path, ok := blobPath(fileID); ok {
			f, err := os.Open(path)
			if err != nil {
				writeJSON(w, 404, map[string]any{"ok": false, "error": "blob missing"})
				return
			}
			defer f.Close()
			st, _ := f.Stat()
			ct := strField(doc, "content_type")
			if ct == "" {
				ct = "application/octet-stream"
			}
			// Always as a download, never rendered: an HTML or SVG attachment
			// served inline would execute in the mailbox's own origin.
			w.Header().Set("Content-Type", ct)
			w.Header().Set("Content-Disposition", "attachment; filename=\""+safeFilename(filename)+"\"")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
			if st != nil {
				w.Header().Set("Content-Length", fmt.Sprintf("%d", st.Size()))
			}
			http.ServeContent(w, r, filename, time.Time{}, f)
			return
		}
	}

	// Inbound (cached in machin-esetres at ingest time — see sync.go's
	// upsertAttachments). Proxied, never a redirect: a raw redirect to an
	// external URL would bypass the same Content-Disposition/nosniff/CSP
	// headers the local-blob path above sets, and machin-esetres has no way
	// to know about that policy itself. This replaces the old dead
	// download_url-redirect branch — Resend never actually returns a
	// per-attachment download_url (see mime_extract.go's header comment for
	// how that was confirmed), so that branch never fired in practice.
	if key := strField(doc, "esetres_key"); key != "" {
		data, ct, err := esetresGet(key)
		if err != nil {
			writeJSON(w, 404, map[string]any{"ok": false, "error": "attachment content unavailable"})
			return
		}
		if ct == "" {
			ct = strField(doc, "content_type")
		}
		if ct == "" {
			ct = "application/octet-stream"
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Content-Disposition", "attachment; filename=\""+safeFilename(filename)+"\"")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		w.Write(data)
		return
	}
	writeJSON(w, 404, map[string]any{"ok": false, "error": "no content stored for this attachment"})
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
