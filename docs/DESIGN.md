# tvcast: design

A homemade replacement for the Sunday Box software: on the phone you pick
photos (or videos) in a web page served by the box itself, and they show up
full screen on the TV.

Box: Rockchip RK3318 (RK3328 family, 4× Cortex-A53, Mali-450 GPU), 1-2 GB of
RAM, wifi only (Murata module, marked HK6334Q), Armbian without a desktop,
HDMI to the TV. **The house where it goes has no wifi or internet**: the box
creates its own wifi ("Tele", 10.42.0.1).

## 1. The recommendation in one sentence

**A Go binary on the box that serves the web page and drives `mpv` through
its IPC socket, without a desktop.** The box creates its own wifi and the TV
shows two QR codes: one to join the wifi and one to open the page. Video is
accepted, but only "if it fits": no promise that a current phone video plays
smoothly.

## 2. Architecture

```
 Phone (browser)                       Box (Armbian)
 ┌──────────────────┐   HTTP :8080   ┌───────────────────────────────┐
 │ web page         │ ─────────────▶ │ tvcast (Go, ~7 MB, 1 process) │
 │ - pick photos    │  multipart     │  ├─ store: files + index       │
 │ - scale to 1080p │  upload        │  ├─ player: queue, slideshow   │
 │ - remote ◀ ⏸ ▶   │ ◀───────────── │  ├─ web: API + SSE + pages     │
 │ - delete         │  SSE (state)   │  ├─ idle: PNG with QR          │
 └──────────────────┘                │  └─ i18n: en/es strings        │
                                     │        │ JSON IPC (unix socket)│
                                     │        ▼                       │
                                     │  mpv full screen ──── HDMI ───▶ TV
                                     └───────────────────────────────┘
```

- **Server (Go)**: a static arm64 binary, no dependencies on the box except
  mpv. Built on the Mac with `make arm64`. Uses a few MB.
- **Viewer (mpv)**: one process shows photos and videos, is driven through a
  socket (`loadfile ... replace`) and reports when a video ends (`end-file`).
  If mpv dies, tvcast relaunches it and restores what was on screen.
- **`/tv` web viewer** (alternative): the same screen in HTML, for local
  testing or if mpv misbehaves. On the box it would be Chromium in kiosk mode.
- **Waiting screen**: a 1920×1080 PNG that tvcast draws with the page's QR
  (and the wifi QR if the box is the access point). Redrawn when the IP changes.
- **Languages**: English and Spanish. The TV uses the box's language
  (`-lang`/`TVCAST_LANG`, default English); the phone page and its error
  messages use the phone's language when supported. See §9.

### Usage flow

1. The TV shows the waiting screen with the QR.
2. The phone scans it: `http://10.42.0.1/?k=<token>`. The page stores the
   token in a cookie and removes `?k=` from the address bar.
3. "Pick photos or videos" opens the gallery. Each photo **is scaled on the
   phone** to 1080p (applying the EXIF rotation) and uploaded with a 320 px
   thumbnail. On iPhone, Safari converts HEIC to JPEG on the way.
4. Whatever arrives last shows on the TV at once. If nobody sends anything,
   items rotate as a slideshow (10 s per photo; videos until they end).
5. The page shows the live list (SSE) with the current item marked. Tapping
   one shows it; ✕ deletes it; there are ◀ ⏸ ▶ buttons.
6. With no photos, the waiting screen comes back.
7. With photos, the QR doesn't go away: there's a small one in the corner with
   a 6-digit code, and every 12 photos the full QR screen is inserted for one
   interval. Whoever reaches the page without the QR can type the code. With a
   keyboard on the box, Ctrl+Alt+Q pins the QR and Ctrl+Alt+S goes back to the slideshow.

Photos are kept (they survive restarts) with a quota: 4 GB / 500 items by
default. When exceeded, the oldest are deleted.

## 3. Decisions and discarded alternatives

| Decision | Chosen | Discarded and why |
|---|---|---|
| Server language | **Go** | Python+Flask: dependencies to install on the box and heavier startup. Node: more RAM. Go gives one binary you copy and that's it. |
| Viewer | **mpv over IPC** | Chromium kiosk: 300-500 MB of RAM, and the Mali-450 GPU (lima driver) accelerates the browser poorly. feh: images only, no fine control. |
| Resizing | **On the phone** | On the box: decoding 12-48 Mpx on an A53 takes seconds and eats RAM. The phone does it in milliseconds and uploads 10× less data. |
| Real time | **SSE** | WebSocket: more code for nothing; only the server pushes. |
| Database | **index.json** | SQLite: unnecessary for hundreds of items. |
| Translations | **Go map + test** | gettext / x/text: tooling and a build step for ~40 strings in two languages. |
| Main network | **Own access point** | See §4. |

**No X, no i3.** mpv draws straight to the TV through DRM/KMS
(`--vo=gpu --gpu-context=drm`, or `--vo=drm` if the GPU fails). Saves RAM and
parts that can break (screensaver, display power off, windows stealing
focus). If something graphical is ever needed, `cage` (a minimal Wayland
kiosk) before a window manager.

## 4. Network: the box creates its own wifi

The house has no wifi, so **the access point is the main mode**, not an
option. (I used to recommend the home wifi; with this fact it no longer applies.)

- **How**: originally a NetworkManager profile in AP mode with
  `ipv4.method shared`, which starts dnsmasq (DHCP and DNS) on 10.42.0.1,
  created with `deploy/network/hotspot.sh`. **Update:** with this box's
  BCM4334, NM's AP + WPA2 didn't let phones in; hostapd + dnsmasq does, and
  that's what the box runs (see LESSONS.md; bringing it into the repo is in TODO.md).
- **Boot**: the box waits for the known home wifi (about 40 s); if it isn't
  there it brings up "Tele". At your place, while preparing it, it joins your
  wifi; in the house without wifi it raises "Tele" by itself.
- **Web on port 80**: the address is `http://10.42.0.1`, easy to type if the QR fails.
- **Blocking: the chip must support AP mode.** Checked: the HK6334Q
  (Broadcom BCM4334, `brcmfmac` driver) lists `* AP` in `iw list`. If a
  different chip doesn't, or works badly (drops, few clients), the way out is
  a **USB wifi dongle with AP support in the kernel**: e.g. a MediaTek
  MT7601U (2.4 GHz, cheap) or MT7612U/MT7921AU (5 GHz). Avoid Realtek with
  out-of-tree drivers.
- **The phone will say "no internet"** when joining "Tele". That's normal. On
  Android it may ask whether to stay connected: say yes. With mobile data on,
  Android may try to send the page over data; if that happens, the fix is a
  rule so the box answers Android's connectivity checks. Not added from the
  start because then the phone believes it has internet over the wifi and
  WhatsApp stops working while connected.
- **Captive portal** (page opening by itself on join): discarded for now. The
  portal mini-browser is limited and closes by itself. The QR is more reliable.
- **A name instead of an IP** (`tele.local` with avahi): optional. iPhones
  resolve it; Android not always. The QR carries the IP, so it isn't needed.
- **Clock**: no internet means no NTP, and the box has no clock battery. The
  time will be that of the last shutdown (Armbian's fake-hwclock). tvcast
  doesn't depend on the time: it orders by arrival, not by date.

## 5. Video and HDMI on the RK3318

- **HDMI**: the mainline driver (dw-hdmi) works. Better to force 1080p even
  if the TV is 4K: the GPU can't keep up at 4K.
- **Hardware decoding**: the chip has a VPU (H.264 up to 1080p, HEVC/VP9 up
  to 4K), but on mainline Linux it goes through the "stateless" V4L2
  interface and **official mpv/ffmpeg don't use it** (LibreELEC carries its
  own patches). On Armbian, count on **CPU decoding**.
- What that means (estimate, to be measured on the box):
  - H.264 720p: fine. H.264 1080p30: tight, may drop frames.
  - HEVC 1080p (iPhone default) or anything 4K: won't play well.
- So: **photos are the core and video is "if it fits"**. There's a 300 MB
  per-file limit. A future improvement would be re-encoding video on the phone
  to 720p H.264 (WebCodecs) before uploading; not now because it's complex
  and browser support is uneven.

## 6. Security

Nothing is exposed to the internet: the router must not forward ports to the
box, and tvcast makes no outgoing connections.

- **QR token**: without it you can't upload, view or delete. So even when the
  box is on the home wifi, only someone who sees the TV can send. Cookie
  `HttpOnly`, `SameSite=Strict` (covers CSRF). The token lives in
  `data/token`; deleting it logs out every phone.
- **6-digit code**: an alternative to the QR for whoever can't scan it. It's
  weaker than the token, so the login has a global limit of 10 failures every
  10 minutes. It lives in `data/pin`; deleting it generates a new one.
- **File validation**: the type is detected from the bytes, not the
  extension. Allowed: JPEG, PNG, GIF, WebP, MP4, MOV, WebM. HEIC is rejected
  with a clear message. SVG and HTML are rejected (avoids XSS).
- **Names on disk**: generated by the server (random ID). The phone's name is
  only displayed, sanitized. IDs in URLs are validated (16 hex).
- **Translated pages**: rendered with `html/template`, so translated strings
  are escaped in HTML and in the JS object, same as any other data.
- **Limits**: per-file size (`-max-file-mb`), total quota and number of
  items. Header timeouts.
- **Viewer**: `/tv` and the "video ended" notice are only accepted from the
  box itself (127.0.0.1).
- **Access point**: WPA2 with an 8+ character password. The wifi QR shows it
  on the TV: that's intended (whoever is in the living room can join) and
  acceptable because that network gives access to nothing else.
- **Pending**: no HTTPS (on a LAN with an IP there's no valid certificate; the
  token travels in clear over the wifi, which is already WPA2-encrypted). No
  per-IP rate limit.

## 7. Phase 2: original remote

**Update:** it turned out to be Bluetooth LE with a custom (non-HID) service,
not IR. Details and status in LESSONS.md. The original plan, still valid for
any remote that shows up as `/dev/input`:

1. See whether the kernel detects the IR receiver (`/sys/class/rc/rc0`) and
   read each button's code with `ir-keytable -t` (see `CHECKS.md`).
2. Write a keymap (`/etc/rc_keymaps/sundaybox.toml`) that turns each code into
   `KEY_LEFT`, `KEY_RIGHT`, `KEY_OK`...
3. The simplest way to wire keys to tvcast is **triggerhappy** (`thd`): it
   listens on `/dev/input` and runs a command per key, for example
   `KEY_RIGHT 1 curl -s -X POST localhost:8080/api/next`. tvcast needs no
   change, because requests from the box itself need no token.
4. If it works, tvcast could later read `/dev/input` directly.

For the BLE remote, step 3 becomes a small service that subscribes to the
button characteristic and makes the same `curl` calls.

**An alternative that may be better: HDMI-CEC.** The chip has CEC. If the TV
supports it (Anynet+, Bravia Sync, SimpLink...), **the TV's own remote** sends
the arrows to the box, and tvcast could also turn the TV on when a photo
arrives. Worth trying before fighting with the original remote.

## 8. Phase 2: Google Photos

**Checked in Google's documentation**: since 31 March 2025 the Google Photos
Library API **no longer lets apps read the user's library**, only what the
app itself uploaded. Shared albums can't be read through the API either. To
pick existing photos there's the **Picker API**:

- tvcast creates a session and gets a link (`pickerUri`) that can be shown as
  a QR on the TV;
- the user opens it on the phone and picks photos in the Google Photos UI;
- tvcast polls until the selection is done and downloads each photo with an
  OAuth token (URLs expire after 60 minutes, so they must be copied to the
  box; that fits the design).

**My take: not worth it for now.**

- It needs internet (only useful in home-wifi mode), a Google Cloud project,
  an OAuth client and signing in with Google. Google asks to verify the app;
  using it as a "testing app" for personal use is possible, but in that mode
  refresh tokens expire after 7 days (re-authorize every week).
- Signing in on a box without a browser is awkward: Google doesn't allow
  OAuth redirects to a private IP like 10.42.0.1. And in a house without
  internet, any cloud option is out from the start.
- **On Android it's already solved almost for free**: the Android 14+ photo
  picker also shows Google Photos cloud photos. Tapping "Pick photos" in
  tvcast probably already lets you pick Google Photos. Try it on your phones.
- If you have (or want) your own photo server, **Immich** has an API with a
  key and would allow a "TV album" that tvcast syncs, without OAuth or
  verification. Much simpler than Google.
- Discarded: scraping shared albums by link from Google's site. It isn't an
  API, breaks without notice and goes against their terms.

Suggested order: try the Android picker → Immich if you use it → Google's
Picker only if really needed.

## 9. Languages

- English is the default and the language of the code, logs and docs.
  Spanish is kept where the box's users read it: TV screen, phone page, boot
  image, `README.es.md` and `docs/es/INSTALL.md`.
- All user-facing strings live in `internal/i18n` as a Go map, one block per
  language. A test checks every language has every key and the same
  placeholders (`%d`, `{n}`), so a translation can't break formatting.
- The TV uses the box's language (`-lang`, `TVCAST_LANG`). The phone page
  uses `Accept-Language` if it names a supported language, otherwise the
  box's. The page gets its strings through `html/template`; the JS gets the
  whole catalog as an object (`S`) and fills `{name}` placeholders.
- To add a language: copy the `en` block, translate, add the code to
  `Supported`, run `make test`.

## 10. Repository layout

```
tvcast/
├── cmd/tvcast/main.go          startup, flags, IP detection, QR
├── cmd/splash/                 boot image generator (raw framebuffer)
├── internal/
│   ├── store/                  files, validation, quota (+ tests)
│   ├── player/                 queue and slideshow; mpv driver (IPC)
│   ├── web/                    API, SSE, auth; static/ (phone, /tv) (+ tests)
│   ├── idle/                   waiting screen PNG
│   └── i18n/                   en/es strings (+ tests)
├── deploy/
│   ├── systemd/tvcast.service  system service (no X)
│   ├── splash/                 boot image service, console shortcut
│   ├── triggerhappy/           keyboard shortcuts
│   └── network/hotspot.sh      access point (NetworkManager)
├── docs/DESIGN.md, INSTALL.md, CHECKS.md, LESSONS.md; docs/es/INSTALL.md
├── Makefile                    build, arm64, test, run, deploy
└── README.md, README.es.md
```

To do: a workflow that runs `go vet`, `go test` and the arm64 build on every
push, and publishes the arm64 binary in releases.

## 11. Roadmap

1. **Now**: follow `INSTALL.md` at your place (with internet): check AP mode,
   install, test with the "Tele" wifi before taking it.
2. Measure real video from your phones and decide whether it stays.
3. Remote: CEC or the BLE remote, all local, no internet.
4. Photos from the cloud: discarded in that house (no internet).
