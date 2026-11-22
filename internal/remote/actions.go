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

// DefaultKeymap maps buttons to calls on tvcast's local API (POST, from
// 127.0.0.1, so no token is needed). Several paths run in order.
//
// Left out on purpose:
//   - Power: no console or restart from the remote. Those are maintenance
//     actions for whoever installs the box (Ctrl+Alt+C / Ctrl+Alt+T on a
//     keyboard); one accidental press would leave the TV on a login prompt.
//   - Up, Down, Heart, Vol+/-, Mic: nothing obvious to do yet. They are
//     logged, so a future mapping can be tried without new captures.
var DefaultKeymap = map[Button][]string{
	Right:     {"/api/next"},
	Left:      {"/api/prev"},
	OK:        {"/api/pause?v=toggle"},
	PlayPause: {"/api/pause?v=toggle"},
	Home:      {"/api/tv/qr?v=toggle"},
	// Back: "get me out of whatever this is": drop the pinned QR and resume.
	Back: {"/api/tv/qr?v=0", "/api/pause?v=0"},
}

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
