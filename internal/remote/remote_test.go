package remote

import (
	"bytes"
	"context"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

func kb(code byte) Notification   { return Notification{Report, []byte{0, 0, code, 0, 0, 0, 0, 0}} }
func cons(code byte) Notification { return Notification{Report, []byte{code}} }

// The table in docs/LESSONS.md, byte for byte.
func TestDecodeTable(t *testing.T) {
	cases := []struct {
		n    Notification
		want Button
	}{
		{kb(0x52), Up}, {kb(0x51), Down}, {kb(0x50), Left}, {kb(0x4f), Right}, {kb(0x28), OK},
		{cons(0x0b), Back}, {cons(0x05), Home}, {cons(0x02), PlayPause}, {cons(0x3c), Heart},
		{cons(0x04), Power}, {cons(0x09), VolUp}, {cons(0x0a), VolDown},
		{Notification{Voice, []byte{0x04}}, Mic},
	}
	for _, c := range cases {
		if ev := Decode(c.n); ev.Button != c.want || ev.Release || ev.Unknown {
			t.Errorf("%v: got %+v, want %v", c.n, ev, c.want)
		}
	}
}

func TestDecodeReleasesAndJunk(t *testing.T) {
	for _, n := range []Notification{kb(0), cons(0), {Voice, []byte{0}}} {
		if ev := Decode(n); !ev.Release {
			t.Errorf("%v: want release, got %+v", n, ev)
		}
	}
	for _, n := range []Notification{
		kb(0x99), cons(0x77), {Report, nil}, {Report, []byte{1, 2, 3}},
		{Report, []byte{0x02, 0, 0x28, 0, 0, 0, 0, 0}}, // modifier
		{Report, []byte{0, 0, 0x28, 0x4f, 0, 0, 0, 0}}, // two keys
		{Voice, []byte{0x04, 0x00}}, {Voice, []byte{0x07}},
	} {
		if ev := Decode(n); !ev.Unknown {
			t.Errorf("%v: want unknown, got %+v", n, ev)
		}
	}
}

type fakeAPI struct {
	mu    sync.Mutex
	calls []string
	err   error
}

func (f *fakeAPI) Post(_ context.Context, p string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, p)
	return f.err
}

func quiet() (*log.Logger, *bytes.Buffer) {
	var b bytes.Buffer
	return log.New(&b, "", 0), &b
}

// A realistic stream: press+release per key, a Mic press with its release.
func TestDispatcherStream(t *testing.T) {
	api := &fakeAPI{}
	lg, out := quiet()
	d := &Dispatcher{API: api, Keymap: DefaultKeymap, Log: lg}
	for _, n := range []Notification{
		kb(0x4f), kb(0), // right
		kb(0x50), kb(0), // left
		kb(0x28), kb(0), // ok
		cons(0x05), cons(0), // home
		cons(0x0b), cons(0), // back
		cons(0x02), cons(0), // play/pause
		{Voice, []byte{4}}, {Voice, []byte{0}}, // mic: logged only
		cons(0x04), cons(0), // power: logged only
		cons(0x42), // unknown
	} {
		d.Handle(context.Background(), n)
	}
	want := []string{"/api/next", "/api/prev", "/api/pause?v=toggle", "/api/tv/qr?v=toggle",
		"/api/tv/qr?v=0", "/api/pause?v=0", "/api/pause?v=toggle"}
	if strings.Join(api.calls, " ") != strings.Join(want, " ") {
		t.Fatalf("calls\n got %v\nwant %v", api.calls, want)
	}
	for _, s := range []string{"mic (not mapped)", "power (not mapped)", "unknown notification: report 42"} {
		if !strings.Contains(out.String(), s) {
			t.Errorf("log lacks %q:\n%s", s, out)
		}
	}
}

func TestDispatcherStopsChainOnError(t *testing.T) {
	api := &fakeAPI{err: errors.New("tvcast down")}
	lg, out := quiet()
	(&Dispatcher{API: api, Keymap: DefaultKeymap, Log: lg}).Handle(context.Background(), cons(0x0b))
	if len(api.calls) != 1 || !strings.Contains(out.String(), "tvcast down") {
		t.Fatalf("calls %v, log %q", api.calls, out)
	}
}

func TestAPIPost(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.RequestURI())
		if r.URL.Path == "/bad" {
			w.WriteHeader(403)
			return
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()
	a := &API{Base: srv.URL + "/"}
	if err := a.Post(context.Background(), "/api/pause?v=toggle"); err != nil {
		t.Fatal(err)
	}
	if err := a.Post(context.Background(), "/bad"); err == nil {
		t.Fatal("403 should be an error")
	}
	if got[0] != "POST /api/pause?v=toggle" {
		t.Fatalf("got %v", got)
	}
}

// fakeLink plays scripted sessions: each emits some notifications and then
// drops with an error, like the remote going out of range.
type fakeLink struct {
	sessions [][]Notification
	n        int
}

func (f *fakeLink) Listen(ctx context.Context, emit func(Notification)) error {
	if f.n >= len(f.sessions) {
		<-ctx.Done()
		return ctx.Err()
	}
	s := f.sessions[f.n]
	f.n++
	if s == nil {
		return errors.New("connect: page timeout")
	}
	for _, x := range s {
		emit(x)
	}
	return errors.New("remote disconnected")
}

func TestRunReconnects(t *testing.T) {
	link := &fakeLink{sessions: [][]Notification{
		nil, nil, nil, // remote asleep: three failed connects
		{kb(0x4f), kb(0)},
		{cons(0x05), cons(0)},
	}}
	api := &fakeAPI{}
	lg, out := quiet()
	d := &Dispatcher{API: api, Keymap: DefaultKeymap, Log: lg}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error)
	go func() { done <- Run(ctx, link, func(n Notification) { d.Handle(ctx, n) }, time.Millisecond, lg) }()
	for {
		api.mu.Lock()
		n := len(api.calls)
		api.mu.Unlock()
		if n == 2 {
			break
		}
		if ctx.Err() != nil {
			t.Fatalf("timed out; calls %v", api.calls)
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	if c := strings.Count(out.String(), "page timeout"); c != 1 {
		t.Errorf("repeated error logged %d times, want 1:\n%s", c, out)
	}
}

// --- BlueZ object parsing, with objects shaped like GetManagedObjects -----

func v(x any) dbus.Variant { return dbus.MakeVariant(x) }

func fixture() objects {
	dev := func(name, addr string, paired bool) map[string]map[string]dbus.Variant {
		return map[string]map[string]dbus.Variant{ifDevice: {
			"Name": v(name), "Alias": v(name), "Address": v(addr), "Paired": v(paired), "Connected": v(false)}}
	}
	char := func(uuid string, flags ...string) map[string]map[string]dbus.Variant {
		return map[string]map[string]dbus.Variant{ifChar: {"UUID": v(uuid), "Flags": v(flags)}}
	}
	d := "/org/bluez/hci1/dev_AA_AA_AA_AA_AA_01"
	return objects{
		"/org/bluez/hci0/dev_AA_AA_AA_AA_AA_02":                      dev("Sunday", "AA:AA:AA:AA:AA:02", false), // not paired
		"/org/bluez/hci0/dev_AA_AA_AA_AA_AA_03":                      dev("Speaker", "AA:AA:AA:AA:AA:03", true),
		dbus.ObjectPath(d):                                           dev("Sunday", "AA:AA:AA:AA:AA:01", true),
		dbus.ObjectPath(d + "/service0010/char0011"):                 char(uuidReport, "read", "notify"),
		dbus.ObjectPath(d + "/service0010/char0015"):                 char(uuidReport, "read", "notify"),
		dbus.ObjectPath(d + "/service0010/char0019"):                 char(uuidReport, "read", "write"), // output report
		dbus.ObjectPath(d + "/service0030/char0031"):                 char(uuidVoice, "notify"),
		dbus.ObjectPath(d + "/service0030/char0035"):                 char("f000b002-0451-4000-b000-000000000000", "notify"),
		dbus.ObjectPath(d + "/service0040/char0041"):                 char("00002a19-0000-1000-8000-00805f9b34fb", "read", "notify"),
		"/org/bluez/hci0/dev_AA_AA_AA_AA_AA_03/service0010/char0011": char(uuidReport, "notify"),
	}
}

func TestFindDevice(t *testing.T) {
	objs := fixture()
	p, err := findDevice(objs, "", DefaultNames)
	if err != nil || p != "/org/bluez/hci1/dev_AA_AA_AA_AA_AA_01" {
		t.Fatalf("by name: %v %v", p, err)
	}
	if p, err = findDevice(objs, "aa:aa:aa:aa:aa:03", nil); err != nil || p != "/org/bluez/hci0/dev_AA_AA_AA_AA_AA_03" {
		t.Fatalf("by address: %v %v", p, err)
	}
	if _, err = findDevice(objs, "AA:AA:AA:AA:AA:02", nil); err == nil {
		t.Fatal("an unpaired device must not be used")
	}
}

func TestFindChars(t *testing.T) {
	d := dbus.ObjectPath("/org/bluez/hci1/dev_AA_AA_AA_AA_AA_01")
	got := findChars(fixture(), d)
	want := map[dbus.ObjectPath]Source{
		d + "/service0010/char0011": Report,
		d + "/service0010/char0015": Report,
		d + "/service0030/char0031": Voice,
	}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for k, s := range want {
		if got[k] != s {
			t.Errorf("%s: got %v want %v", k, got[k], s)
		}
	}
}

func TestParseSignal(t *testing.T) {
	d := dbus.ObjectPath("/org/bluez/hci1/dev_AA_AA_AA_AA_AA_01")
	c := d + "/service0010/char0011"
	chars := map[dbus.ObjectPath]Source{c: Report}
	sig := func(path dbus.ObjectPath, iface string, ch map[string]dbus.Variant) *dbus.Signal {
		return &dbus.Signal{Path: path, Name: ifProps + ".PropertiesChanged", Body: []any{iface, ch, []string{}}}
	}
	n, drop, ok := parseSignal(sig(c, ifChar, map[string]dbus.Variant{"Value": v([]byte{0, 0, 0x4f, 0, 0, 0, 0, 0})}), d, chars)
	if !ok || drop || Decode(n).Button != Right {
		t.Fatalf("value: %v %v %v", n, drop, ok)
	}
	if _, drop, _ = parseSignal(sig(d, ifDevice, map[string]dbus.Variant{"Connected": v(false)}), d, chars); !drop {
		t.Fatal("Connected=false must end the session")
	}
	if _, drop, ok = parseSignal(sig(d, ifDevice, map[string]dbus.Variant{"RSSI": v(int16(-60))}), d, chars); drop || ok {
		t.Fatal("other device properties are ignored")
	}
	if _, _, ok = parseSignal(sig(d+"/service0030/char0035", ifChar, map[string]dbus.Variant{"Value": v([]byte{1})}), d, chars); ok {
		t.Fatal("audio characteristic must be ignored")
	}
}
