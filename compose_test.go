package main

import "testing"

func TestParseAddrList(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want []string
	}{
		{"nil", nil, []string{}},
		{"comma string", "a@b.fr, c@d.fr", []string{"a@b.fr", "c@d.fr"}},
		{"empty entries dropped", "a@b.fr,, c@d.fr", []string{"a@b.fr", "c@d.fr"}},
		{"semicolon string", "a@b.fr;c@d.fr", []string{"a@b.fr", "c@d.fr"}},
		{"display name", "Contact <contact@lacure.enbauges.fr>", []string{"contact@lacure.enbauges.fr"}},
		{"[]any", []any{"a@b.fr", "c@d.fr"}, []string{"a@b.fr", "c@d.fr"}},
		{"[]any mixed types", []any{"a@b.fr", 42}, []string{"a@b.fr"}},
		{"unsupported type", 123, []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseAddrList(c.in)
			if len(got) != len(c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("got %v, want %v", got, c.want)
				}
			}
		})
	}
}

func TestLooksLikeEmail(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"contact@lacure.enbauges.fr", true},
		{"a@b.co", true},
		{"no-at-sign", false},
		{"@b.co", false},
		{"a@", false},
		{"a@@b.co", false},
		{"a@b", false},
		{"a@.co", false},
		{"a@b.co.", false},
		{"a b@c.co", false},
		{"a@b.co,c@d.co", false},
	}
	for _, c := range cases {
		if got := looksLikeEmail(c.in); got != c.want {
			t.Errorf("looksLikeEmail(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestSafeFilename(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"devis.pdf", "devis.pdf"},
		{"../../etc/passwd", "passwd"},
		{`C:\Users\x\file.txt`, "file.txt"},
		{"...hidden", "hidden"},
		{"", "attachment"},
		{"   ", "attachment"},
	}
	for _, c := range cases {
		if got := safeFilename(c.in); got != c.want {
			t.Errorf("safeFilename(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if got := safeFilename(string(make([]byte, 300))); len(got) > 200 {
		t.Errorf("safeFilename did not cap length: got %d chars", len(got))
	}
}

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{500, "500 B"},
		{2048, "2 KB"},
		{5 * 1024 * 1024, "5.0 MB"},
	}
	for _, c := range cases {
		if got := humanBytes(c.in); got != c.want {
			t.Errorf("humanBytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
