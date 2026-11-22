// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 usarral

package remote

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// DefaultKeymap sends every button to tvcast's /api/tv/key (POST, from
// 127.0.0.1, so no token is needed). tvcast decides what each key does: the
// everyday actions (Right/Left photo, OK and Play/Pause pause, Home QR, Back
// slideshow), the secret sequence that opens the admin panel, and the panel's
// navigation while it is open. Keeping that logic in tvcast means the
// keyboard (triggerhappy) and the remote behave the same.
//
// Mic is left out: the remote sends bursts of Mic after reconnecting.
var DefaultKeymap = func() map[Button][]string {
	m := map[Button][]string{}
	for b := Up; b <= VolDown; b++ {
		m[b] = []string{"/api/tv/key?k=" + b.String()}
	}
	return m
}()

// API calls tvcast's HTTP API.
type API struct {
	Base   string // e.g. http://127.0.0.1
	Client *http.Client
}

func (a *API) Post(ctx context.Context, path string) error {
	c := a.Client
	if c == nil {
		c = &http.Client{Timeout: 3 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(a.Base, "/")+path, nil)
	if err != nil {
		return err
	}
	res, err := c.Do(req)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, io.LimitReader(res.Body, 4<<10))
	res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("POST %s: %s", path, res.Status)
	}
	return nil
}

// Poster is what the dispatcher needs from the API (faked in tests).
type Poster interface {
	Post(ctx context.Context, path string) error
}

// Dispatcher acts on presses. Releases are ignored: every press is one action,
// and the remote sends no auto-repeat while a key is held.
type Dispatcher struct {
	API    Poster
	Keymap map[Button][]string
	Log    *log.Logger
}

// Handle decodes one notification and runs its action. API errors are logged
// and dropped: if tvcast is restarting, the next press will work.
func (d *Dispatcher) Handle(ctx context.Context, n Notification) {
	ev := Decode(n)
	switch {
	case ev.Unknown:
		d.Log.Printf("remote: unknown notification: %v", n)
		return
	case ev.Release:
		return
	}
	paths := d.Keymap[ev.Button]
	if len(paths) == 0 {
		d.Log.Printf("remote: %v (not mapped)", ev.Button)
		return
	}
	d.Log.Printf("remote: %v", ev.Button)
	for _, p := range paths {
		if err := d.API.Post(ctx, p); err != nil {
			d.Log.Printf("remote: %v: %v", ev.Button, err)
			return
		}
	}
}
