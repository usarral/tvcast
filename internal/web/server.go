// Package web sirve la página del móvil, la API y el visor web /tv.
package web

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/usarral/tvcast/internal/player"
	"github.com/usarral/tvcast/internal/store"
)

//go:embed static
var static embed.FS

const cookieName = "tvcast"

type Server struct {
	Store     *store.Store
	Player    *player.Player
	Token     string // vacío = sin token (solo desarrollo)
	IdlePath  func() string
	CornerPNG func() string
	PIN       string // código corto alternativo al QR (vacío = desactivado)
	MaxUpload int64

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
	// Atajos de teclado de la caja (triggerhappy -> curl a 127.0.0.1).
	mux.HandleFunc("POST /api/tv/qr", s.local(s.holdQR))
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

// Notify reparte estado y lista a móviles y visores web conectados.
func (s *Server) Notify() {
	b, _ := json.Marshal(s.snapshot())
	s.hub.broadcast(b)
}

type snapshot struct {
	State player.State `json:"state"`
	Items []store.Item `json:"items"`
}

func (s *Server) snapshot() snapshot {
	return snapshot{State: s.Player.State(), Items: s.Store.List()}
}

// --- autenticación ---------------------------------------------------------
//
// El QR de la tele lleva ?k=TOKEN. La primera visita guarda el token en una
// cookie y redirige a "/" para que no quede en el historial. Así solo puede
// enviar quien ve la tele, aunque la caja esté en la wifi de casa.

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
			http.Error(w, "escanea el QR de la tele", http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

// local: solo desde la propia caja (el visor /tv corre en ella).
func (s *Server) local(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isLoopback(r) {
			http.Error(w, "solo desde la caja", http.StatusForbidden)
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
	return func(w http.ResponseWriter, r *http.Request) {
		b, err := static.ReadFile(name)
		if err != nil {
			http.Error(w, "no encontrado", 404)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(b)
	}
}

// login con el código corto que sale en la tele. Límite global de intentos
// fallidos para que no se pueda probar el millón de combinaciones.
const (
	pinMaxFails = 10
	pinWindow   = 10 * time.Minute
)

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if s.Token == "" || s.PIN == "" {
		http.Error(w, "no disponible", 404)
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
		http.Error(w, "Demasiados intentos. Espera unos minutos o escanea el QR.", http.StatusTooManyRequests)
		return
	}
	pin := strings.TrimSpace(r.FormValue("pin"))
	if subtle.ConstantTimeCompare([]byte(pin), []byte(s.PIN)) != 1 {
		s.pinMu.Lock()
		s.pinFails = append(s.pinFails, now)
		s.pinMu.Unlock()
		log.Printf("código incorrecto desde %s", r.RemoteAddr)
		http.Error(w, "Código incorrecto", http.StatusForbidden)
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

// holdQR: ?v=1 fija la pantalla de QR, ?v=0 vuelve al pase, ?v=toggle alterna.
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

// --- API -------------------------------------------------------------------

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.snapshot())
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	// Margen para la miniatura y las cabeceras multipart.
	r.Body = http.MaxBytesReader(w, r.Body, s.MaxUpload+1<<20)
	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "se esperaba multipart/form-data", 400)
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
			uploadError(w, err)
			return
		}
		switch part.FormName() {
		case "thumb":
			// La miniatura va ANTES que el fichero en el formulario.
			thumb, err = io.ReadAll(io.LimitReader(part, 512<<10))
			if err != nil {
				uploadError(w, err)
				return
			}
		case "file":
			it, err := s.Store.Save(part.FileName(), part, thumb)
			thumb = nil
			if err != nil {
				uploadError(w, err)
				return
			}
			log.Printf("subido %s %s %d bytes desde %s", it.ID, it.Mime, it.Size, r.RemoteAddr)
			saved = append(saved, it)
		}
		drain(part)
	}
	if len(saved) == 0 {
		http.Error(w, "no llegó ningún fichero", 400)
		return
	}
	// "Enviar a la tele": lo último que llega se muestra ya.
	s.Player.Show(saved[len(saved)-1].ID)
	s.Notify()
	writeJSON(w, 201, saved)
}

func drain(p *multipart.Part) { io.Copy(io.Discard, io.LimitReader(p, 1<<20)) }

func uploadError(w http.ResponseWriter, err error) {
	var mbe *http.MaxBytesError
	switch {
	case errors.As(err, &mbe), errors.Is(err, store.ErrTooLarge):
		http.Error(w, "demasiado grande", http.StatusRequestEntityTooLarge)
	case errors.Is(err, store.ErrHEIC), errors.Is(err, store.ErrUnsupported):
		http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
	default:
		log.Printf("subida: %v", err)
		http.Error(w, "error al guardar", 500)
	}
}

func (s *Server) del(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !store.ValidID(id) {
		http.Error(w, "id no válido", 400)
		return
	}
	if err := s.Store.Delete(id); err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	s.Player.Refresh()
	s.Notify()
	w.WriteHeader(204)
}

func (s *Server) showItem(w http.ResponseWriter, r *http.Request) {
	if !s.Player.Show(r.PathValue("id")) {
		http.Error(w, "no existe", 404)
		return
	}
	w.WriteHeader(204)
}

func (s *Server) pause(w http.ResponseWriter, r *http.Request) {
	s.Player.SetPaused(r.URL.Query().Get("v") != "0")
	w.WriteHeader(204)
}

func (s *Server) media(thumb bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !store.ValidID(id) {
			http.Error(w, "id no válido", 400)
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
		http.ServeFile(w, r, p) // soporta Range (necesario para <video>)
	}
}

// --- eventos (SSE) ----------------------------------------------------------

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "sin streaming", 500)
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
		default: // cliente lento: se pierde este aviso, recibirá el siguiente
		}
	}
}

// --- utilidades -------------------------------------------------------------

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
