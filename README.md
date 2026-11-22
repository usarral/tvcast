# tvcast

*[Leer en español](README.es.md)*

Send photos (and videos) from your phone to the TV. Built for a recycled TV
box (Sunday Box, RK3318) running Armbian: the box serves a web page, the TV
shows a QR code, and anyone in the living room can send photos that appear
full screen.

- One Go binary with no dependencies (+ `mpv` on the box).
- Photos are scaled down to 1080p on the phone itself before uploading.
- Automatic slideshow, delete, previous/next/pause.
- Access with the QR on the TV or with the 6-digit code it shows.
- Works without internet: the box creates its own wifi.
- English and Spanish (TV screen, phone page, boot image).

I haven't found earlier write-ups about reusing the Sunday Box like this, but
that doesn't mean nobody has done it, and sending photos to a TV is not a new
idea. See [docs/LESSONS.md](docs/LESSONS.md#about-nobody-has-done-it).

Design and decisions: [docs/DESIGN.md](docs/DESIGN.md).
What we learned on the real hardware: [docs/LESSONS.md](docs/LESSONS.md). To do: [TODO.md](TODO.md).
Install: [docs/INSTALL.md](docs/INSTALL.md) ([en español](docs/es/INSTALL.md)). Extra checks: [docs/CHECKS.md](docs/CHECKS.md).

## Try it locally (Mac or Linux)

Needs Go 1.25+.

```sh
make test
make run            # web viewer: open http://localhost:8080/tv and, in another tab, http://localhost:8080
```

With mpv installed (`brew install mpv`) you can try the real viewer:
`./bin/tvcast -data ./data` (opens a full-screen mpv window; press `q` in it
and tvcast will reopen it; Ctrl+C to quit).

From the phone, on the same wifi, scan the QR or open the `?k=` URL printed
in the log.

## Install on the box

Step by step, from a fresh Armbian: [docs/INSTALL.md](docs/INSTALL.md).

## Language

- **TV screen** (QR screen, corner QR): `-lang en|es` or `TVCAST_LANG`
  (default `en`). On a box for a Spanish-speaking home, put
  `TVCAST_LANG=es` in `/etc/tvcast.env`.
- **Phone page**: follows the phone's language when it is English or
  Spanish; otherwise the box's `-lang`.
- **Boot image**: `go run ./cmd/splash -lang es ...` (see `deploy/splash`).

Strings live in [`internal/i18n`](internal/i18n/i18n.go); to add a language,
copy the `en` block and add it to `Supported` (a test checks no key is missing).

## Main options

| Flag | Default | |
|---|---|---|
| `-addr` | `:8080` | HTTP port |
| `-data` | `./data` | photos, index, token and waiting screen |
| `-display` | `mpv` | `mpv` or `web` (only the `/tv` viewer) |
| `-mpv-args` | | extra mpv arguments, e.g. `--vo=drm` |
| `-interval` | `10s` | time per photo |
| `-max-file-mb` / `-max-total-gb` / `-max-items` | 300 / 4 / 500 | limits |
| `-iface` | | interface for the IP in the QR (`wlan0`) |
| `-wifi-ssid` (or `TVCAST_WIFI_SSID`) + `TVCAST_WIFI_PASS` | | adds the join-the-wifi QR to the TV |
| `-lang` (or `TVCAST_LANG`) | `en` | TV language and default for the phone page: `en`, `es` |
| `-corner-qr` | `true` | small QR with the code in a corner during the slideshow |
| `-qr-every` | `12` | show the QR screen every N photos (0 = never) |
| `-no-auth` | `false` | no token (development only) |

## API

Everything needs the token cookie except from 127.0.0.1. The cookie comes
from opening the QR's URL (`/?k=<token>`) or from the 6-digit code on the TV
(`POST /api/login`, `pin=...`; 10 failures in 10 min block the login).
Error messages are plain text in the phone's language (`Accept-Language`).

| Method | Path | |
|---|---|---|
| GET | `/api/state` | state and list |
| GET | `/api/events` | the same over SSE on every change |
| POST | `/api/upload` | multipart: `thumb` (optional, JPEG) then `file` |
| POST | `/api/show/{id}` · `/api/next` · `/api/prev` | what is on screen |
| POST | `/api/pause?v=1\|0\|toggle` | pause, resume or flip (remote: OK, Play/Pause) |
| DELETE | `/api/items/{id}` | delete |
| POST | `/api/tv/qr?v=1\|0\|toggle` | box only: pin or release the QR screen (shortcuts Ctrl+Alt+Q / Ctrl+Alt+S) |

Example for a remote or script on the box itself:
`curl -X POST localhost:8080/api/next`.
