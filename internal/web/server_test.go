package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/usarral/tvcast/internal/i18n"
	"github.com/usarral/tvcast/internal/player"
	"github.com/usarral/tvcast/internal/store"
)

func newServer(t *testing.T, lang i18n.Lang) http.Handler {
	t.Helper()
	st, err := store.Open(t.TempDir(), 1<<20, 1<<30, 0)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: st, Player: player.New(st, time.Hour, 0), Token: "secrettoken", PIN: "123456", Lang: lang}
	return s.Handler()
}

// get makes a request from a phone (not loopback), optionally with a language.
func get(h http.Handler, method, path, acceptLang string) (int, string) {
	r := httptest.NewRequest(method, path, nil)
	r.RemoteAddr = "10.42.0.20:5555"
	if acceptLang != "" {
		r.Header.Set("Accept-Language", acceptLang)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	b, _ := io.ReadAll(w.Result().Body)
	return w.Code, string(b)
}

func TestPagesFollowLanguage(t *testing.T) {
	h := newServer(t, i18n.EN)
	for _, c := range []struct{ accept, want, notWant string }{
		{"", `lang="en"`, "Escribe el código"},
		{"es-ES,es;q=0.9", "Escribe el código", "Type the 6-digit"},
		{"fr-FR", "Type the 6-digit", "Escribe"}, // unsupported -> box default
	} {
		code, body := get(h, "GET", "/", c.accept)
		if code != 200 || !strings.Contains(body, c.want) || strings.Contains(body, c.notWant) {
			t.Errorf("Accept-Language %q: code %d, want %q in page", c.accept, code, c.want)
		}
	}
	// The box default applies when the phone sends no supported language.
	if _, body := get(newServer(t, i18n.ES), "GET", "/", ""); !strings.Contains(body, `lang="es"`) {
		t.Error("box default es not used")
	}
}

func TestIndexPageEmbedsCatalogAsJS(t *testing.T) {
	h := newServer(t, i18n.ES)
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:4000" // loopback: authorized, gets index.html
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	body := w.Body.String()
	if !strings.Contains(body, `"page.sending":"Enviando {i} de {n}…"`) {
		t.Errorf("catalog not embedded as a JS object:\n%s", body[:min(len(body), 2000)])
	}
	if strings.Contains(body, "{{") {
		t.Error("unexpanded template action in page")
	}
}

func TestErrorsAreTranslated(t *testing.T) {
	h := newServer(t, i18n.EN)
	if code, body := get(h, "GET", "/api/state", "es"); code != 401 || !strings.Contains(body, "Escanea el QR") {
		t.Errorf("es: %d %q", code, body)
	}
	if code, body := get(h, "GET", "/api/state", ""); code != 401 || !strings.Contains(body, "Scan the QR") {
		t.Errorf("en: %d %q", code, body)
	}
}
