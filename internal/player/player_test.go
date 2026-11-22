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

// pretend the interval has passed and process one tick
func expire(p *Player) {
	p.mu.Lock()
	p.shownAt = time.Time{}
	p.mu.Unlock()
	p.tick()
}

func TestQRInterleaved(t *testing.T) {
	p, d, ids := setup(t, 3, 2)
	p.Show(ids[0])
	expire(p) // 1st automatic -> ids[1]
	if d.get() != ids[1] {
		t.Fatalf("want %s, got %q", ids[1], d.get())
	}
	expire(p) // 2nd automatic -> QR is due
	if d.get() != "" || !p.State().QRSlide {
		t.Fatalf("want inserted QR, got %q", d.get())
	}
	expire(p) // QR done -> continue where it was
	if d.get() != ids[2] {
		t.Fatalf("after the QR want %s, got %q", ids[2], d.get())
	}
}

func TestHoldQRIgnoresPhotos(t *testing.T) {
	p, d, ids := setup(t, 3, 0)
	p.Show(ids[0])
	p.HoldQR(true)
	if d.get() != "" || !p.Held() {
		t.Fatal("HoldQR did not show the QR screen")
	}
	p.Show(ids[2]) // a photo arrives: kept for later but not shown
	p.Next()
	expire(p)
	if d.get() != "" {
		t.Fatalf("with the QR pinned, %q was shown", d.get())
	}
	p.HoldQR(false)
	if d.get() == "" || p.Held() {
		t.Fatal("releasing did not return to the slideshow")
	}
}

func TestEmptyShowsQR(t *testing.T) {
	p, d, _ := setup(t, 0, 2)
	p.Refresh()
	expire(p)
	if d.get() != "" {
		t.Fatalf("with no photos the QR should show, got %q", d.get())
	}
}
