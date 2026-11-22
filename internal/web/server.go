// Package web serves the phone page, the API and the /tv web viewer.
package web

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"image"
	"image/png"
	"io"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/usarral/tvcast/internal/admin"
	"github.com/usarral/tvcast/internal/i18n"
	"github.com/usarral/tvcast/internal/player"
	"github.com/usarral/tvcast/internal/store"
)

//go:embed static
var static embed.FS

// The pages are html/template files: {{index .S "key"}} in HTML and the whole
// catalog as a JS object ({{.S}}) inside <script>, escaped by the template.
var pages = template.Must(template.ParseFS(static, "static/*.html"))

const cookieName = "tvcast"

type Server struct {
	Store     *store.Store
	Player    *player.Player
	Token     string // empty = no token (development only)
	IdlePath  func() string
	CornerPNG func() string
	PIN       string // short code as an alternative to the QR (empty = disabled)
	MaxUpload int64
	// Lang is the default language of the phone pages and errors; the
	// phone's own language (Accept-Language) wins when it is supported.
	Lang i18n.Lang
	// Admin receives the keys of the keyboard and the remote (/api/tv/key)
	// and owns the hidden admin panel. nil = /api/tv/key is not served.
	Admin *admin.Controller

	hub hub

	pinMu    sync.Mutex
	pinFails []time.Time
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /tv", s.local(s.page("static/tv.html")))
	mux.HandleFunc("GET /api/state", s.auth(s.state))
	mux.HandleFunc("POST /api/upload", s.auth(s.upload))
	mux.HandleFunc("DELETE /api/items/{id}", s.auth(s.del))
	mux.HandleFunc("POST /api/show/{id}", s.auth(s.showItem))
	mux.HandleFunc("POST /api/next", s.auth(func(w http.ResponseWriter, r *http.Request) { s.Player.Next(); w.WriteHeader(204) }))
	mux.HandleFunc("POST /api/prev", s.auth(func(w http.ResponseWriter, r *http.Request) { s.Player.Prev(); w.WriteHeader(204) }))
	mux.HandleFunc("POST /api/pause", s.auth(s.pause))
	mux.HandleFunc("GET /api/events", s.auth(s.events))
	mux.HandleFunc("POST /api/tv/ended/{id}", s.local(func(w http.ResponseWriter, r *http.Request) {
		s.Player.Ended(r.PathValue("id"))
		w.WriteHeader(204)
	}))
	mux.HandleFunc("GET /media/{id}", s.auth(s.media(false)))
	mux.HandleFunc("GET /thumb/{id}", s.auth(s.media(true)))
	mux.HandleFunc("POST /api/login", s.login)
	// Keyboard shortcuts on the box (triggerhappy -> curl to 127.0.0.1).
	mux.HandleFunc("POST /api/tv/qr", s.local(s.holdQR))
	// Every key from the keyboard (triggerhappy) and the remote (tvcast-remote).
	mux.HandleFunc("POST /api/tv/key", s.local(s.key))
	mux.HandleFunc("POST /api/tv/remote", s.local(s.remoteStatus))
	mux.HandleFunc("GET /api/tv/admin.png", s.local(s.adminPNG))
	mux.HandleFunc("GET /corner.png", s.auth(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, s.CornerPNG())
	}))
	mux.HandleFunc("GET /idle.png", s.auth(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, s.IdlePath())
	}))
	return secHeaders(mux)
}

// Notify sends the state and list to connected phones and web viewers.
func (s *Server) Notify() {
	b, _ := json.Marshal(s.snapshot())
	s.hub.broadcast(b)
}

type snapshot struct {
	State player.State `json:"state"`
	Items []store.Item `json:"items"`
	Admin int          `json:"admin,omitempty"` // admin panel revision, 0 = closed
}

func (s *Server) snapshot() snapshot {
	sn := snapshot{State: s.Player.State(), Items: s.Store.List()}
	if s.Admin != nil {
		sn.Admin = s.Admin.Rev()
	}
	return sn
}

// --- authentication ----------------------------------------------------------
//
// The TV's QR carries ?k=TOKEN. The first visit stores the token in a cookie
// and redirects to "/" so it doesn't stay in the history. That way only
// someone who can see the TV can send, even when the box is on the home wifi.

func (s *Server) authorized(r *http.Request) bool {
	if s.Token == "" || isLoopback(r) {
		return true
	}
	c, err := r.Cookie(cookieName)
	return err == nil && subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.Token)) == 1
}

func (s *Server) auth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			s.fail(w, r, http.StatusUnauthorized, "err.scan_qr")
			return
		}
		h(w, r)
	}
}

// local: only from the box itself (the /tv viewer runs on it).
func (s *Server) local(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isLoopback(r) {
			s.fail(w, r, http.StatusForbidden, "err.local_only")
			return
		}
		h(w, r)
	}
}

func isLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if k := r.URL.Query().Get("k"); k != "" && s.Token != "" {
		if subtle.ConstantTimeCompare([]byte(k), []byte(s.Token)) == 1 {
			s.setCookie(w)
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
	}
	if !s.authorized(r) {
		s.page("static/locked.html")(w, r)
		return
	}
	s.page("static/index.html")(w, r)
}

func (s *Server) page(name string) http.HandlerFunc {
	name = strings.TrimPrefix(name, "static/")
	return func(w http.ResponseWriter, r *http.Request) {
		lang := s.lang(r)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Language", string(lang))
		w.Header().Add("Vary", "Accept-Language")
		data := struct {
			Lang i18n.Lang
			S    map[string]string
		}{lang, i18n.Strings(lang)}
		if err := pages.ExecuteTemplate(w, name, data); err != nil {
			log.Printf("page %s: %v", name, err)
		}
	}
}

// lang: the phone's language if supported, else the box's default.
func (s *Server) lang(r *http.Request) i18n.Lang { return i18n.FromRequest(r, s.Lang) }

// fail answers with a translated plain-text error.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, code int, key string) {
	http.Error(w, i18n.T(s.lang(r), key), code)
}

// login with the short code shown on the TV. Global limit on failed attempts
// so nobody can try the million combinations.
const (
	pinMaxFails = 10
	pinWindow   = 10 * time.Minute
)

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if s.Token == "" || s.PIN == "" {
		s.fail(w, r, 404, "err.unavailable")
		return
	}
	s.pinMu.Lock()
	now := time.Now()
	kept := s.pinFails[:0]
	for _, t := range s.pinFails {
		if now.Sub(t) < pinWindow {
			kept = append(kept, t)
		}
	}
	s.pinFails = kept
	blocked := len(s.pinFails) >= pinMaxFails
	s.pinMu.Unlock()
	if blocked {
		s.fail(w, r, http.StatusTooManyRequests, "err.too_many")
		return
	}
	pin := strings.TrimSpace(r.FormValue("pin"))
	if subtle.ConstantTimeCompare([]byte(pin), []byte(s.PIN)) != 1 {
		s.pinMu.Lock()
		s.pinFails = append(s.pinFails, now)
		s.pinMu.Unlock()
		log.Printf("wrong code from %s", r.RemoteAddr)
		s.fail(w, r, http.StatusForbidden, "err.bad_code")
		return
	}
	s.setCookie(w)
	w.WriteHeader(204)
}

func (s *Server) setCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: s.Token, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteStrictMode, MaxAge: 365 * 24 * 3600,
	})
}

// holdQR: ?v=1 pins the QR screen, ?v=0 goes back to the slideshow, ?v=toggle switches.
func (s *Server) holdQR(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Query().Get("v") {
	case "1":
		s.Player.HoldQR(true)
	case "0":
		s.Player.HoldQR(false)
	case "toggle", "":
		s.Player.HoldQR(!s.Player.Held())
	default:
		http.Error(w, "v=1|0|toggle", 400)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) key(w http.ResponseWriter, r *http.Request) {
	if s.Admin == nil {
		http.Error(w, "no admin", 404)
		return
	}
	k, ok := admin.ParseKey(r.URL.Query().Get("k"))
	if !ok {
		http.Error(w, "unknown key", 400)
		return
	}
	s.Admin.Key(k)
	w.WriteHeader(204)
}

// remoteStatus: tvcast-remote reports connected=0|1 and battery=0..100.
func (s *Server) remoteStatus(w http.ResponseWriter, r *http.Request) {
	if s.Admin == nil {
		w.WriteHeader(204)
		return
	}
	bat, err := strconv.Atoi(r.URL.Query().Get("battery"))
	if err != nil || bat < 0 || bat > 100 {
		bat = -1
	}
	s.Admin.SetRemote(r.URL.Query().Get("connected") == "1", bat)
	w.WriteHeader(204)
}

// adminPNG is the panel for the /tv web viewer (mpv gets it as an overlay).
func (s *Server) adminPNG(w http.ResponseWriter, r *http.Request) {
	var img image.Image
	if s.Admin != nil {
		if i := s.Admin.Image(); i != nil {
			img = i
		}
	}
	if img == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	png.Encode(w, img)
}

// --- API -------------------------------------------------------------------

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.snapshot())
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	// Room for the thumbnail and the multipart headers.
	r.Body = http.MaxBytesReader(w, r.Body, s.MaxUpload+1<<20)
	mr, err := r.MultipartReader()
	if err != nil {
		s.fail(w, r, 400, "err.multipart")
		return
	}
	var thumb []byte
	var saved []store.Item
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			s.uploadError(w, r, err)
			return
		}
		switch part.FormName() {
		case "thumb":
			// The thumbnail comes BEFORE the file in the form.
			thumb, err = io.ReadAll(io.LimitReader(part, 512<<10))
			if err != nil {
				s.uploadError(w, r, err)
				return
			}
		case "file":
			it, err := s.Store.Save(part.FileName(), part, thumb)
			thumb = nil
			if err != nil {
				s.uploadError(w, r, err)
				return
			}
			log.Printf("uploaded %s %s %d bytes from %s", it.ID, it.Mime, it.Size, r.RemoteAddr)
			saved = append(saved, it)
		}
		drain(part)
	}
	if len(saved) == 0 {
		s.fail(w, r, 400, "err.no_file")
		return
	}
	// "Send to the TV": whatever arrives last is shown right away.
	s.Player.Show(saved[len(saved)-1].ID)
	s.Notify()
	writeJSON(w, 201, saved)
}

func drain(p *multipart.Part) { io.Copy(io.Discard, io.LimitReader(p, 1<<20)) }

func (s *Server) uploadError(w http.ResponseWriter, r *http.Request, err error) {
	var mbe *http.MaxBytesError
	switch {
	case errors.As(err, &mbe), errors.Is(err, store.ErrTooLarge):
		s.fail(w, r, http.StatusRequestEntityTooLarge, "err.too_large")
	case errors.Is(err, store.ErrHEIC):
		s.fail(w, r, http.StatusUnsupportedMediaType, "err.heic")
	case errors.Is(err, store.ErrUnsupported):
		s.fail(w, r, http.StatusUnsupportedMediaType, "err.unsupported")
	default:
		log.Printf("upload: %v", err)
		s.fail(w, r, 500, "err.save")
	}
}

func (s *Server) del(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !store.ValidID(id) {
		s.fail(w, r, 400, "err.bad_id")
		return
	}
	if err := s.Store.Delete(id); err != nil {
		s.fail(w, r, 404, "err.not_found")
		return
	}
	s.Player.Refresh()
	s.Notify()
	w.WriteHeader(204)
}

func (s *Server) showItem(w http.ResponseWriter, r *http.Request) {
	if !s.Player.Show(r.PathValue("id")) {
		s.fail(w, r, 404, "err.not_found")
		return
	}
	w.WriteHeader(204)
}

func (s *Server) pause(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Query().Get("v") {
	case "toggle":
		s.Player.TogglePause()
	case "0":
		s.Player.SetPaused(false)
	default: // "1" or nothing, as before
		s.Player.SetPaused(true)
	}
	w.WriteHeader(204)
}

func (s *Server) media(thumb bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !store.ValidID(id) {
			s.fail(w, r, 400, "err.bad_id")
			return
		}
		it, ok := s.Store.Get(id)
		if !ok || (thumb && !it.HasThumb) {
			http.NotFound(w, r)
			return
		}
		p, ct := s.Store.Path(&it), it.Mime
		if thumb {
			p, ct = s.Store.ThumbPath(&it), "image/jpeg"
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Cache-Control", "private, max-age=86400, immutable")
		http.ServeFile(w, r, p) // supports Range (needed by <video>)
	}
}

// --- events (SSE) -------------------------------------------------------------

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		s.fail(w, r, 500, "err.no_stream")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	ch := s.hub.add()
	defer s.hub.remove(ch)
	b, _ := json.Marshal(s.snapshot())
	fmt.Fprintf(w, "data: %s\n\n", b)
	fl.Flush()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			fl.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		}
	}
}

type hub struct {
	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

func (h *hub) add() chan []byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs == nil {
		h.subs = map[chan []byte]struct{}{}
	}
	ch := make(chan []byte, 8)
	h.subs[ch] = struct{}{}
	return ch
}

func (h *hub) remove(ch chan []byte) { h.mu.Lock(); delete(h.subs, ch); h.mu.Unlock() }

func (h *hub) broadcast(b []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- b:
		default: // slow client: it misses this update and gets the next one
		}
	}
}

// --- helpers ------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func secHeaders(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' blob: data:; media-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		h.ServeHTTP(w, r)
	})
}
