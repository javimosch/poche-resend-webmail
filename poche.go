package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Poche is a thin HTTP client for poche serve (admin + collection APIs).
type Poche struct {
	Base   string
	Token  string
	Client *http.Client
}

func newPocheFromEnv() *Poche {
	base := envOr("POCHE_URL", "http://127.0.0.1:17781")
	token := os.Getenv("POCHE_TOKEN")
	return &Poche{
		Base:   strings.TrimRight(base, "/"),
		Token:  token,
		Client: &http.Client{Timeout: 30 * time.Second},
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

type pocheEnvelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (p *Poche) do(method, path string, body any) (int, json.RawMessage, error) {
	var rdr io.Reader
	if body != nil {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(body); err != nil {
			return 0, nil, err
		}
		// Encode adds trailing newline; trim for poche valid_doc
		b := bytes.TrimSpace(buf.Bytes())
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, p.Base+path, rdr)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.Token != "" {
		req.Header.Set("Authorization", "Bearer "+p.Token)
	}
	res, err := p.Client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return res.StatusCode, nil, err
	}
	var env pocheEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return res.StatusCode, raw, fmt.Errorf("non-json response (%d): %s", res.StatusCode, truncate(string(raw), 200))
	}
	if !env.OK {
		msg := "poche error"
		if env.Error != nil {
			msg = env.Error.Message
		}
		return res.StatusCode, env.Data, fmt.Errorf("%s", msg)
	}
	return res.StatusCode, env.Data, nil
}

func (p *Poche) Health() error {
	res, err := p.Client.Get(p.Base + "/health")
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("health status %d", res.StatusCode)
	}
	return nil
}

func (p *Poche) AdminSchema(name, fields string) error {
	_, _, err := p.do("POST", "/admin/schema", map[string]string{"name": name, "fields": fields})
	return err
}

func (p *Poche) AdminIndex(coll, field, kind string) error {
	body := map[string]string{"collection": coll, "field": field}
	if kind != "" {
		body["kind"] = kind
	}
	_, _, err := p.do("POST", "/admin/index", body)
	return err
}

func (p *Poche) AdminExpose(coll, actions string) error {
	_, _, err := p.do("POST", "/admin/expose", map[string]string{"collection": coll, "actions": actions})
	return err
}

func (p *Poche) Create(coll string, doc map[string]any) (json.RawMessage, error) {
	_, data, err := p.do("POST", "/api/"+url.PathEscape(coll), doc)
	return data, err
}

func (p *Poche) Update(coll, id string, doc map[string]any) (json.RawMessage, error) {
	_, data, err := p.do("PUT", "/api/"+url.PathEscape(coll)+"/"+url.PathEscape(id), doc)
	return data, err
}

func (p *Poche) Delete(coll, id string) error {
	_, _, err := p.do("DELETE", "/api/"+url.PathEscape(coll)+"/"+url.PathEscape(id), nil)
	return err
}

func (p *Poche) Get(coll, id string) (json.RawMessage, error) {
	_, data, err := p.do("GET", "/api/"+url.PathEscape(coll)+"/"+url.PathEscape(id), nil)
	return data, err
}

func (p *Poche) List(coll, where string, limit, offset int, sort string, desc bool) (json.RawMessage, error) {
	return p.ListLinked(coll, where, nil, nil, limit, offset, sort, desc)
}

func (p *Poche) ListLinked(coll, where string, hasLink, missingLink []string, limit, offset int, sort string, desc bool) (json.RawMessage, error) {
	q := url.Values{}
	q.Set("limit", fmt.Sprintf("%d", limit))
	q.Set("offset", fmt.Sprintf("%d", offset))
	if where != "" {
		q.Set("where", where)
	}
	if sort != "" {
		q.Set("sort", sort)
		if desc {
			q.Set("order", "desc")
		}
	}
	for _, h := range hasLink {
		q.Add("has_link", h)
	}
	for _, m := range missingLink {
		q.Add("missing_link", m)
	}
	_, data, err := p.do("GET", "/api/"+url.PathEscape(coll)+"?"+q.Encode(), nil)
	return data, err
}

func (p *Poche) CountLinked(coll, where string, hasLink, missingLink []string) (int, error) {
	q := url.Values{}
	if where != "" {
		q.Set("where", where)
	}
	for _, h := range hasLink {
		q.Add("has_link", h)
	}
	for _, m := range missingLink {
		q.Add("missing_link", m)
	}
	path := "/api/" + url.PathEscape(coll) + "/count"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	_, data, err := p.do("GET", path, nil)
	if err != nil {
		return 0, err
	}
	var out struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return 0, err
	}
	return out.Count, nil
}

func (p *Poche) Count(coll, where string) (int, error) {
	q := url.Values{}
	if where != "" {
		q.Set("where", where)
	}
	path := "/api/" + url.PathEscape(coll) + "/count"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	_, data, err := p.do("GET", path, nil)
	if err != nil {
		return 0, err
	}
	var out struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return 0, err
	}
	return out.Count, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
