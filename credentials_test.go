package main

import "testing"

func TestMaskSecret(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"short", "…"},
		{"re_fake000_NotARealKeyABCDEFGHijkl", "re_…ijkl"},
		{"whsec_abcdefghijklmnop", "whs…mnop"},
	}
	for _, c := range cases {
		if got := maskSecret(c.in); got != c.want {
			t.Errorf("maskSecret(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
