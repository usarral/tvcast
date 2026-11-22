package player

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/usarral/tvcast/internal/store"
)

// MPV shows photos and videos full screen with mpv, driven through its JSON
// IPC socket. One process for everything: no switching programs between
// photo and video, and mpv uses hardware decoding when available.
type MPV struct {
	Bin      string   // "" = don't launch mpv, just connect to Socket
	Args     []string // extra arguments (e.g. --vo=drm)
	Socket   string
	Path     func(*store.Item) string
	IdlePath func() string
	OnEnded  func(id string)
	// Corner returns the small corner QR as BGRA (ok=false: no QR).
	Corner func() (path string, w, h int, ok bool)
	// PanelDir is where the admin panel image is written (tmpfs).
	PanelDir string

	mu      sync.Mutex
	conn    net.Conn
	reqID   int
	pending map[int]chan mpvResp
	entries map[int64]string // playlist_entry_id -> item ID
	last    *store.Item
	hasLast bool
	gen     int // invalidates pending overlay placements
	panelN  int // alternates the admin panel file
}

type mpvResp struct {
	RequestID int             `json:"request_id"`
	Error     string          `json:"error"`
	Data      json.RawMessage `json:"data"`
	Event     string          `json:"event"`
	Reason    string          `json:"reason"`
	EntryID   int64           `json:"playlist_entry_id"`
}

// mpvSocketWait is how long to wait at most for mpv to create its IPC socket.
const mpvSocketWait = 30 * time.Second

func (m *MPV) Start(stop <-chan struct{}) {
	m.pending = map[int]chan mpvResp{}
	m.entries = map[int64]string{}
	go func() {
		for {
			if err := m.runOnce(stop); err != nil {
				log.Printf("mpv: %v", err)
			}
			select {
			case <-stop:
				return
			case <-time.After(2 * time.Second):
			}
		}
	}()
}

func (m *MPV) runOnce(stop <-chan struct{}) error {
	var cmd *exec.Cmd
	var exited chan struct{} // closed when the mpv process exits
	if m.Bin != "" {
		os.Remove(m.Socket)
		args := append([]string{
			"--idle=yes", "--force-window=yes", "--fullscreen",
			"--image-display-duration=inf", "--keep-open=no",
			"--osc=no", "--osd-level=0", "--cursor-autohide=always",
			"--hwdec=auto-safe", "--no-terminal", "--input-ipc-server=" + m.Socket,
		}, m.Args...)
		cmd = exec.Command(m.Bin, args...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("could not start mpv: %w", err)
		}
		exited = make(chan struct{})
		go func() { cmd.Wait(); close(exited) }()
		go func() {
			select {
			case <-stop:
				cmd.Process.Kill()
			case <-exited:
			}
		}()
	}

	// Cold (box booting, CPU at 100 %) mpv takes more than 5 s to open DRM
	// and create the socket. Killing it at 5 s made the TV show the console
	// for a few seconds on every boot. Now we wait while mpv is still alive,
	// with a generous cap.
	var conn net.Conn
	var err error
	deadline := time.Now().Add(mpvSocketWait)
	for {
		if conn, err = net.Dial("unix", m.Socket); err == nil {
			break
		}
		if time.Now().After(deadline) {
			break
		}
		if exited != nil {
			select {
			case <-exited:
				return fmt.Errorf("mpv exited before opening the IPC socket: %w", err)
			default:
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		if cmd != nil {
			cmd.Process.Kill()
			<-exited
		}
		return fmt.Errorf("no IPC socket after %s: %w", mpvSocketWait, err)
	}
	m.mu.Lock()
	m.conn = conn
	last, hasLast := m.last, m.hasLast
	m.mu.Unlock()
	log.Printf("mpv connected on %s", m.Socket)
	if hasLast { // restore what was shown before mpv restarted
		go m.Show(last)
	}

	m.readLoop(conn)

	m.mu.Lock()
	m.conn = nil
	for id, ch := range m.pending {
		close(ch)
		delete(m.pending, id)
	}
	m.mu.Unlock()
	if cmd != nil {
		cmd.Process.Kill()
		<-exited
	}
	return errors.New("mpv closed; restarting")
}

func (m *MPV) readLoop(conn net.Conn) {
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var r mpvResp
		if json.Unmarshal(sc.Bytes(), &r) != nil {
			continue
		}
		if r.Event == "" {
			m.mu.Lock()
			ch := m.pending[r.RequestID]
			delete(m.pending, r.RequestID)
			m.mu.Unlock()
			if ch != nil {
				ch <- r
			}
			continue
		}
		if r.Event == "end-file" && r.Reason == "eof" {
			m.mu.Lock()
			id := m.entries[r.EntryID]
			delete(m.entries, r.EntryID)
			m.mu.Unlock()
			if id != "" && m.OnEnded != nil {
				go m.OnEnded(id)
			}
		}
	}
	conn.Close()
}

func (m *MPV) command(args ...any) (mpvResp, error) {
	m.mu.Lock()
	if m.conn == nil {
		m.mu.Unlock()
		return mpvResp{}, errors.New("mpv not connected")
	}
	m.reqID++
	id := m.reqID
	ch := make(chan mpvResp, 1)
	m.pending[id] = ch
	b, _ := json.Marshal(map[string]any{"command": args, "request_id": id})
	_, err := m.conn.Write(append(b, '\n'))
	m.mu.Unlock()
	if err != nil {
		return mpvResp{}, err
	}
	select {
	case r, ok := <-ch:
		if !ok {
			return mpvResp{}, errors.New("mpv disconnected")
		}
		if r.Error != "success" {
			return r, fmt.Errorf("mpv: %s", r.Error)
		}
		return r, nil
	case <-time.After(5 * time.Second):
		m.mu.Lock()
		delete(m.pending, id)
		m.mu.Unlock()
		return mpvResp{}, errors.New("mpv not responding")
	}
}

func (m *MPV) Show(it *store.Item) error {
	m.mu.Lock()
	m.last, m.hasLast = it, true
	connected := m.conn != nil
	m.mu.Unlock()
	if !connected {
		return nil // will be shown on connect
	}
	path, id := m.IdlePath(), ""
	if it != nil {
		path, id = m.Path(it), it.ID
	}
	r, err := m.command("loadfile", path, "replace")
	if err != nil {
		return err
	}
	var d struct {
		EntryID int64 `json:"playlist_entry_id"`
	}
	if json.Unmarshal(r.Data, &d) == nil && d.EntryID > 0 {
		m.mu.Lock()
		m.entries = map[int64]string{d.EntryID: id} // only the current one matters
		m.mu.Unlock()
	}
	m.mu.Lock()
	m.gen++
	gen := m.gen
	m.mu.Unlock()
	if it == nil || m.Corner == nil {
		m.command("overlay-remove", overlayID) // the QR screen already has it, large
		return nil
	}
	go m.placeCorner(gen)
	return nil
}

const overlayID = 1

// placeCorner puts the small QR at the bottom right. It waits until mpv knows
// the screen size (osd-width) and scales it when not 1080p.
func (m *MPV) placeCorner(gen int) {
	path, w, h, ok := m.Corner()
	if !ok {
		m.command("overlay-remove", overlayID)
		return
	}
	var ow, oh int
	for i := 0; i < 30; i++ {
		m.mu.Lock()
		stale := m.gen != gen
		m.mu.Unlock()
		if stale {
			return
		}
		ow, oh = m.intProp("osd-width"), m.intProp("osd-height")
		if ow > 0 && oh > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if ow <= 0 || oh <= 0 {
		return
	}
	k := float64(oh) / 1080
	dw, dh := int(float64(w)*k), int(float64(h)*k)
	margin := int(40 * k)
	x, y := ow-dw-margin, oh-dh-margin
	if dw != w || dh != h {
		// Scaling (dw, dh) only exists in mpv >= 0.38; if it fails, no scaling.
		if _, err := m.command("overlay-add", overlayID, x, y, path, 0, "bgra", w, h, w*4, dw, dh); err == nil {
			return
		}
		x, y = ow-w-margin, oh-h-margin
	}
	if _, err := m.command("overlay-add", overlayID, x, y, path, 0, "bgra", w, h, w*4); err != nil {
		log.Printf("mpv overlay: %v", err)
	}
}

func (m *MPV) intProp(name string) int {
	r, err := m.command("get_property", name)
	if err != nil {
		return 0
	}
	var v float64
	if json.Unmarshal(r.Data, &v) != nil {
		return 0
	}
	return int(v)
}

// panelOverlayID is above the corner QR (mpv draws overlays in ID order).
const panelOverlayID = 63

// ShowPanel draws the admin panel full screen on top of whatever mpv shows.
// The image is written as raw BGRA to PanelDir (tmpfs: /run/tvcast), to two
// alternating files so mpv never reads one that is being rewritten.
func (m *MPV) ShowPanel(img *image.RGBA) error {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	raw := make([]byte, w*h*4)
	for i := 0; i < w*h; i++ {
		raw[i*4+0] = img.Pix[i*4+2]
		raw[i*4+1] = img.Pix[i*4+1]
		raw[i*4+2] = img.Pix[i*4+0]
		raw[i*4+3] = 0xff
	}
	m.mu.Lock()
	m.panelN ^= 1
	path := filepath.Join(m.PanelDir, fmt.Sprintf("tvcast-panel-%d.bgra", m.panelN))
	m.mu.Unlock()
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return err
	}
	ow, oh := m.intProp("osd-width"), m.intProp("osd-height")
	if ow > 0 && oh > 0 && (ow != w || oh != h) {
		if _, err := m.command("overlay-add", panelOverlayID, 0, 0, path, 0, "bgra", w, h, w*4, ow, oh); err == nil {
			return nil
		}
	}
	_, err := m.command("overlay-add", panelOverlayID, 0, 0, path, 0, "bgra", w, h, w*4)
	return err
}

// HidePanel removes the admin panel.
func (m *MPV) HidePanel() error {
	_, err := m.command("overlay-remove", panelOverlayID)
	for i := 0; i < 2; i++ {
		os.Remove(filepath.Join(m.PanelDir, fmt.Sprintf("tvcast-panel-%d.bgra", i)))
	}
	return err
}
