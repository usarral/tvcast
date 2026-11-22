// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 usarral

package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/usarral/tvcast/internal/admin"
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

type testBox struct{ *player.Player }

func (testBox) DeleteAll() (int, error) { return 0, nil }

func local(h http.Handler, method, path string) int {
	r := httptest.NewRequest(method, path, nil)
	r.RemoteAddr = "127.0.0.1:4444"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code
}

func TestKeyEndpoint(t *testing.T) {
	st, err := store.Open(t.TempDir(), 1<<20, 1<<30, 0)
	if err != nil {
		t.Fatal(err)
	}
	pl := player.New(st, time.Hour, 0)
	s := &Server{Store: st, Player: pl, Token: "secrettoken", Lang: i18n.EN}
	s.Admin = &admin.Controller{Box: testBox{pl}, Lang: i18n.EN, IdleClose: time.Hour}
	h := s.Handler()

	if c, _ := get(h, "POST", "/api/tv/key?k=ok", ""); c != 403 {
		t.Fatalf("phone pressed a key: %d", c)
	}
	if c := local(h, "POST", "/api/tv/key?k=mic"); c != 400 {
		t.Fatalf("unknown key: %d", c)
	}
	if c := local(h, "POST", "/api/tv/key?k=ok"); c != 204 || !pl.Paused() {
		t.Fatalf("OK should pause: %d %v", c, pl.Paused())
	}
	if c := local(h, "GET", "/api/tv/admin.png"); c != 404 {
		t.Fatalf("panel image while closed: %d", c)
	}
	if c := local(h, "POST", "/api/tv/key?k=admin"); c != 204 || s.snapshot().Admin == 0 {
		t.Fatalf("admin key: %d, rev %d", c, s.snapshot().Admin)
	}
	if c := local(h, "GET", "/api/tv/admin.png"); c != 200 {
		t.Fatalf("panel image while open: %d", c)
	}
	if c := local(h, "POST", "/api/tv/remote?connected=1&battery=77"); c != 204 {
		t.Fatalf("remote status: %d", c)
	}
	local(h, "POST", "/api/tv/key?k=home")
	if s.snapshot().Admin != 0 {
		t.Fatal("Home should close the panel")
	}
}

// A local `const t = ...` inside the page's render loop once shadowed the
// translation function t() and made the photo grid come out empty.
func TestIndexDoesNotShadowTranslate(t *testing.T) {
	b, err := static.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(b), "const t = "); n != 1 {
		t.Fatalf("index.html declares const t %d times, want 1 (the translate function)", n)
	}
}
