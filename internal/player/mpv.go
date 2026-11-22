package player

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/usarral/tvcast/internal/store"
)

// MPV muestra fotos y vídeos a pantalla completa con mpv, controlado por su
// socket JSON IPC. Un solo proceso para todo: no hay que cambiar de programa
// entre foto y vídeo, y mpv usa decodificación por hardware si existe.
type MPV struct {
	Bin      string   // "" = no lanzar mpv, solo conectarse a Socket
	Args     []string // argumentos extra (p. ej. --vo=drm)
	Socket   string
	Path     func(*store.Item) string
	IdlePath func() string
	OnEnded  func(id string)
	// Corner devuelve el QR pequeño en BGRA para la esquina (ok=false: sin QR).
	Corner func() (path string, w, h int, ok bool)

	mu      sync.Mutex
	conn    net.Conn
	reqID   int
	pending map[int]chan mpvResp
	entries map[int64]string // playlist_entry_id -> item ID
	last    *store.Item
	hasLast bool
	gen     int // invalida colocaciones de overlay pendientes
}

type mpvResp struct {
	RequestID int             `json:"request_id"`
	Error     string          `json:"error"`
	Data      json.RawMessage `json:"data"`
	Event     string          `json:"event"`
	Reason    string          `json:"reason"`
	EntryID   int64           `json:"playlist_entry_id"`
}

// mpvSocketWait es lo máximo que se espera a que mpv cree su socket IPC.
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
	var exited chan struct{} // se cierra cuando el proceso mpv termina
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
			return fmt.Errorf("no se pudo lanzar mpv: %w", err)
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

	// En frío (arranque de la caja, CPU al 100 %) mpv tarda más de 5 s en
	// abrir DRM y crear el socket. Si lo matábamos a los 5 s, la tele
	// enseñaba la consola unos segundos en cada arranque. Ahora se espera
	// mientras mpv siga vivo, con un tope generoso.
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
				return fmt.Errorf("mpv terminó antes de abrir el socket IPC: %w", err)
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
		return fmt.Errorf("sin socket IPC tras %s: %w", mpvSocketWait, err)
	}
	m.mu.Lock()
	m.conn = conn
	last, hasLast := m.last, m.hasLast
	m.mu.Unlock()
	log.Printf("mpv conectado en %s", m.Socket)
	if hasLast { // reponer lo que había antes de reiniciar mpv
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
	return errors.New("mpv se cerró; reiniciando")
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
		return mpvResp{}, errors.New("mpv no conectado")
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
			return mpvResp{}, errors.New("mpv desconectado")
		}
		if r.Error != "success" {
			return r, fmt.Errorf("mpv: %s", r.Error)
		}
		return r, nil
	case <-time.After(5 * time.Second):
		m.mu.Lock()
		delete(m.pending, id)
		m.mu.Unlock()
		return mpvResp{}, errors.New("mpv no responde")
	}
}

func (m *MPV) Show(it *store.Item) error {
	m.mu.Lock()
	m.last, m.hasLast = it, true
	connected := m.conn != nil
	m.mu.Unlock()
	if !connected {
		return nil // se mostrará al conectar
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
		m.entries = map[int64]string{d.EntryID: id} // solo importa el actual
		m.mu.Unlock()
	}
	m.mu.Lock()
	m.gen++
	gen := m.gen
	m.mu.Unlock()
	if it == nil || m.Corner == nil {
		m.command("overlay-remove", overlayID) // la pantalla de QR ya lo lleva grande
		return nil
	}
	go m.placeCorner(gen)
	return nil
}

const overlayID = 1

// placeCorner pone el QR pequeño abajo a la derecha. Espera a que mpv sepa
// el tamaño de pantalla (osd-width) y lo escala si no es 1080p.
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
		// Escalar (dw, dh) solo existe en mpv >= 0.38; si falla, sin escalar.
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
