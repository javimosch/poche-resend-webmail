package main

import "testing"

func TestEmailDomain(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"javi@intrane.fr", "intrane.fr"},
		{"Javi@Intrane.FR", "intrane.fr"},
		{"  admin@intrane.fr  ", "intrane.fr"},
		{"no-at-sign", ""},
		{"", ""},
		{"@intrane.fr", "intrane.fr"},
	}
	for _, c := range cases {
		if got := emailDomain(c.in); got != c.want {
			t.Errorf("emailDomain(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
