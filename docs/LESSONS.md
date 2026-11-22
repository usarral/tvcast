# Lessons from the Sunday Box (November 2026)

Notes on what worked and what didn't on the real hardware, so we don't repeat it.

## Hardware

- Rockchip RK3318, board `RK3328_8D4_V1.1`, 1.9 GB usable RAM.
- Wifi: Murata module (marked HK6334Q), `brcmfmac` driver (BCM4334 family).
  `iw list` shows `* AP`: **it can create a network**.
- No Ethernet. The small connector is an optical audio output.
- Armbian trixie (Rk3318-box image), kernel 6.18.

## Access point

- **NetworkManager in AP mode with WPA2 did not work** with this chip: the
  network showed up but phones never finished the password (a PMF/PSK issue
  with `brcmfmac`). Without a password it did connect.
- **hostapd works** with WPA2-PSK (CCMP): an iPhone joined and got an IP over
  DHCP. That's why hostapd is the way (see TODO.md: the box's exact config
  still has to be brought into the repo).
- Booting without the home wifi: wait about 40 s for NetworkManager to find
  the known wifi; if it doesn't show up, bring up "Tele". The first version
  didn't bring it up by itself on boot.

## tvcast

- With photos already uploaded the TV only showed photos and the QR never
  appeared, but the page asked people to scan it. Fixed: small QR with a
  6-digit code in the corner, QR screen every 12 photos, and login with the code.
- When starting before the network, the QR showed 127.0.0.1 for up to a
  minute. Now the IP is checked every 3 s.
- No internet means no NTP and no clock battery: don't rely on the time.
- The TV showed the console for a few seconds on every cold boot. It wasn't
  the network: tvcast waited only 5 s for mpv to create its IPC socket and,
  with the CPU maxed out at boot, mpv takes longer to open DRM. tvcast killed
  it (console visible) and relaunched it 2 s later. The journal gives it away
  with `mpv: no IPC socket` (older builds logged `mpv: sin socket IPC`) right
  after boot. Now it waits up to 30 s while the mpv process is alive.
- Ctrl+Alt+C stops tvcast on purpose (console). If "the TV restarts" after
  touching the keyboard, check `journalctl -t thd` before looking elsewhere.
- Without a clock battery, `journalctl --list-boots` and log times lie until
  NTP arrives: use `-o short-monotonic` to order the boot.

## Booting without a console

- Plymouth isn't worth it on this box: it has to go in the initramfs and when
  it quits it leaves a gap (black or the login) until mpv paints, which takes
  about 8 s cold. Simpler: `console=serial` in `armbianEnv.txt` (kernel and
  systemd write only to the serial port), `vt.global_cursor_default=0
  logo.nologo`, and a raw image copied to `/dev/fb0` as soon as systemd
  starts. Details in `deploy/splash/README.md`.
- The console framebuffer ran at 2560x1440 on the TV (its preferred mode),
  not 1920x1080 like mpv, and at other sizes on other screens: see below.
- A login on tty1 would draw over the image: it moves to tty2 and Ctrl+Alt+C
  does `chvt 2`. U-Boot's part (first seconds) can't be hidden without rebuilding it.

- **The boot image must be drawn for the screen it's on.** The first version
  was a raw BGRX file made beforehand for the TV's 2560x1440 framebuffer and
  copied with `cat` to `/dev/fb0`. On a 1080p portable monitor it came out
  garbled: same bytes, different width and line length. Now `tvcast-splash`
  asks the framebuffer (FBIOGET_VSCREENINFO / FBIOGET_FSCREENINFO) for its
  visible size, bits per pixel, color offsets and line length and draws the
  image at boot. tvcast itself was fine because mpv sets its own DRM mode.

## Original remote

- **It isn't infrared.** The board has an IR receiver (`gpio_ir_recv`, rc0,
  `/dev/lirc0`) with an empty table; with every protocol enabled and
  `ir-keytable -t` nothing arrives when pressing the remote.
- **It's Bluetooth LE**: it always advertises as `Sunday` (on the cardboard
  box: "advertising always on", LED UUID 0xFED0). No key combo is needed to
  pair: `bluetoothctl pair/trust/connect <MAC>` with the remote nearby.
- It isn't a standard HID (no 0x1812 service), so Linux creates no
  `/dev/input`. The UUIDs come from the Android backup (app `Sunrise.apk`,
  package `love.sunday.sunrise.remotecontrol`, decompiled with jadx): buttons
  on the custom service `0xFF00`, characteristic `0x2a4d` (1 byte, or an
  8-byte report with the key in byte 2); voice on
  `f000b000-0451-4000-b000-000000000000` (`b001` start/stop, `b002` audio);
  LED on `0xFED0` (`fed1` color, `fed2` period, `fed3` effect); battery
  `0x180F`. Names accepted by the app: `Sunday` and `4MOD 45 BLE`.
- Holding Mic lights up the remote, same as with Android (the remote does it
  by itself; we didn't turn it on).
- Tested from the Mac with `tools/mando/sunday_remote.py` (Python + bleak):
  it connects without pairing, reads the battery (`2a19`, %) and firmware
  (`v1.19`, manufacturer "4MOD TECHNOLOGY", model `4MOD3401`), and allows
  subscribing to everything, but **no key arrives** and writing to the light
  (`fed1..3`) does nothing visible. The `0xFF00` service is a disguised
  HID-over-GATT (HID information `11 01 00 01`, protocol mode, control point,
  three `2a4d` reports with a `2908` descriptor), but its report map (`2a4b`)
  returns garbage (firmware memory) and reading the reports gives "Invalid
  Offset". Hypothesis: the remote only sends keys over an encrypted (paired)
  link, and Core Bluetooth on macOS doesn't let a script force pairing
  ("Pairing is not available in Core Bluetooth"). BlueZ does pair
  (`Paired/Bonded: yes`), so the real test is on Linux, with `btmon`.
- If a BLE remote stops advertising after odd drops: take the batteries out
  for 10 s and press a button while scanning.
- To read the backup without touching it: the GPT table says where each
  partition is; cut `vendor`/`system` out of `gunzip -c` and read them with
  `debugfs` (e2fsprogs) on the Mac, without mounting.
- **Careful: the whole box hung twice with the internal chip's Bluetooth
  (BCM4334, wifi and Bluetooth on the same chip) active**: once copying
  192 MB over wifi and once with the remote connected. The journal stopped
  abruptly. That's why `bluetooth.service` is disabled at boot, `journald`
  syncs every 2 s and `kernel.panic=10` reboots if the kernel fails. A third
  try stayed connected 2.5 min without hanging, but no button notification
  arrived either: still to be checked with `btmon`. If the internal chip
  isn't reliable, plan B: a USB Bluetooth adapter.

### On Linux with a USB adapter (November 2026)

Tested on a desktop Linux (BlueZ 5.x, CSR USB adapter `0a12:0001`) with
`bluetoothctl`, `btmon` and `bleak`. This answered the open question above:
**keys only arrive over a paired, encrypted link**, which macOS can't force.

- `bluetoothctl`: `scan on`, `pair`, `trust`, `connect`. It bonds **without
  PIN or confirmation** (btmon: LTK exchanged, `Encryption: Enabled with
  AES-CCM`). With the bond stored, `disconnect` + `connect` works again
  without pairing.
- The remote talks to one host at a time. Connection parameters it asks for:
  interval 12.5 ms, latency 44, supervision timeout 20 s.
- BlueZ keeps it connected but doesn't treat it as a keyboard (the `0xFF00`
  service imitates HID under another UUID): nothing in `/dev/input`. You have
  to subscribe over GATT from user space.
- `bleak` can't use a remote that is already connected, because it wants to
  see it advertise while scanning. BlueZ's own D-Bus API doesn't care:
  `Device1.Connect` on the bonded device works either way. That's one reason
  `tvcast-remote` talks to BlueZ directly instead of using bleak.
- Every press sends the button's bytes and every release sends zeros:

| Button | Characteristic | Bytes |
|---|---|---|
| Up | `2a4d` keyboard (8-byte report) | `00 00 52 00 00 00 00 00` |
| Down | `2a4d` keyboard | `00 00 51 00 00 00 00 00` |
| Left | `2a4d` keyboard | `00 00 50 00 00 00 00 00` |
| Right | `2a4d` keyboard | `00 00 4f 00 00 00 00 00` |
| OK | `2a4d` keyboard | `00 00 28 00 00 00 00 00` |
| Back | `2a4d` consumer (1 byte) | `0b` |
| Home | `2a4d` consumer | `05` |
| Play/Pause | `2a4d` consumer | `02` |
| Heart | `2a4d` consumer | `3c` |
| Power | `2a4d` consumer | `04` |
| Vol + | `2a4d` consumer | `09` |
| Vol − | `2a4d` consumer | `0a` |
| Mic | `f000b001` + `f000b002` | `04` on b001, ~170 notifications of 20 bytes (audio) on b002, `00` on b001 on release |

- Keyboard codes are standard HID usages (0x4f–0x52 arrows, 0x28 Enter).
  Consumer codes are one byte with the remote's own numbers, not a bitmask.
  Holding Mic gives the same as a short press. No auto-repeat while held.
- Characteristics that notify: `2a4d` ×2 (keyboard and consumer), `2a22`,
  `fed3`, the four `005f000x-2ff2-4ed5-b045-4c7463617865` of `0xFEF6`, `2a19`
  (battery), `f000b001` and `f000b002`. Only `2a4d` and `f000b00x` sent
  anything when pressing buttons.
- The LED: writing `fed1` (3 bytes), `fed2` and `fed3` with response is
  accepted and ignored (reads back 0, nothing lights). The firmware drives the
  light with Mic. Seen once: the light stopped working after several
  disconnects and came back after reconnecting.

### How tvcast uses it (`tvcast-remote`)

- **Go, not Python.** The repo is Go and the box gets static binaries copied
  over; Python + bleak would mean pip/venv on a box with no internet, and
  bleak's need to see the remote advertise is a problem for reconnecting.
  `godbus` (pure Go, no cgo) is the only new dependency, and the BlueZ D-Bus
  calls needed are few: GetManagedObjects, Device1.Connect, StartNotify, and
  PropertiesChanged signals.
- **Calls tvcast's HTTP API on 127.0.0.1, no uinput.** The API already
  trusts loopback and has next/prev/pause/QR. uinput would need root or a
  udev rule, plus key bindings somewhere to turn keys into actions again.
  Added `v=toggle` to `/api/pause` so OK can flip pause without a read first.
- **Its own service, not inside tvcast.** Bluetooth is the risky part on this
  box; `systemctl disable --now tvcast-remote` leaves the photos untouched,
  and restarting tvcast doesn't drop the link.
- **No address in config.** It picks the paired device named "Sunday" (or
  "4MOD 45 BLE"), on any adapter. Only paired devices count, so another
  remote advertising nearby isn't used. `-addr` exists for two remotes.
- **Reports told apart by length** (8 bytes keyboard, 1 byte consumer), not
  by handle or report-reference descriptor: handles can change between
  firmwares, the length doesn't. The audio characteristic is never subscribed.
- **Reconnect = retry forever every 2 s**, logging a repeated error once.
  BlueZ only auto-reconnects LE devices for profiles that ask for it (HID),
  so for this one someone has to call Connect. Confirmed on the desktop: with
  the bond stored, pressing a key makes the remote advertise but BlueZ doesn't
  connect by itself; `bluetoothctl connect` (the same `Device1.Connect`) does,
  at once, without scanning. Still to check on the box:
  whether the remote goes to sleep when idle and how long the first key
  after waking takes.
- First run on the desktop with the real remote: keys arrive and map as
  expected, and it reconnects by itself about 2 s after the remote is
  switched off and on. On the first connection `StartNotify` on `f000b001`
  can fail with ATT error 0x0e (likely the link isn't encrypted yet); it's
  only logged, it doesn't repeat after reconnecting, and Mic still arrives.
  All 11 keys confirmed. Batteries out and in: back ~2 s after BlueZ notices
  the drop. After reconnecting, ~10 "mic" events arrive before the first real
  key (the remote's start-up noise?). Harmless while Mic is unmapped; **if Mic
  ever gets an action, ignore it for the first seconds after connecting.**
  End to end with a local tvcast and `/tv`: Left/Right change the photo,
  Home shows the QR, Back drops it and Play/Pause pauses. Validated on the
  desktop; the box (USB adapter, internal Bluetooth off) is next.
- **No console or restart from the remote.** Power stays unmapped: one
  accidental press would leave a family member's TV on a login prompt.

### Internal Bluetooth after all (November 2026)

- The plan was a USB adapter for the remote and the internal Bluetooth off.
  Decision: no adapter in the box (the only one stays on the desktop), so
  the remote goes on the internal BCM4334, the chip that hung the box twice.
- Not scanning (tvcast-remote only calls Connect on the bonded device) may
  help, since one hang involved scanning, but it doesn't remove the risk:
  the real use (box as "Tele" + phone uploading + remote connected) is the
  same shape as the 192 MB copy that hung it.
- **`kernel.panic=10` doesn't cover a freeze**: a hard hang never panics. The
  safety net is the SoC's hardware watchdog through systemd
  (`RuntimeWatchdogSec`), so a hang costs about 1.5 min of TV instead of a
  trip to unplug it.
- **The guard can't tell a hang from a power cut** (no clock battery, no
  reliable reset reason), so it only runs during the test period. In a house
  that unplugs the box at night it would turn Bluetooth off for nothing.
- Things worth checking if it hangs again: whether the Bluetooth `.hcd`
  patch loads (without it the chip runs old ROM firmware) and the coexistence
  settings (`btc_*`) in the wifi nvram, comparing with the Android backup.
  Plan B stays a €5 USB adapter.

## English as the main language (November 2026)

- The repo went from Spanish to English (docs, comments, logs, flags) to make
  it usable outside Spain, keeping Spanish where the people who use the box
  read it: the TV screen, the phone page, the boot image, `README.es.md` and
  `docs/es/INSTALL.md`.
- **A plain map beats gettext here.** About 40 strings and two languages: a
  Go map in `internal/i18n` with a test that every language has every key and
  the same placeholders. gettext/x/text would add tooling and a build step for
  no gain. If it ever grows to many languages, revisit.
- **The phone page follows the phone** (`Accept-Language`), the TV follows
  the box (`-lang` / `TVCAST_LANG`). The TV can't know who is watching, the
  phone can. One flag for everything would have forced one language on every
  visitor.
- **The default is English, so an existing Spanish box must set
  `TVCAST_LANG=es`** in `/etc/tvcast.env` before updating the binary, or the
  TV switches to English. The phone page is unaffected (Spanish phones keep
  Spanish).
- **Don't rename what the box already calls.** Paths and names used by
  installed boxes stay as they are even if Spanish: `/usr/local/sbin/tvcast-consola`,
  `armbianEnv.txt.antes-splash`, the "Tele" SSID, and the console commands
  installed on the box (`ayuda`, `casa`, `tele`, `vaciar`, `clave`...). When
  those commands come into the repo, add English aliases rather than renaming.
- Only the user and install docs are kept in two languages. DESIGN, LESSONS,
  CHECKS and TODO are English only: two copies of documents that change every
  week drift apart, and the Spanish versions remain in git history.

## About "nobody has done it"

I haven't found earlier write-ups about reusing the Sunday Box with Armbian.
That doesn't mean nobody has done it. And sending photos from a phone to the
TV is not a new idea: there are homemade photo frames (TeleFrame over
Telegram, photoframe with Google Photos, Pimmich with Immich, picframe…),
digital signage (Obscreen) and file sharing between devices (PairDrop). What
tvcast brings is a specific combination: it works **without internet** (the
box creates its own wifi), **without installing any app** (the phone's
browser and a QR) and on a **€4 recycled TV box** with a single Go binary and mpv.
