package main

import "testing"

func TestExtractMessageIDs(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"<a@x.com>", []string{"<a@x.com>"}},
		{"<a@x.com> <b@x.com> <c@x.com>", []string{"<a@x.com>", "<b@x.com>", "<c@x.com>"}},
		{"", nil},
		// A comma-joined References header (no space) is not RFC-standard
		// but seen in the wild — strings.Fields-based splitting used to
		// silently merge these into one bogus token.
		{"<a@x.com>,<b@x.com>", []string{"<a@x.com>", "<b@x.com>"}},
		{"no angle brackets here", nil},
	}
	for _, c := range cases {
		got := extractMessageIDs(c.in)
		if len(got) != len(c.want) {
			t.Errorf("extractMessageIDs(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("extractMessageIDs(%q)[%d] = %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

func TestHeaderVal(t *testing.T) {
	doc := map[string]any{
		"headers": map[string]any{
			"in-reply-to": "<parent@x.com>",
		},
	}
	if got := headerVal(doc, "in-reply-to"); got != "<parent@x.com>" {
		t.Errorf("headerVal = %q, want <parent@x.com>", got)
	}
	if got := headerVal(doc, "references"); got != "" {
		t.Errorf("headerVal missing key = %q, want empty", got)
	}
	if got := headerVal(map[string]any{}, "in-reply-to"); got != "" {
		t.Errorf("headerVal no headers = %q, want empty", got)
	}
}
