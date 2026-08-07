package main

import (
	"strings"
	"testing"
)

func TestNormalizeFormat(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"html", formatHTML},
		{"HTML", formatHTML},
		{" html ", formatHTML},
		{"markdown", formatMarkdown},
		{"md", formatMarkdown},
		{"text", formatText},
		{"", formatText},
		{"garbage", formatText},
	}
	for _, c := range cases {
		if got := normalizeFormat(c.in); got != c.want {
			t.Errorf("normalizeFormat(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRenderBodyText(t *testing.T) {
	text, html, err := renderBody("plain body", formatText)
	if err != nil {
		t.Fatal(err)
	}
	if text != "plain body" || html != "" {
		t.Errorf("got text=%q html=%q, want text=%q html=empty", text, html, "plain body")
	}
}

func TestRenderBodyMarkdown(t *testing.T) {
	text, html, err := renderBody("# Bonjour\n\n**devis** ci-joint", formatMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	if text != "# Bonjour\n\n**devis** ci-joint" {
		t.Errorf("markdown source should pass through as the text alternative, got %q", text)
	}
	if html == "" {
		t.Error("expected non-empty rendered html")
	}
}

func TestRenderBodyHTML(t *testing.T) {
	_, html, err := renderBody("<p>hi</p><script>alert(1)</script>", formatHTML)
	if err != nil {
		t.Fatal(err)
	}
	if html == "" {
		t.Fatal("expected non-empty html")
	}
	if strings.Contains(html, "<script") {
		t.Errorf("expected script tag to be sanitized out, got %q", html)
	}
}

func TestHtmlToPlainText(t *testing.T) {
	got := htmlToPlainText("<p>line one</p>\n<p>line two</p>")
	want := "line one\nline two"
	if got != want {
		t.Errorf("htmlToPlainText = %q, want %q", got, want)
	}
}

func TestHtmlToPlainTextCollapsesBlankLines(t *testing.T) {
	// Runs of blank lines collapse to a single blank line, not to zero.
	got := htmlToPlainText("<p>a</p>\n\n\n<p>b</p>")
	if got != "a\n\nb" {
		t.Errorf("expected consecutive blank lines collapsed to one, got %q", got)
	}
}

func TestHtmlToPlainTextStripsTagsWithoutInsertingBreaks(t *testing.T) {
	// stripTags does not add line breaks at block boundaries — only pre-existing
	// newlines in the source separate lines. Documents current behavior.
	got := htmlToPlainText("<p>a</p><p>b</p>")
	if got != "ab" {
		t.Errorf("got %q, want %q", got, "ab")
	}
}

