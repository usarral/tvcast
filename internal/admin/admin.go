// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 usarral

package admin

import (
	"fmt"
	"image"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/usarral/tvcast/internal/i18n"
)

// Box is what the panel and the plain keys do to the slideshow.
type Box interface {
	Next()
	Prev()
	TogglePause()
	SetPaused(bool)
	Paused() bool
	HoldQR(bool)
	Held() bool
	DeleteAll() (int, error)
}

// System runs what needs the system: privileged actions (as root, through
// systemd; see deploy/admin) and reading logs.
type System interface {
	Run(verb string) error
	Log(which string, n int) ([]string, error)
}

// Screen shows the panel on top of whatever is on the TV.
type Screen interface {
	ShowPanel(img *image.RGBA) error
	HidePanel() error
}

// Remote is what tvcast-remote last reported.
type Remote struct {
	Connected bool
	Battery   int // percent, -1 = unknown
	Seen      time.Time
}

// Controller receives every key (keyboard and remote) through /api/tv/key.
// With the panel closed, keys do their everyday job and the secret sequence
// opens the panel; with it open, keys drive the menu.
type Controller struct {
	Box    Box
	Sys    System
	Screen Screen // nil: only the web viewer shows the panel
	Lang   i18n.Lang
	// Status returns the lines at the top of the main menu.
	Status func(Remote) []string
	// IdleClose closes the panel after this long without keys (0 = 2 min),
	// so the TV never stays on the panel by mistake.
	IdleClose time.Duration
	OnChange  func() // repaint web viewers
	Now       func() time.Time

	mu        sync.Mutex
	st        *State
	open      bool
	seq       seq
	rev       int
	pubRev    atomic.Int64 // rev while open, 0 closed; read without mu (see Rev)
	img       *image.RGBA
	wasPaused bool // slideshow state to restore on close
	timer     *time.Timer
	remote    Remote
}

func (c *Controller) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Key handles one key.
func (c *Controller) Key(k Key) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.open {
		if k == KAdmin || c.seq.push(k, c.now()) {
			c.openLocked()
			return
		}
		c.everyday(k)
		return
	}
	c.resetTimerLocked()
	c.st.Message = ""
	action, close := c.st.Key(k)
	if close {
		c.closeLocked()
		return
	}
	if action != "" {
		if c.run(action) {
			return // the action closed the panel
		}
	}
	c.drawLocked()
}

// everyday is what keys do with the panel closed (same as before the panel).
func (c *Controller) everyday(k Key) {
	switch k {
	case KRight:
		c.Box.Next()
	case KLeft:
		c.Box.Prev()
	case KOK, KPlayPause:
		c.Box.TogglePause()
	case KHome:
		c.Box.HoldQR(!c.Box.Held())
	case KBack:
		c.Box.HoldQR(false)
		c.Box.SetPaused(false)
	}
}

// run executes a menu action. It returns true if the panel was closed.
func (c *Controller) run(action string) bool {
	T := func(k string) string { return i18n.T(c.Lang, k) }
	switch {
	case action == "qr":
		on := !c.Box.Held()
		c.Box.HoldQR(on)
		c.st.Message = T(map[bool]string{true: "adm.msg_qr_on", false: "adm.msg_qr_off"}[on])
	case action == "pause":
		c.wasPaused = !c.wasPaused
		c.st.Message = T(map[bool]string{true: "adm.msg_paused", false: "adm.msg_resumed"}[c.wasPaused])
	case action == "photos-delete":
		n, err := c.Box.DeleteAll()
		if err != nil {
			c.st.Message = fmt.Sprintf(T("adm.msg_error"), err)
		} else {
			c.st.Message = fmt.Sprintf(T("adm.msg_deleted"), n)
		}
	case strings.HasPrefix(action, "log:"):
		which := strings.TrimPrefix(action, "log:")
		lines, err := c.Sys.Log(which, 22)
		if err != nil {
			lines = append(lines, fmt.Sprintf(T("adm.msg_error"), err))
		}
		c.st.ShowLog("adm.log_"+which, lines)
	case strings.HasPrefix(action, "sys:"):
		verb := strings.TrimPrefix(action, "sys:")
		log.Printf("admin: %s", verb)
		if err := c.Sys.Run(verb); err != nil {
			c.st.Message = fmt.Sprintf(T("adm.msg_error"), err)
			return false
		}
		switch verb {
		case "restart-tvcast", "reboot", "poweroff", "console", "new-code", "lang-es", "lang-en":
			// tvcast is about to stop: leave the screen clean.
			c.closeLocked()
			return true
		}
		c.st.Message = T("adm.msg_started")
	}
	return false
}

func (c *Controller) openLocked() {
	c.open = true
	c.st = newState()
	c.seq = seq{}
	c.wasPaused = c.Box.Paused()
	c.Box.SetPaused(true) // nothing moves (and no video sound) behind the panel
	log.Printf("admin: panel open")
	c.resetTimerLocked()
	c.drawLocked()
}

func (c *Controller) closeLocked() {
	if !c.open {
		return
	}
	c.open = false
	if c.timer != nil {
		c.timer.Stop()
	}
	if c.Screen != nil {
		if err := c.Screen.HidePanel(); err != nil {
			log.Printf("admin: %v", err)
		}
	}
	c.Box.SetPaused(c.wasPaused)
	c.img = nil
	c.rev++
	c.pubRev.Store(0)
	log.Printf("admin: panel closed")
	c.notify()
}

func (c *Controller) resetTimerLocked() {
	d := c.IdleClose
	if d == 0 {
		d = 2 * time.Minute
	}
	if c.timer != nil {
		c.timer.Stop()
	}
	c.timer = time.AfterFunc(d, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.closeLocked()
	})
}

func (c *Controller) drawLocked() {
	var status []string
	if c.Status != nil {
		status = c.Status(c.remote)
	}
	img, err := Render(c.st.View(), status, c.Lang)
	if err != nil {
		log.Printf("admin: render: %v", err)
		return
	}
	c.img = img
	c.rev++
	c.pubRev.Store(int64(c.rev))
	if c.Screen != nil {
		if err := c.Screen.ShowPanel(img); err != nil {
			log.Printf("admin: %v", err)
		}
	}
	c.notify()
}

func (c *Controller) notify() {
	if c.OnChange != nil {
		go c.OnChange()
	}
}

// Open and Close are for tests and the API.
func (c *Controller) Open() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.open {
		c.openLocked()
	}
}

func (c *Controller) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeLocked()
}

// Rev is 0 when closed, and changes on every repaint while open. It doesn't
// take the lock: the player's change callback ends up here while Key holds it.
func (c *Controller) Rev() int { return int(c.pubRev.Load()) }

// Image is the last panel drawn (nil when closed).
func (c *Controller) Image() *image.RGBA {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.img
}

// SetRemote records what tvcast-remote reports; shown in the status.
func (c *Controller) SetRemote(connected bool, battery int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.remote = Remote{Connected: connected, Battery: battery, Seen: c.now()}
	if c.open {
		c.drawLocked()
	}
}

// view is for tests.
func (c *Controller) view() View {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.st == nil {
		return View{}
	}
	return c.st.View()
}
