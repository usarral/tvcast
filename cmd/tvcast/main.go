// tvcast: envía fotos y vídeos desde el móvil a la tele.
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

	"github.com/usarral/tvcast/internal/idle"
	"github.com/usarral/tvcast/internal/player"
	"github.com/usarral/tvcast/internal/store"
	"github.com/usarral/tvcast/internal/web"
)

func main() {
	var (
		addr      = flag.String("addr", ":8080", "dirección HTTP")
		dataDir   = flag.String("data", "./data", "carpeta de datos (fotos, índice, token)")
		display   = flag.String("display", "mpv", "pantalla: mpv | web (solo /tv) ")
		mpvBin    = flag.String("mpv-bin", "mpv", "binario de mpv; vacío = conectarse a un mpv ya lanzado")
		mpvArgs   = flag.String("mpv-args", "", "argumentos extra para mpv, separados por espacios (p. ej. --vo=drm)")
		mpvSock   = flag.String("mpv-socket", "/tmp/tvcast-mpv.sock", "socket IPC de mpv")
		interval  = flag.Duration("interval", 10*time.Second, "tiempo por foto en el pase")
		maxFileMB = flag.Int64("max-file-mb", 300, "tamaño máximo por fichero (MB)")
		maxTotGB  = flag.Float64("max-total-gb", 4, "espacio máximo total (GB); se borran los más antiguos")
		maxItems  = flag.Int("max-items", 500, "número máximo de elementos")
		publicURL = flag.String("url", "", "URL para el QR (vacío = detectar IP)")
		iface     = flag.String("iface", "", "interfaz preferida para detectar la IP (p. ej. wlan0)")
		wifiSSID  = flag.String("wifi-ssid", "", "si la caja hace de punto de acceso: SSID para el QR de wifi (o TVCAST_WIFI_SSID)")
		wifiPass  = flag.String("wifi-pass", "", "contraseña del punto de acceso (mejor por variable TVCAST_WIFI_PASS)")
		noAuth    = flag.Bool("no-auth", false, "desactiva el token del QR (solo desarrollo)")
		corner    = flag.Bool("corner-qr", true, "QR pequeño en una esquina durante el pase")
		qrEvery   = flag.Int("qr-every", 12, "intercala la pantalla de QR cada N fotos del pase (0 = nunca)")
	)
	flag.Parse()
	if *wifiSSID == "" {
		*wifiSSID = os.Getenv("TVCAST_WIFI_SSID")
	}
	if *wifiPass == "" {
		*wifiPass = os.Getenv("TVCAST_WIFI_PASS")
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
		if err := idle.Render(idlePath, idle.Info{URL: qr, ShowURL: base, WifiSSID: *wifiSSID, WifiPass: *wifiPass, PIN: pin}); err != nil {
			log.Printf("pantalla de espera: %v", err)
		}
		w, h, err := idle.Corner(cornerPNG, cornerRaw, qr, pin)
		if err != nil {
			log.Printf("QR de esquina: %v", err)
		}
		cornerMu.Lock()
		cornerW, cornerH = w, h
		cornerMu.Unlock()
		log.Printf("web: %s  (QR: %s, código: %s)", base, qr, pin)
	}
	curBase := baseURL()
	renderIdle(curBase)

	pl := player.New(st, *interval, *qrEvery)
	srv := &web.Server{
		Store: st, Player: pl, Token: token, PIN: pin, MaxUpload: *maxFileMB << 20,
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
		// Solo el visor /tv (Chromium kiosco o navegador para pruebas).
	default:
		log.Fatalf("display desconocido: %q", *display)
	}
	go pl.Run(stop)

	// Si cambia la IP (arranque sin red todavía, o paso de la wifi de casa a
	// "Tele") se rehace el QR. Mirar las interfaces cada 3 s es barato.
	go watchURL(stop, 3*time.Second, curBase, baseURL, func(b string) {
		renderIdle(b)
		pl.Redraw() // repinta la pantalla de QR o el QR de la esquina
	})

	hs := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		// Sin ReadTimeout/WriteTimeout globales: las subidas de vídeo por
		// wifi lenta y el SSE son largos. El tamaño ya está limitado.
	}
	go func() {
		<-stop
		sctx, c := context.WithTimeout(context.Background(), 3*time.Second)
		defer c()
		hs.Shutdown(sctx)
	}()
	log.Printf("escuchando en %s, datos en %s, pantalla %s", *addr, *dataDir, *display)
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

// watchURL llama a changed cada vez que baseURL() devuelve algo distinto.
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

// loadPIN: código de 6 cifras para teclear en la web si el QR no se lee.
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

// detectIP devuelve la primera IPv4 privada, prefiriendo la interfaz pedida.
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
