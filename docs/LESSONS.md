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
- The console framebuffer runs at 2560x1440 (the TV's preferred mode), not
  1920x1080 like mpv: the image must be the size in
  `/sys/class/graphics/fb0/virtual_size`, in BGRX.
- A login on tty1 would draw over the image: it moves to tty2 and Ctrl+Alt+C
  does `chvt 2`. U-Boot's part (first seconds) can't be hidden without rebuilding it.

## Original remote (in progress)

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
