// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 usarral

// tvcast-remote listens to the official Sunday Box Bluetooth LE remote through
// BlueZ and turns its keys into calls to tvcast's local API.
//
// It runs as its own service (deploy/systemd/tvcast-remote.service), apart from
// tvcast: if Bluetooth misbehaves, stopping this service leaves the photos
// working, and restarting tvcast doesn't drop the Bluetooth link.
//
// One-time setup: pair and trust the remote with bluetoothctl (docs/INSTALL.md).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/usarral/tvcast/internal/remote"
)

type logOnly struct{ lg *log.Logger }

func (l logOnly) Post(_ context.Context, p string) error {
	l.lg.Printf("remote: dry run: POST %s", p)
	return nil
}

func main() {
	api := flag.String("api", "http://127.0.0.1", "tvcast base URL (must be loopback: the API trusts 127.0.0.1)")
	addr := flag.String("addr", os.Getenv("TVCAST_REMOTE_ADDR"), "remote's Bluetooth address (default: any paired device with one of -names)")
	names := flag.String("names", strings.Join(remote.DefaultNames, ","), "comma-separated names the remote may advertise")
	retry := flag.Duration("retry", 2*time.Second, "wait between reconnection attempts")
	connTimeout := flag.Duration("connect-timeout", 20*time.Second, "limit for one connection attempt")
	dry := flag.Bool("dry-run", false, "log the actions instead of calling tvcast")
	flag.Parse()

	lg := log.New(os.Stderr, "", 0) // journald adds the time
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		lg.Fatalf("remote: system D-Bus: %v", err)
	}
	defer conn.Close()

	var poster remote.Poster = &remote.API{Base: *api}
	if *dry {
		poster = logOnly{lg}
	}
	d := &remote.Dispatcher{API: poster, Keymap: remote.DefaultKeymap, Log: lg}
	link := &remote.BlueZ{Conn: conn, Address: *addr, Names: strings.Split(*names, ","),
		ConnectTimeout: *connTimeout, Log: lg,
		// Shown in tvcast's admin panel. Best effort: tvcast may be restarting.
		OnState: func(connected bool, battery int) {
			c := "0"
			if connected {
				c = "1"
			}
			go poster.Post(context.Background(), fmt.Sprintf("/api/tv/remote?connected=%s&battery=%d", c, battery))
		}}

	// Keys are handled one at a time, in order, on the D-Bus goroutine:
	// each is one quick local HTTP call (3 s timeout at most).
	err = remote.Run(ctx, link, func(n remote.Notification) { d.Handle(ctx, n) }, *retry, lg)
	if err != nil && ctx.Err() == nil {
		lg.Fatal(err)
	}
}
