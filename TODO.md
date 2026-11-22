# To do

## Tomorrow (23 Nov 2026)

- [x] **Boot screen.** Done without Plymouth: kernel console on serial only,
  hidden cursor, image on `/dev/fb0` when systemd starts and login on tty2
  (see `deploy/splash/README.md`). Tested on the box.
- [x] **Console for a few seconds at boot.** It was tvcast killing mpv at
  5 s; now it waits up to 30 s. Tested with a reboot.
- [ ] **Original remote (Bluetooth LE, not IR).** Keys decoded on desktop
  Linux with a USB adapter (docs/LESSONS.md) and `tvcast-remote` written:
  Right/Left photo, OK and Play/Pause pause, Home QR, Back slideshow.
  Next:
  - [x] Run `tvcast-remote -dry-run` on the desktop with the paired remote:
        all keys, reconnects alone in ~2 s.
  - [x] Against a local tvcast with `/tv` open: works.
  - [x] On the box with the internal Bluetooth: the remote works, but with
        photo uploads the network drops (system alive). Guard turned BT off
        after two unclean boots; `tvcast-netwd` added (docs/LESSONS.md).
  - [ ] Solder the wifi antenna, then repeat with internal BT and uploads
        limited with `tc`. If the network still drops: €5 USB adapter on
        the xHCI port.
  - [ ] Leave it a day: does the remote sleep when idle, how long does the
        first key take after waking, does the box stay up.
  - [ ] Maybe later: Up/Down, Heart and Vol (nothing obvious yet), HDMI-CEC
        with the TV's own remote.
- [ ] **hostapd as the default.** Bring the config that works on the box into
  the repo (hostapd.conf, boot service that waits for the home wifi and falls
  back) and replace `deploy/network/hotspot.sh` (NetworkManager in AP mode +
  WPA2 didn't work with this BCM4334: see docs/LESSONS.md).
- [ ] **QR with 127.0.0.1 at boot.** Fixed in code (tvcast checks the IP every
  3 s and redraws the QR; it used to be every 30 s and on the box it was
  solved by restarting the service). Deploy and check on a cold boot; if it
  works, drop the tvcast restart in the "Tele" script.
- [ ] **Language on the box.** Before deploying the English-default binary,
  add `TVCAST_LANG=es` to `/etc/tvcast.env` on the box, and use `-lang es`
  if the splash image is ever regenerated.

## Later

- [ ] GitHub Actions workflow: `go vet`, `go test` and arm64 build on every
      push; binary in Releases.
- [ ] Pick a license if it goes public (MIT or GPL-3.0).
- [ ] Video: measure with real videos from the family's phones.
- [ ] Re-encode video on the phone (WebCodecs) if video is worth it.
- [ ] Bring the box's console commands (`ayuda`, `casa`, `tele`...) into the
      repo with English aliases, keeping the Spanish names. The admin panel
      already calls `casa` and `tele` (deploy/admin/tvcast-admin).
- [ ] Admin panel on the box: install (docs/INSTALL.md 6b), check the polkit
      rule, open it with Ctrl+Alt+A and with the remote's sequence.
- [ ] Translate `tools/mando/sunday_remote.py` to English (comments, output;
      keep `--efecto` as an alias if the flag is renamed).
