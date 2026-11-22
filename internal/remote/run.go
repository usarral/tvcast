// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 usarral

package remote

import (
	"context"
	"log"
	"time"
)

// Link is a connection to the remote. Listen connects if needed, subscribes,
// calls emit for every notification and blocks until the link drops (error)
// or ctx ends. BlueZ implements it; tests use a fake.
type Link interface {
	Listen(ctx context.Context, emit func(Notification)) error
}

// Run keeps the link up forever: when Listen returns, it waits retry and
// tries again. The remote goes quiet when idle or out of range and BlueZ only
// reconnects when asked, so retrying is the normal path, not an error path.
// A repeated error is logged once, so a remote that is off for a week doesn't
// fill the journal.
func Run(ctx context.Context, link Link, emit func(Notification), retry time.Duration, lg *log.Logger) error {
	last := ""
	for {
		start := time.Now()
		err := link.Listen(ctx, emit)
		if time.Since(start) > time.Minute {
			last = "" // it was up for a while: log the next drop again
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		msg := "link closed"
		if err != nil {
			msg = err.Error()
		}
		if msg != last {
			lg.Printf("remote: %s; retrying every %v", msg, retry)
			last = msg
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retry):
		}
	}
}
