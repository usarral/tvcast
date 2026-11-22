package player

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"github.com/usarral/tvcast/internal/store"
)

type fakeDisplay struct {
	mu   sync.Mutex
	last string // "" = QR
}

func (f *fakeDisplay) Show(it *store.Item) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.last = ""
	if it != nil {
		f.last = it.ID
	}
	return nil
}

func (f *fakeDisplay) get() string { f.mu.Lock(); defer f.mu.Unlock(); return f.last }

func setup(t *testing.T, n, qrEvery int) (*Player, *fakeDisplay, []string) {
	t.Helper()
	st, err := store.Open(t.TempDir(), 1<<20, 1<<30, 0)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for i := 0; i < n; i++ {
		it, err := st.Save("x.jpg", bytes.NewReader([]byte{0xff, 0xd8, 0xff, 0xe0, 0, 0x10, 'J', 'F', 'I', 'F', 0}), nil)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, it.ID)
	}
	p := New(st, time.Hour, qrEvery)
	d := &fakeDisplay{}
	p.AddDisplay(d)
	return p, d, ids
}

// forzar que "ha pasado el intervalo" y procesar un tick
func expire(p *Player) {
	p.mu.Lock()
	p.shownAt = time.Time{}
	p.mu.Unlock()
	p.tick()
}

func TestQRInterleaved(t *testing.T) {
	p, d, ids := setup(t, 3, 2)
	p.Show(ids[0])
	expire(p) // 1ª automática -> ids[1]
	if d.get() != ids[1] {
		t.Fatalf("esperaba %s, hay %q", ids[1], d.get())
	}
	expire(p) // 2ª automática -> toca QR
	if d.get() != "" || !p.State().QRSlide {
		t.Fatalf("esperaba QR intercalado, hay %q", d.get())
	}
	expire(p) // acaba el QR -> sigue donde iba
	if d.get() != ids[2] {
		t.Fatalf("tras el QR esperaba %s, hay %q", ids[2], d.get())
	}
}

func TestHoldQRIgnoresPhotos(t *testing.T) {
	p, d, ids := setup(t, 3, 0)
	p.Show(ids[0])
	p.HoldQR(true)
	if d.get() != "" || !p.Held() {
		t.Fatal("HoldQR no puso la pantalla de QR")
	}
	p.Show(ids[2]) // llega una foto: se guarda para luego pero no se ve
	p.Next()
	expire(p)
	if d.get() != "" {
		t.Fatalf("con QR fijo se vio %q", d.get())
	}
	p.HoldQR(false)
	if d.get() == "" || p.Held() {
		t.Fatal("al soltar no volvió al pase")
	}
}

func TestEmptyShowsQR(t *testing.T) {
	p, d, _ := setup(t, 0, 2)
	p.Refresh()
	expire(p)
	if d.get() != "" {
		t.Fatalf("sin fotos debería verse el QR, hay %q", d.get())
	}
}
