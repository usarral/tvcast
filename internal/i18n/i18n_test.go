package i18n

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// Every language must have exactly the keys of English, with the same
// placeholders, so a translation can never break a fmt verb or a {name}.
func TestCatalogComplete(t *testing.T) {
	for _, l := range Supported {
		for k, en := range catalog[EN] {
			s, ok := catalog[l][k]
			if !ok {
				t.Errorf("%s: missing %q", l, k)
				continue
			}
			if placeholders(s) != placeholders(en) {
				t.Errorf("%s: %q placeholders %q, English has %q", l, k, placeholders(s), placeholders(en))
			}
		}
		for k := range catalog[l] {
			if _, ok := catalog[EN][k]; !ok {
				t.Errorf("%s: extra key %q not in English", l, k)
			}
		}
	}
}

func placeholders(s string) string {
	var out []string
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '%':
			if i+1 < len(s) {
				out = append(out, s[i:i+2])
				i++
			}
		case '{':
			if j := strings.IndexByte(s[i:], '}'); j > 0 {
				out = append(out, s[i:i+j+1])
			}
		}
	}
	return strings.Join(out, " ")
}

func TestParse(t *testing.T) {
	for in, want := range map[string]Lang{"es": ES, "es_ES.UTF-8": ES, "ES-es": ES, " en-GB;q=0.8": EN, "en": EN} {
		if got, ok := Parse(in); !ok || got != want {
			t.Errorf("Parse(%q) = %q,%v want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "fr", "*"} {
		if _, ok := Parse(in); ok {
			t.Errorf("Parse(%q) should fail", in)
		}
	}
}

func TestFromRequest(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	if got := FromRequest(r, ES); got != ES {
		t.Errorf("no header: got %q, want default", got)
	}
	r.Header.Set("Accept-Language", "fr-FR,fr;q=0.9,es;q=0.8,en;q=0.7")
	if got := FromRequest(r, EN); got != ES {
		t.Errorf("got %q, want es (first supported)", got)
	}
}

func TestFallback(t *testing.T) {
	if T("fr", "tv.title") != catalog[EN]["tv.title"] {
		t.Error("unknown language should fall back to English")
	}
	if T(ES, "no.such.key") != "no.such.key" {
		t.Error("unknown key should return the key")
	}
}
