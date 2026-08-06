package main

import (
	"bytes"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// Outbound bodies can be written as plain text, HTML, or Markdown. Markdown is
// converted here rather than in the browser so the CLI and any API caller get
// the same result, and so the HTML that leaves the building has been through
// the same sanitizer as everything else.

const (
	formatText     = "text"
	formatHTML     = "html"
	formatMarkdown = "markdown"
)

var markdownRenderer = goldmark.New(
	goldmark.WithExtensions(extension.GFM), // tables, strikethrough, autolinks
)

func normalizeFormat(f string) string {
	switch strings.ToLower(strings.TrimSpace(f)) {
	case formatHTML:
		return formatHTML
	case formatMarkdown, "md":
		return formatMarkdown
	default:
		return formatText
	}
}

// renderBody turns the composed body into the (text, html) pair to send.
// html is empty for plain text, so those mails stay plain.
func renderBody(body, format string) (text string, html string, err error) {
	switch normalizeFormat(format) {
	case formatHTML:
		clean := sanitizeEmailHTML(body)
		return htmlToPlainText(clean), clean, nil
	case formatMarkdown:
		var buf bytes.Buffer
		if err := markdownRenderer.Convert([]byte(body), &buf); err != nil {
			return "", "", err
		}
		clean := sanitizeEmailHTML(buf.String())
		// Markdown source doubles as a readable plain-text alternative.
		return body, clean, nil
	default:
		return body, "", nil
	}
}

// htmlToPlainText builds the text/plain alternative. Mail without one looks
// like spam to filters and is unreadable in text-only clients.
func htmlToPlainText(html string) string {
	s := stripTags(html)
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blank := false
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			if blank {
				continue
			}
			blank = true
		} else {
			blank = false
		}
		out = append(out, l)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
