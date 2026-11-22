# To do

## Tomorrow (23 Nov 2026)

- [x] **Boot screen.** Done without Plymouth: kernel console on serial only,
  hidden cursor, image on `/dev/fb0` when systemd starts and login on tty2
  (see `deploy/splash/README.md`). Tested on the box.
- [x] **Console for a few seconds at boot.** It was tvcast killing mpv at
  5 s; now it waits up to 30 s. Tested with a reboot.
- [ ] **Original remote (Bluetooth LE, not IR).** Paired and connected, but no
  key arrives yet. Next: repeat the capture with `btmon` to see whether the
  `0xFF00` service notifications arrive and why the connection drops. Careful:
  the box hung twice with the BCM4334's Bluetooth active; `bluetooth.service`
  is disabled at boot until we understand it. If the internal chip isn't
  reliable: a USB Bluetooth adapter just for the remote.
  Then: a service that listens to the remote and calls the tvcast API
  (arrows → previous/next, OK → pause, Home → QR). See docs/LESSONS.md.
  From the Mac (`tools/mando/sunday_remote.py`) it connects but no keys
  arrive: still to try with a paired link on Linux.
  Also try HDMI-CEC (the TV's remote).
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
      repo with English aliases, keeping the Spanish names.
- [ ] Translate `tools/mando/sunday_remote.py` to English (comments, output;
      keep `--efecto` as an alias if the flag is renamed).
