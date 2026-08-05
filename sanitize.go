package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/microcosm-cc/bluemonday"
)

// Inbound HTML is hostile by default: it arrives from anyone who knows the
// address and is rendered into the mailbox owner's session. Everything stored
// in body_html goes through this policy first, so the render path never has to
// trust the sender.

var (
	mailPolicyOnce sync.Once
	mailPolicy     *bluemonday.Policy
)

func emailHTMLPolicy() *bluemonday.Policy {
	mailPolicyOnce.Do(func() {
		p := bluemonday.UGCPolicy()

		// Drop these elements *and* their contents — otherwise script text
		// would survive as visible text and style blocks could smuggle rules.
		p.SkipElementsContent("script", "style", "iframe", "object", "embed",
			"applet", "form", "noscript", "svg", "math", "template", "base", "link", "meta")

		// Typical newsletter markup: tables, images, basic layout attributes.
		p.AllowTables()
		p.AllowImages()
		p.AllowStandardURLs()
		p.AllowElements("center", "font", "figure", "figcaption", "small", "sub", "sup")
		p.AllowAttrs("align", "valign", "bgcolor", "width", "height",
			"cellpadding", "cellspacing", "border").
			OnElements("table", "thead", "tbody", "tr", "td", "th", "img", "div", "p", "font", "center")
		p.AllowAttrs("color", "face", "size").OnElements("font")

		// Inline styles only from a safe property allowlist; bluemonday parses
		// and re-serializes the CSS, so url(javascript:…) never survives.
		p.AllowStyles(
			"color", "background-color", "font-size", "font-family", "font-weight",
			"font-style", "text-align", "text-decoration", "line-height",
			"padding", "padding-top", "padding-bottom", "padding-left", "padding-right",
			"margin", "margin-top", "margin-bottom", "margin-left", "margin-right",
			"border", "border-top", "border-bottom", "border-left", "border-right",
			"border-color", "border-width", "border-style", "border-radius",
			"width", "height", "max-width", "min-width", "display", "vertical-align",
		).Globally()

		// Links open in a new tab and cannot reach back into the opener.
		p.RequireNoFollowOnLinks(true)
		p.RequireNoReferrerOnLinks(true)
		p.AddTargetBlankToFullyQualifiedLinks(true)

		mailPolicy = p
	})
	return mailPolicy
}

// sanitizeEmailHTML strips anything that could execute in the reader's session.
func sanitizeEmailHTML(html string) string {
	if html == "" {
		return ""
	}
	return emailHTMLPolicy().Sanitize(html)
}

// ─── one-off migration for messages stored before sanitizing existed ────

func handleSanitizeCmd() {
	fs := newFlagSet("sanitize")
	apply := fs.Bool("apply", false, "write changes (default is a dry run)")
	limit := fs.Int("limit", 10000, "max messages to scan")
	_ = fs.Parse(os.Args[2:])

	p := newPocheFromEnv()
	if p.Token == "" {
		fail(80, "input", "set POCHE_TOKEN", "")
	}
	data, err := p.List("messages", "", *limit, 0, "created_at", true)
	if err != nil {
		fail(100, "integration", "list: "+err.Error(), "")
	}
	var page struct {
		Items []struct {
			ID  string         `json:"id"`
			Doc map[string]any `json:"doc"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(data, &page); err != nil {
		fail(110, "internal", "decode: "+err.Error(), "")
	}
	scanned, changed, failed := 0, 0, 0
	for _, it := range page.Items {
		scanned++
		raw := strField(it.Doc, "body_html")
		if raw == "" {
			continue
		}
		clean := sanitizeEmailHTML(raw)
		if clean == raw {
			continue
		}
		changed++
		if !*apply {
			continue
		}
		doc := it.Doc
		doc["body_html"] = clean
		if _, err := p.Update("messages", it.ID, doc); err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "{\"event\":\"sanitize_err\",\"id\":%q,\"err\":%q}\n", it.ID, err.Error())
		}
	}
	outOK(map[string]any{
		"scanned": scanned,
		"changed": changed,
		"failed":  failed,
		"applied": *apply,
	})
}
