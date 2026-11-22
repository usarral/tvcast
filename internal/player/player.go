// Package player decide qué se ve en la tele: lo último que se envía se
// muestra al momento y, si no hay novedades, rota en pase de diapositivas.
//
// Además:
//   - cada QREvery fotos del pase automático intercala la pantalla de QR
//     durante un intervalo, para que siempre se pueda descubrir cómo enviar;
//   - HoldQR fija la pantalla de QR "sí o sí" (atajo de teclado en la caja)
//     hasta que se quite, aunque lleguen fotos nuevas.
package player

import (
	"log"
	"sync"
	"time"

	"github.com/usarral/tvcast/internal/store"
)

// Display es la pantalla física (mpv) o el visor web (/tv).
// item == nil significa "pantalla de espera" (QR).
type Display interface {
	Show(item *store.Item) error
}

// State es lo que ven el móvil y el visor web.
type State struct {
	CurrentID string `json:"currentId"`
	Paused    bool   `json:"paused"`
	Interval  int    `json:"intervalSec"`
	QRHeld    bool   `json:"qrHeld"`
	QRSlide   bool   `json:"qrSlide"` // la pantalla de QR intercalada en el pase
}

type Player struct {
	st       *store.Store
	interval time.Duration
	qrEvery  int

	mu       sync.Mutex
	displays []Display
	current  string // ID mostrado ("" = pantalla de QR)
	paused   bool
	held     bool   // QR fijo por atajo
	qrSlide  bool   // QR intercalado; al acabar se sigue desde resume
	resume   string // ID desde el que seguir tras el QR
	auto     int    // fotos pasadas automáticamente desde el último QR
	shownAt  time.Time
	onChange func(State)
	wake     chan struct{}
}

// New: qrEvery = 0 desactiva el QR intercalado.
func New(st *store.Store, interval time.Duration, qrEvery int) *Player {
	return &Player{st: st, interval: interval, qrEvery: qrEvery, wake: make(chan struct{}, 1)}
}

func (p *Player) AddDisplay(d Display) {
	p.mu.Lock()
	p.displays = append(p.displays, d)
	p.mu.Unlock()
}
func (p *Player) OnChange(f func(State)) { p.mu.Lock(); p.onChange = f; p.mu.Unlock() }
func (p *Player) State() State           { p.mu.Lock(); defer p.mu.Unlock(); return p.stateLocked() }
func (p *Player) stateLocked() State {
	return State{CurrentID: p.current, Paused: p.paused, Interval: int(p.interval / time.Second),
		QRHeld: p.held, QRSlide: p.qrSlide}
}

// Run hace avanzar el pase. Las imágenes duran `interval`; los vídeos
// avanzan cuando la pantalla avisa con Ended().
func (p *Player) Run(stop <-chan struct{}) {
	p.Refresh()
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-p.wake:
		case <-t.C:
		}
		p.tick()
	}
}

func (p *Player) tick() {
	p.mu.Lock()
	if p.held || p.paused || time.Since(p.shownAt) < p.interval {
		p.mu.Unlock()
		return
	}
	if p.qrSlide { // terminó el QR intercalado: seguir el pase
		p.qrSlide = false
		p.mu.Unlock()
		p.step(+1)
		return
	}
	cur := p.current
	p.mu.Unlock()
	if cur == "" {
		return // pantalla de espera sin fotos
	}
	if it, ok := p.st.Get(cur); !ok || it.Kind != store.KindImage {
		return // los vídeos avanzan con Ended()
	}
	p.autoNext()
}

// autoNext avanza el pase automático e intercala el QR cuando toca.
func (p *Player) autoNext() {
	p.mu.Lock()
	p.auto++
	if p.qrEvery > 0 && p.auto >= p.qrEvery {
		p.auto = 0
		p.qrSlide = true
		p.resume = p.current
		p.mu.Unlock()
		p.show(nil, false)
		return
	}
	p.mu.Unlock()
	p.step(+1)
}

// Show pone un elemento concreto (p. ej. recién subido).
func (p *Player) Show(id string) bool {
	it, ok := p.st.Get(id)
	if !ok {
		return false
	}
	p.show(&it, true)
	return true
}

func (p *Player) Next() { p.step(+1) }
func (p *Player) Prev() { p.step(-1) }

func (p *Player) SetPaused(v bool) {
	p.mu.Lock()
	p.paused = v
	p.shownAt = time.Now()
	s, f := p.stateLocked(), p.onChange
	p.mu.Unlock()
	if f != nil {
		f(s)
	}
}

// HoldQR fija (true) o suelta (false) la pantalla de QR.
func (p *Player) HoldQR(v bool) {
	p.mu.Lock()
	was := p.held
	p.held = v
	if v && !was {
		p.resume = p.current
	}
	p.mu.Unlock()
	if v {
		p.render(nil)
		return
	}
	if was {
		p.mu.Lock()
		resume := p.resume
		p.mu.Unlock()
		if it, ok := p.st.Get(resume); ok {
			p.show(&it, true)
		} else {
			p.step(+1)
		}
	}
}

func (p *Player) Held() bool { p.mu.Lock(); defer p.mu.Unlock(); return p.held }

// Ended lo llama una pantalla cuando termina un vídeo.
func (p *Player) Ended(id string) {
	p.mu.Lock()
	cur, paused, held := p.current, p.paused, p.held
	p.mu.Unlock()
	if id != cur || held {
		return // aviso viejo, ya se cambió
	}
	if paused {
		if it, ok := p.st.Get(cur); ok {
			p.show(&it, true) // repetir
		}
		return
	}
	p.autoNext()
}

// Refresh se llama cuando cambia la lista (borrado).
func (p *Player) Refresh() {
	p.mu.Lock()
	cur, slide := p.current, p.qrSlide
	p.mu.Unlock()
	if cur != "" || slide {
		if _, ok := p.st.Get(cur); ok || slide {
			return
		}
	}
	p.step(+1)
}

// Redraw vuelve a pintar lo que se ve (p. ej. cambió la IP y hay QR nuevo).
// Un vídeo no se reinicia: su QR de esquina se actualizará en el siguiente.
func (p *Player) Redraw() {
	p.mu.Lock()
	cur, held := p.current, p.held
	p.mu.Unlock()
	if cur == "" || held {
		p.render(nil)
		return
	}
	if it, ok := p.st.Get(cur); ok && it.Kind == store.KindImage {
		p.render(&it)
	}
}

func (p *Player) step(dir int) {
	items := p.st.List()
	if len(items) == 0 {
		p.show(nil, true)
		return
	}
	p.mu.Lock()
	cur := p.current
	if cur == "" {
		cur = p.resume
	}
	p.mu.Unlock()
	idx := -1
	for i, it := range items {
		if it.ID == cur {
			idx = i
		}
	}
	var next int
	switch {
	case idx < 0 && dir > 0:
		next = len(items) - 1 // lo más reciente
	case idx < 0:
		next = 0
	default:
		next = (idx + dir + len(items)) % len(items)
	}
	p.show(&items[next], true)
}

// show cambia lo que se ve. manual=true (subida, botones) cancela el QR
// intercalado. Con el QR fijo solo se anota dónde seguir después.
func (p *Player) show(it *store.Item, manual bool) {
	p.mu.Lock()
	if p.held {
		if it != nil {
			p.resume = it.ID
		}
		p.mu.Unlock()
		return
	}
	if manual {
		p.qrSlide = false
	}
	p.mu.Unlock()
	p.render(it)
}

// render pinta en todas las pantallas sin mirar el modo.
func (p *Player) render(it *store.Item) {
	p.mu.Lock()
	if it == nil {
		p.current = ""
	} else {
		p.current = it.ID
	}
	p.shownAt = time.Now()
	ds := append([]Display(nil), p.displays...)
	s, f := p.stateLocked(), p.onChange
	p.mu.Unlock()
	for _, d := range ds {
		if err := d.Show(it); err != nil {
			log.Printf("pantalla: %v", err)
		}
	}
	if f != nil {
		f(s)
	}
	select {
	case p.wake <- struct{}{}:
	default:
	}
}
