// Package store guarda en disco las fotos y vídeos subidos.
//
// Reglas de seguridad:
//   - El nombre en disco lo genera el servidor (ID aleatorio + extensión
//     derivada del tipo detectado). Nunca se usa el nombre que manda el móvil.
//   - El tipo se detecta por los bytes del fichero (magic bytes), no por la
//     extensión ni por el Content-Type del cliente.
//   - Hay límite por fichero y cuota total; al pasarse se borran los más
//     antiguos.
package store

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type Kind string

const (
	KindImage Kind = "image"
	KindVideo Kind = "video"
)

type Item struct {
	ID       string    `json:"id"`
	Kind     Kind      `json:"kind"`
	Mime     string    `json:"mime"`
	Ext      string    `json:"ext"`
	Size     int64     `json:"size"`
	Name     string    `json:"name"` // nombre original, solo para mostrar
	HasThumb bool      `json:"hasThumb"`
	Created  time.Time `json:"created"`
}

var (
	ErrTooLarge    = errors.New("fichero demasiado grande")
	ErrUnsupported = errors.New("formato no soportado")
	ErrHEIC        = errors.New("HEIC no soportado: envíala desde el navegador del móvil (la convierte a JPEG) o cambia la cámara a 'Más compatible'")
	ErrNotFound    = errors.New("no existe")
)

// allowed: tipo detectado -> (clase, extensión)
var allowed = map[string]struct {
	kind Kind
	ext  string
}{
	"image/jpeg":      {KindImage, ".jpg"},
	"image/png":       {KindImage, ".png"},
	"image/gif":       {KindImage, ".gif"},
	"image/webp":      {KindImage, ".webp"},
	"video/mp4":       {KindVideo, ".mp4"},
	"video/quicktime": {KindVideo, ".mov"},
	"video/webm":      {KindVideo, ".webm"},
}

type Store struct {
	dir      string
	maxFile  int64
	maxTotal int64
	maxItems int

	mu    sync.Mutex
	items []*Item // orden de llegada (más antiguo primero)
}

func Open(dir string, maxFile, maxTotal int64, maxItems int) (*Store, error) {
	s := &Store{dir: dir, maxFile: maxFile, maxTotal: maxTotal, maxItems: maxItems}
	if err := os.MkdirAll(s.mediaDir(), 0o750); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(s.indexPath())
	switch {
	case err == nil:
		if err := json.Unmarshal(b, &s.items); err != nil {
			return nil, fmt.Errorf("index.json corrupto: %w", err)
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		return nil, err
	}
	// Quita entradas cuyo fichero ya no existe.
	kept := s.items[:0]
	for _, it := range s.items {
		if _, err := os.Stat(s.Path(it)); err == nil {
			kept = append(kept, it)
		}
	}
	s.items = kept
	return s, nil
}

func (s *Store) mediaDir() string  { return filepath.Join(s.dir, "media") }
func (s *Store) indexPath() string { return filepath.Join(s.dir, "index.json") }

func (s *Store) Path(it *Item) string      { return filepath.Join(s.mediaDir(), it.ID+it.Ext) }
func (s *Store) ThumbPath(it *Item) string { return filepath.Join(s.mediaDir(), it.ID+".thumb.jpg") }

func (s *Store) List() []Item {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Item, len(s.items))
	for i, it := range s.items {
		out[i] = *it
	}
	return out
}

func (s *Store) Get(id string) (Item, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, it := range s.items {
		if it.ID == id {
			return *it, true
		}
	}
	return Item{}, false
}

// Save lee r (como mucho maxFile bytes), valida el tipo y lo guarda.
// thumb es opcional: miniatura JPEG generada en el móvil.
func (s *Store) Save(name string, r io.Reader, thumb []byte) (Item, error) {
	id, err := newID()
	if err != nil {
		return Item{}, err
	}
	tmp, err := os.CreateTemp(s.mediaDir(), ".upload-*")
	if err != nil {
		return Item{}, err
	}
	defer os.Remove(tmp.Name()) // no-op si ya se renombró

	// Leemos la cabecera para detectar el tipo antes de escribir el resto.
	head := make([]byte, 512)
	n, err := io.ReadFull(r, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		tmp.Close()
		if errors.Is(err, io.EOF) {
			return Item{}, ErrUnsupported
		}
		return Item{}, err
	}
	head = head[:n]
	mime, err := Sniff(head)
	if err != nil {
		tmp.Close()
		return Item{}, err
	}
	a := allowed[mime]

	written, err := io.Copy(tmp, io.LimitReader(io.MultiReader(bytes.NewReader(head), r), s.maxFile+1))
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return Item{}, err
	}
	if written > s.maxFile {
		return Item{}, ErrTooLarge
	}

	it := &Item{
		ID: id, Kind: a.kind, Mime: mime, Ext: a.ext, Size: written,
		Name: cleanName(name), Created: time.Now().UTC(),
	}
	if err := os.Rename(tmp.Name(), s.Path(it)); err != nil {
		return Item{}, err
	}
	if len(thumb) > 0 && len(thumb) < 512<<10 {
		if m, _ := Sniff(thumb); m == "image/jpeg" {
			if os.WriteFile(s.ThumbPath(it), thumb, 0o640) == nil {
				it.HasThumb = true
			}
		}
	}

	s.mu.Lock()
	s.items = append(s.items, it)
	evicted := s.enforceQuotaLocked()
	err = s.persistLocked()
	s.mu.Unlock()
	for _, e := range evicted {
		s.removeFiles(e)
	}
	return *it, err
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	idx := -1
	for i, it := range s.items {
		if it.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		s.mu.Unlock()
		return ErrNotFound
	}
	it := s.items[idx]
	s.items = append(s.items[:idx], s.items[idx+1:]...)
	err := s.persistLocked()
	s.mu.Unlock()
	s.removeFiles(it)
	return err
}

func (s *Store) removeFiles(it *Item) {
	os.Remove(s.Path(it))
	os.Remove(s.ThumbPath(it))
}

// enforceQuotaLocked borra los más antiguos hasta cumplir cuota (nunca el
// último subido).
func (s *Store) enforceQuotaLocked() []*Item {
	var total int64
	for _, it := range s.items {
		total += it.Size
	}
	var evicted []*Item
	for len(s.items) > 1 && (total > s.maxTotal || (s.maxItems > 0 && len(s.items) > s.maxItems)) {
		old := s.items[0]
		s.items = s.items[1:]
		total -= old.Size
		evicted = append(evicted, old)
	}
	return evicted
}

func (s *Store) persistLocked() error {
	// El orden es el de llegada (append), no por fecha: la caja no tiene
	// reloj con pila ni internet para NTP, así que la hora puede ir hacia atrás.
	b, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.indexPath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o640); err != nil {
		return err
	}
	return os.Rename(tmp, s.indexPath())
}

// Sniff detecta el tipo por magic bytes y devuelve error si no está permitido.
func Sniff(head []byte) (string, error) {
	// Contenedores ISO-BMFF (mp4/mov/heic): caja "ftyp" en el byte 4.
	if len(head) >= 12 && string(head[4:8]) == "ftyp" {
		brand := string(head[8:12])
		switch brand {
		case "heic", "heix", "hevc", "heim", "heis", "mif1", "msf1", "avif":
			return "", ErrHEIC
		case "qt  ":
			return "video/quicktime", nil
		default:
			return "video/mp4", nil // isom, mp41, mp42, avc1, M4V ...
		}
	}
	mime := http.DetectContentType(head)
	if i := strings.IndexByte(mime, ';'); i >= 0 {
		mime = mime[:i]
	}
	if _, ok := allowed[mime]; !ok {
		return "", ErrUnsupported
	}
	return mime, nil
}

func newID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// ValidID evita que un ID de la URL se use para salir del directorio.
func ValidID(id string) bool {
	if len(id) != 16 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func cleanName(n string) string {
	n = filepath.Base(strings.ReplaceAll(n, "\\", "/"))
	if !utf8.ValidString(n) {
		n = strings.ToValidUTF8(n, "")
	}
	n = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, n)
	if len([]rune(n)) > 80 {
		n = string([]rune(n)[:80])
	}
	return n
}
