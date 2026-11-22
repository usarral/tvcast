// Package player decides what the TV shows: whatever is sent last is shown
// right away and, when nothing new arrives, items rotate as a slideshow.
//
// Also:
//   - every QREvery automatic slideshow photos it inserts the QR screen for
//     one interval, so people can always find out how to send;
//   - HoldQR pins the QR screen no matter what (keyboard shortcut on the box)
//     until released, even if new photos arrive.
package player

import (
	"log"
	"sync"
	"time"

	"github.com/usarral/tvcast/internal/store"
)

// Display is the physical screen (mpv) or the web viewer (/tv).
// item == nil means "waiting screen" (QR).
type Display interface {
	Show(item *store.Item) error
}

// State is what the phone and the web viewer see.
type State struct {
	CurrentID string `json:"currentId"`
	Paused    bool   `json:"paused"`
	Interval  int    `json:"intervalSec"`
	QRHeld    bool   `json:"qrHeld"`
	QRSlide   bool   `json:"qrSlide"` // the QR screen inserted in the slideshow
}

type Player struct {
	st       *store.Store
	interval time.Duration
	qrEvery  int

	mu       sync.Mutex
	displays []Display
	current  string // shown ID ("" = QR screen)
	paused   bool
	held     bool   // QR pinned by shortcut
	qrSlide  bool   // inserted QR; when done, continue from resume
	resume   string // ID to continue from after the QR
	auto     int    // photos advanced automatically since the last QR
	shownAt  time.Time
	onChange func(State)
	wake     chan struct{}
}

// New: qrEvery = 0 disables the inserted QR.
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

// Run advances the slideshow. Images last `interval`; videos advance when
// the display reports Ended().
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
	if p.qrSlide { // the inserted QR is over: continue the slideshow
		p.qrSlide = false
		p.mu.Unlock()
		p.step(+1)
		return
	}
	cur := p.current
	p.mu.Unlock()
	if cur == "" {
		return // waiting screen, no photos
	}
	if it, ok := p.st.Get(cur); !ok || it.Kind != store.KindImage {
		return // videos advance with Ended()
	}
	p.autoNext()
}

// autoNext advances the slideshow and inserts the QR when due.
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

// Show shows a given item (e.g. just uploaded).
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

func (p *Player) SetPaused(v bool) { p.setPaused(func(bool) bool { return v }) }

// TogglePause flips pause atomically (the remote's OK and Play/Pause).
func (p *Player) TogglePause() { p.setPaused(func(was bool) bool { return !was }) }

func (p *Player) setPaused(f func(was bool) bool) {
	p.mu.Lock()
	p.paused = f(p.paused)
	p.shownAt = time.Now()
	s, cb := p.stateLocked(), p.onChange
	p.mu.Unlock()
	if cb != nil {
		cb(s)
	}
}

// HoldQR pins (true) or releases (false) the QR screen.
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

func (p *Player) Paused() bool { p.mu.Lock(); defer p.mu.Unlock(); return p.paused }

// Ended is called by a display when a video finishes.
func (p *Player) Ended(id string) {
	p.mu.Lock()
	cur, paused, held := p.current, p.paused, p.held
	p.mu.Unlock()
	if id != cur || held {
		return // stale notice, already moved on
	}
	if paused {
		if it, ok := p.st.Get(cur); ok {
			p.show(&it, true) // repeat
		}
		return
	}
	p.autoNext()
}

// Refresh is called when the list changes (deletion).
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

// Redraw repaints what is on screen (e.g. the IP changed and there is a new QR).
// A video is not restarted: its corner QR is updated on the next item.
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
		next = len(items) - 1 // the most recent
	case idx < 0:
		next = 0
	default:
		next = (idx + dir + len(items)) % len(items)
	}
	p.show(&items[next], true)
}

// show changes what is on screen. manual=true (upload, buttons) cancels the
// inserted QR. With the QR pinned it only records where to continue later.
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

// render paints on every display regardless of the mode.
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
			log.Printf("display: %v", err)
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
