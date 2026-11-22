package admin

import (
	"errors"
	"image"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/usarral/tvcast/internal/i18n"
	"github.com/usarral/tvcast/internal/remote"
)

type fakeBox struct {
	calls  []string
	paused bool
	held   bool
}

func (b *fakeBox) Next()            { b.calls = append(b.calls, "next") }
func (b *fakeBox) Prev()            { b.calls = append(b.calls, "prev") }
func (b *fakeBox) TogglePause()     { b.paused = !b.paused; b.calls = append(b.calls, "toggle") }
func (b *fakeBox) SetPaused(v bool) { b.paused = v }
func (b *fakeBox) Paused() bool     { return b.paused }
func (b *fakeBox) HoldQR(v bool) {
	b.held = v
	b.calls = append(b.calls, map[bool]string{true: "qr1", false: "qr0"}[v])
}
func (b *fakeBox) Held() bool              { return b.held }
func (b *fakeBox) DeleteAll() (int, error) { b.calls = append(b.calls, "deleteall"); return 3, nil }

type fakeSys struct {
	ran []string
	err error
}

func (s *fakeSys) Run(v string) error { s.ran = append(s.ran, v); return s.err }
func (s *fakeSys) Log(w string, n int) ([]string, error) {
	return []string{"line for " + w}, nil
}

type fakeScreen struct {
	mu            sync.Mutex
	shown, hidden int
}

func (f *fakeScreen) ShowPanel(*image.RGBA) error { f.mu.Lock(); f.shown++; f.mu.Unlock(); return nil }
func (f *fakeScreen) HidePanel() error            { f.mu.Lock(); f.hidden++; f.mu.Unlock(); return nil }

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func setup() (*Controller, *fakeBox, *fakeSys, *fakeScreen, *clock) {
	b, s, sc := &fakeBox{}, &fakeSys{}, &fakeScreen{}
	ck := &clock{t: time.Unix(1000, 0)}
	c := &Controller{Box: b, Sys: s, Screen: sc, Lang: i18n.EN, Now: ck.now, IdleClose: time.Hour}
	return c, b, s, sc, ck
}

func keys(c *Controller, ks ...Key) {
	for _, k := range ks {
		c.Key(k)
	}
}

func TestEverydayKeysWhenClosed(t *testing.T) {
	c, b, _, _, _ := setup()
	keys(c, KRight, KLeft, KOK, KPlayPause, KHome, KBack, KHeart, KUp, KVolUp, KPower)
	want := "next prev toggle toggle qr1 qr0"
	if got := strings.Join(b.calls, " "); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if c.Rev() != 0 {
		t.Fatal("panel opened by everyday keys")
	}
}

func TestSecretSequence(t *testing.T) {
	c, b, _, sc, ck := setup()
	keys(c, KHeart, KUp, KHeart) // incomplete
	if c.Rev() != 0 {
		t.Fatal("opened too early")
	}
	ck.t = ck.t.Add(6 * time.Second) // too slow
	c.Key(KDown)
	if c.Rev() != 0 {
		t.Fatal("opened after the window")
	}
	keys(c, KRight, KHeart, KUp, KHeart, KDown) // noise before is fine
	if c.Rev() == 0 || sc.shown == 0 {
		t.Fatal("sequence did not open the panel")
	}
	if !b.paused {
		t.Fatal("slideshow must pause behind the panel")
	}
}

func TestKeyboardShortcutAndCloseRestoresPause(t *testing.T) {
	c, b, _, sc, _ := setup()
	c.Key(KAdmin)
	if c.Rev() == 0 {
		t.Fatal("Ctrl+Alt+A should open")
	}
	c.Key(KDown) // "pause" item: flip what happens on close
	c.Key(KOK)
	if !strings.Contains(c.view().Message, "paused") {
		t.Fatalf("message %q", c.view().Message)
	}
	c.Key(KHome)
	if c.Rev() != 0 || sc.hidden != 1 {
		t.Fatal("Home should close")
	}
	if !b.paused {
		t.Fatal("asked to stay paused")
	}
	// Everyday keys work again; navigation keys don't leak while open.
	if strings.Contains(strings.Join(b.calls, " "), "next") {
		t.Fatal("Down/OK inside the panel reached the slideshow")
	}
}

func find(items []Item, label string) int {
	for i, it := range items {
		if it.Label == label {
			return i
		}
	}
	return -1
}

func goTo(c *Controller, label string) {
	for i := 0; i < 20 && c.view().Items[c.view().Cursor] != label; i++ {
		c.Key(KDown)
	}
}

func TestConfirmDefaultsToNo(t *testing.T) {
	c, _, s, sc, _ := setup()
	c.Open()
	goTo(c, "adm.system")
	c.Key(KOK)
	goTo(c, "adm.reboot")
	c.Key(KOK)
	if c.view().Confirm != "adm.reboot" || c.view().Yes {
		t.Fatalf("want confirmation on No: %+v", c.view())
	}
	c.Key(KOK) // OK on "No"
	if len(s.ran) != 0 {
		t.Fatal("ran without confirming")
	}
	c.Key(KOK)    // ask again
	c.Key(KRight) // move to Yes
	c.Key(KOK)
	if strings.Join(s.ran, ",") != "reboot" {
		t.Fatalf("ran %v", s.ran)
	}
	if c.Rev() != 0 || sc.hidden == 0 {
		t.Fatal("reboot should close the panel")
	}
}

func TestSystemErrorShown(t *testing.T) {
	c, _, s, _, _ := setup()
	s.err = errors.New("polkit said no")
	c.Open()
	goTo(c, "adm.network")
	keys(c, KOK, KOK, KRight, KOK) // Use the home wifi -> Yes
	if s.ran[0] != "wifi-home" || !strings.Contains(c.view().Message, "polkit said no") {
		t.Fatalf("ran %v, message %q", s.ran, c.view().Message)
	}
}

func TestLogsAndBack(t *testing.T) {
	c, _, _, _, _ := setup()
	c.Open()
	goTo(c, "adm.logs")
	c.Key(KOK)
	goTo(c, "adm.log_remote")
	c.Key(KOK)
	v := c.view()
	if v.Log == nil || v.Log[0] != "line for remote" || v.LogTitle != "adm.log_remote" {
		t.Fatalf("log view %+v", v)
	}
	c.Key(KBack) // leave the log
	c.Key(KBack) // leave the submenu
	if c.view().Title != "" || c.view().Log != nil {
		t.Fatalf("not back at main: %+v", c.view())
	}
	c.Key(KBack) // Back on the main menu closes
	if c.Rev() != 0 {
		t.Fatal("Back on the main menu should close")
	}
}

func TestDeleteAllPhotos(t *testing.T) {
	c, b, _, _, _ := setup()
	c.Open()
	goTo(c, "adm.photos")
	keys(c, KOK, KOK, KDown, KOK) // Down also toggles No/Yes
	if !strings.Contains(strings.Join(b.calls, " "), "deleteall") || !strings.Contains(c.view().Message, "3 photos") {
		t.Fatalf("calls %v message %q", b.calls, c.view().Message)
	}
}

func TestIdleClose(t *testing.T) {
	c, _, _, sc, _ := setup()
	c.IdleClose = 20 * time.Millisecond
	c.Open()
	deadline := time.Now().Add(2 * time.Second)
	for c.Rev() != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if c.Rev() != 0 || sc.hidden != 1 {
		t.Fatal("panel did not close by itself")
	}
}

func TestMenuActionsAreKnown(t *testing.T) {
	var walk func([]Item)
	walk = func(items []Item) {
		for _, it := range items {
			if it.Sub != nil {
				walk(it.Sub)
				continue
			}
			switch {
			case strings.HasPrefix(it.Action, "sys:"):
				if !validVerb(strings.TrimPrefix(it.Action, "sys:")) {
					t.Errorf("%s: verb not in Verbs", it.Action)
				}
				if !it.Confirm {
					t.Errorf("%s: system actions must ask for confirmation", it.Action)
				}
			case strings.HasPrefix(it.Action, "log:"):
				if _, ok := logUnits[strings.TrimPrefix(it.Action, "log:")]; !ok {
					t.Errorf("%s: unknown log", it.Action)
				}
			case it.Action == "qr" || it.Action == "pause" || it.Action == "photos-delete" || it.Action == "close":
			default:
				t.Errorf("unknown action %q", it.Action)
			}
			for _, l := range i18n.Supported {
				if i18n.T(l, it.Label) == it.Label {
					t.Errorf("%s: no text for %s", l, it.Label)
				}
			}
		}
	}
	walk(Menu())
	if find(Menu(), "adm.close") < 0 {
		t.Error("no Close item")
	}
}

func TestParseKey(t *testing.T) {
	for in, want := range map[string]Key{"enter": KOK, "esc": KBack, "heart": KHeart, "admin": KAdmin} {
		if k, ok := ParseKey(in); !ok || k != want {
			t.Errorf("%s: %v %v", in, k, ok)
		}
	}
	if _, ok := ParseKey("mic"); ok {
		t.Error("mic must not be accepted (the remote sends noise on it)")
	}
}

func TestNetworkMode(t *testing.T) {
	ap := "Interface wlan0\n\tifindex 3\n\tssid Tele\n\ttype AP\n\tchannel 6\n"
	if m, s := NetworkMode(ap); m != "AP" || s != "Tele" {
		t.Fatalf("%q %q", m, s)
	}
	home := "Interface wlan0\n\tssid My Home 5G\n\ttype managed\n"
	if m, s := NetworkMode(home); m != "managed" || s != "My Home 5G" {
		t.Fatalf("%q %q", m, s)
	}
}

func TestStatusLines(t *testing.T) {
	now := time.Unix(10000, 0)
	in := Info{Version: "v1", IP: "10.42.0.1", Network: "own wifi", Photos: 12, FreeBytes: 3 << 30,
		Uptime: 125 * time.Minute, Now: now, Remote: Remote{Connected: true, Battery: 80}}
	got := strings.Join(StatusLines(in, i18n.EN), "\n")
	for _, w := range []string{"10.42.0.1", "12 photos", "3.0 GB", "2 h 05 min", "battery 80 %", "tvcast v1"} {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in\n%s", w, got)
		}
	}
	in.Remote = Remote{Seen: now.Add(-7 * time.Minute), Battery: -1}
	if got := StatusLines(in, i18n.ES)[3]; !strings.Contains(got, "hace 7 min") {
		t.Errorf("es remote line %q", got)
	}
}

func TestRenderAllScreens(t *testing.T) {
	st := newState()
	for _, v := range []View{st.View(),
		{Title: "adm.system", Items: []string{"adm.reboot"}, Confirm: "adm.reboot", Yes: true},
		{LogTitle: "adm.log_kernel", Log: []string{strings.Repeat("very long line ", 40)}},
		{LogTitle: "adm.log_kernel", Log: []string{}},
	} {
		for _, l := range i18n.Supported {
			img, err := Render(v, []string{"status"}, l)
			if err != nil || img.Bounds().Dx() != W || len(BGRA(img)) != W*H*4 {
				t.Fatalf("render %v: %v", l, err)
			}
		}
	}
}

// Every key tvcast-remote sends must be one tvcast understands.
func TestRemoteKeysAreAccepted(t *testing.T) {
	for b, paths := range remote.DefaultKeymap {
		for _, p := range paths {
			k := strings.TrimPrefix(p, "/api/tv/key?k=")
			if k == p {
				t.Errorf("%v: %s is not /api/tv/key", b, p)
			}
			if _, ok := ParseKey(k); !ok {
				t.Errorf("%v: key %q not accepted by tvcast", b, k)
			}
		}
	}
}
