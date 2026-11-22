// tvcast sends photos and videos from a phone to the TV.
package main

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/usarral/tvcast/internal/i18n"
	"github.com/usarral/tvcast/internal/idle"
	"github.com/usarral/tvcast/internal/player"
	"github.com/usarral/tvcast/internal/store"
	"github.com/usarral/tvcast/internal/web"
)

func main() {
	var (
		addr      = flag.String("addr", ":8080", "HTTP address")
		dataDir   = flag.String("data", "./data", "data folder (photos, index, token)")
		display   = flag.String("display", "mpv", "display: mpv | web (/tv viewer only)")
		mpvBin    = flag.String("mpv-bin", "mpv", "mpv binary; empty = connect to an mpv already running")
		mpvArgs   = flag.String("mpv-args", "", "extra mpv arguments, space separated (e.g. --vo=drm)")
		mpvSock   = flag.String("mpv-socket", "/tmp/tvcast-mpv.sock", "mpv IPC socket")
		interval  = flag.Duration("interval", 10*time.Second, "time per photo in the slideshow")
		maxFileMB = flag.Int64("max-file-mb", 300, "maximum size per file (MB)")
		maxTotGB  = flag.Float64("max-total-gb", 4, "maximum total space (GB); the oldest items are deleted")
		maxItems  = flag.Int("max-items", 500, "maximum number of items")
		publicURL = flag.String("url", "", "URL for the QR (empty = detect IP)")
		iface     = flag.String("iface", "", "preferred interface to detect the IP (e.g. wlan0)")
		wifiSSID  = flag.String("wifi-ssid", "", "if the box is the access point: SSID for the wifi QR (or TVCAST_WIFI_SSID)")
		wifiPass  = flag.String("wifi-pass", "", "access point password (better via TVCAST_WIFI_PASS)")
		noAuth    = flag.Bool("no-auth", false, "disable the QR token (development only)")
		corner    = flag.Bool("corner-qr", true, "small QR in a corner during the slideshow")
		qrEvery   = flag.Int("qr-every", 12, "show the QR screen every N slideshow photos (0 = never)")
		langFlag  = flag.String("lang", "", "language of the TV screen and default for the phone pages: en | es (or TVCAST_LANG; default en)")
	)
	flag.Parse()
	if *wifiSSID == "" {
		*wifiSSID = os.Getenv("TVCAST_WIFI_SSID")
	}
	if *wifiPass == "" {
		*wifiPass = os.Getenv("TVCAST_WIFI_PASS")
	}
	if *langFlag == "" {
		*langFlag = os.Getenv("TVCAST_LANG")
	}
	lang := i18n.Default
	if *langFlag != "" {
		l, ok := i18n.Parse(*langFlag)
		if !ok {
			log.Fatalf("unknown language %q (supported: %v)", *langFlag, i18n.Supported)
		}
		lang = l
	}

	if err := os.MkdirAll(*dataDir, 0o750); err != nil {
		log.Fatal(err)
	}
	st, err := store.Open(*dataDir, *maxFileMB<<20, int64(*maxTotGB*float64(1<<30)), *maxItems)
	if err != nil {
		log.Fatal(err)
	}

	token, pin := "", ""
	if !*noAuth {
		if token, err = loadToken(filepath.Join(*dataDir, "token")); err != nil {
			log.Fatal(err)
		}
		if pin, err = loadPIN(filepath.Join(*dataDir, "pin")); err != nil {
			log.Fatal(err)
		}
	}

	_, port, _ := net.SplitHostPort(*addr)
	idlePath := filepath.Join(*dataDir, "idle.png")
	cornerPNG := filepath.Join(*dataDir, "corner.png")
	cornerRaw := filepath.Join(*dataDir, "corner.bgra")
	var cornerMu sync.Mutex
	var cornerW, cornerH int
	baseURL := func() string {
		if *publicURL != "" {
			return strings.TrimRight(*publicURL, "/")
		}
		return fmt.Sprintf("http://%s:%s", detectIP(*iface), port)
	}
	renderIdle := func(base string) {
		qr := base + "/"
		if token != "" {
			qr += "?k=" + token
		}
		if err := idle.Render(idlePath, idle.Info{URL: qr, ShowURL: base, WifiSSID: *wifiSSID, WifiPass: *wifiPass, PIN: pin, Lang: lang}); err != nil {
			log.Printf("waiting screen: %v", err)
		}
		w, h, err := idle.Corner(cornerPNG, cornerRaw, qr, pin, lang)
		if err != nil {
			log.Printf("corner QR: %v", err)
		}
		cornerMu.Lock()
		cornerW, cornerH = w, h
		cornerMu.Unlock()
		log.Printf("web: %s  (QR: %s, code: %s)", base, qr, pin)
	}
	curBase := baseURL()
	renderIdle(curBase)

	pl := player.New(st, *interval, *qrEvery)
	srv := &web.Server{
		Store: st, Player: pl, Token: token, PIN: pin, MaxUpload: *maxFileMB << 20, Lang: lang,
		IdlePath:  func() string { return idlePath },
		CornerPNG: func() string { return cornerPNG },
	}
	pl.OnChange(func(player.State) { srv.Notify() })

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	stop := ctx.Done()

	switch *display {
	case "mpv":
		m := &player.MPV{
			Bin: *mpvBin, Args: strings.Fields(*mpvArgs), Socket: *mpvSock,
			Path: func(it *store.Item) string { return st.Path(it) }, IdlePath: func() string { return idlePath },
			OnEnded: pl.Ended,
		}
		if *corner {
			m.Corner = func() (string, int, int, bool) {
				cornerMu.Lock()
				defer cornerMu.Unlock()
				return cornerRaw, cornerW, cornerH, cornerW > 0
			}
		}
		m.Start(stop)
		pl.AddDisplay(m)
	case "web":
		// Only the /tv viewer (Chromium kiosk, or a browser for testing).
	default:
		log.Fatalf("unknown display: %q", *display)
	}
	go pl.Run(stop)

	// When the IP changes (booted before the network, or moved from the home
	// wifi to "Tele") the QR is redrawn. Checking the interfaces every 3 s is cheap.
	go watchURL(stop, 3*time.Second, curBase, baseURL, func(b string) {
		renderIdle(b)
		pl.Redraw() // repaint the QR screen or the corner QR
	})

	hs := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		// No global ReadTimeout/WriteTimeout: video uploads over slow wifi
		// and SSE are long. The size is already limited.
	}
	go func() {
		<-stop
		sctx, c := context.WithTimeout(context.Background(), 3*time.Second)
		defer c()
		hs.Shutdown(sctx)
	}()
	log.Printf("listening on %s, data in %s, display %s, language %s", *addr, *dataDir, *display, lang)
	if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func loadToken(path string) (string, error) {
	if b, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(b))) >= 8 {
		return strings.TrimSpace(string(b)), nil
	}
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	tok := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))
	return tok, os.WriteFile(path, []byte(tok+"\n"), 0o600)
}

// watchURL calls changed every time baseURL() returns something different.
func watchURL(stop <-chan struct{}, every time.Duration, cur string, baseURL func() string, changed func(string)) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-t.C:
			if b := baseURL(); b != cur {
				cur = b
				changed(b)
			}
		}
	}
}

// loadPIN: 6-digit code to type on the web page if the QR can't be scanned.
func loadPIN(path string) (string, error) {
	if b, err := os.ReadFile(path); err == nil {
		if p := strings.TrimSpace(string(b)); len(p) == 6 {
			return p, nil
		}
	}
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	p := fmt.Sprintf("%06d", n.Int64())
	return p, os.WriteFile(path, []byte(p+"\n"), 0o600)
}

// detectIP returns the first private IPv4, preferring the given interface.
func detectIP(prefer string) string {
	ifs, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}
	var fallback string
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.To4() == nil || !ipn.IP.IsPrivate() {
				continue
			}
			if prefer == "" || ifc.Name == prefer {
				return ipn.IP.String()
			}
			if fallback == "" {
				fallback = ipn.IP.String()
			}
		}
	}
	if fallback != "" {
		return fallback
	}
	return "127.0.0.1"
}
