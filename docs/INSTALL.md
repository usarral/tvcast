# Installing from scratch

*[En español](es/INSTALL.md)*

From a fresh Armbian to the TV showing the QR, for a house **without wifi or
internet**: the box creates its own wifi ("Tele").

**Prepare everything at your place**, where the box has internet and SSH. You
won't be able to install anything in the other house, so don't forget any
package.

You need:
- The box booting Armbian, on your wifi and with SSH (`ssh user@BOX_IP`).
- The TV (or any HDMI monitor) connected to see the results.
- On your computer: the `tvcast-linux-arm64` binary and the project's `deploy/` folder.
- Recommended: a USB keyboard, in case you lose SSH while changing the network.

In the commands, `user` is your Armbian user.

---

## 1. Setup (on the box, over SSH)

```sh
sudo apt update && sudo apt full-upgrade -y
sudo timedatectl set-timezone Europe/Madrid   # your time zone
sudo apt install -y mpv iw network-manager fake-hwclock
# For phase 2 (remote and CEC), since there will be no internet there:
sudo apt install -y v4l-utils ir-keytable evtest triggerhappy
```

Wifi country (legal channels and power; `ES` for Spain):

```sh
echo "options cfg80211 ieee80211_regdom=ES" | sudo tee /etc/modprobe.d/regdom.conf
```

## 2. Let NetworkManager manage the wifi

```sh
systemctl is-active NetworkManager
nmcli device status
```

- If it says `active` and `wlan0` shows as `connected`: go to step 3.
- If not, another service runs the wifi (netplan with systemd-networkd).
  **Do this with the keyboard on the box**, because you will lose SSH for a moment:

  ```sh
  sudo tee /etc/netplan/90-nm.yaml >/dev/null <<'EOF'
  network:
    version: 2
    renderer: NetworkManager
  EOF
  sudo chmod 600 /etc/netplan/90-nm.yaml
  sudo netplan apply
  sudo nmcli device wifi connect "YOUR_WIFI" password "YOUR_PASSWORD"
  ```

## 3. Can the wifi chip create a network? (blocking step)

```sh
readlink -f /sys/class/net/wlan0/device/driver
iw list | grep -A10 'Supported interface modes'
```

- If **`* AP`** is in the list: good, carry on.
- **If it isn't**, this chip can't be an access point. Don't go on to step 6.
  The fix is a USB wifi dongle with AP support in the kernel (for example a
  MediaTek MT7601U, MT7612U or MT7921AU chip); repeat this step with its
  interface (`wlan1`).

## 4. Install tvcast

On your computer, from the project folder:

```sh
scp bin/tvcast-linux-arm64 deploy/systemd/tvcast.service deploy/network/hotspot.sh user@BOX_IP:/tmp/
```

On the box:

```sh
sudo install -m755 /tmp/tvcast-linux-arm64 /usr/local/bin/tvcast
sudo useradd --system --home-dir /var/lib/tvcast --shell /usr/sbin/nologin tvcast
sudo cp /tmp/tvcast.service /etc/systemd/system/tvcast.service

# Name and password of the wifi the box will create (8 characters or more).
# They are used for the QR on the TV.
sudo tee /etc/tvcast.env >/dev/null <<'EOF'
TVCAST_WIFI_SSID=Tele
TVCAST_WIFI_PASS=put-a-password-here
# Language of the TV screen: en (default) or es. Phones get the web page in
# their own language when supported.
TVCAST_LANG=en
EOF
sudo chmod 600 /etc/tvcast.env

sudo systemctl daemon-reload
sudo systemctl enable --now tvcast
journalctl -u tvcast -f        # Ctrl+C to quit
```

The TV should show the screen with two QR codes. Right now the second QR has
your home IP (BOX_IP): scan it with the phone on your wifi and send a photo
to try it.

**If the TV stays black or shows text** and the log mentions `gpu` or `EGL`:
the GPU doesn't work with mpv. In `/etc/systemd/system/tvcast.service`
replace `--vo=gpu --gpu-context=drm` with `--vo=drm` and:

```sh
sudo systemctl daemon-reload && sudo systemctl restart tvcast
```

## 5. Create the "Tele" wifi

> With the Sunday Box (BCM4334) this step **did not work** with a password:
> NetworkManager's access point with WPA2 doesn't let phones in. The box uses
> hostapd instead (see docs/LESSONS.md and TODO.md; bringing it into the repo
> is pending). This script is fine for chips where NM works.

Use **the same password** as in `/etc/tvcast.env`:

```sh
sudo sh /tmp/hotspot.sh Tele 'put-a-password-here'
```

This creates the network and gives your home wifi higher priority. So at your
place it joins your wifi, and in the other house, not finding it, it creates "Tele".

## 6. Test it at your place before taking it

Simulate the other house (you will lose SSH; if something goes wrong, use the keyboard):

```sh
sudo nmcli connection up tvcast-ap
```

1. In about 30 s the TV changes the QR to `http://10.42.0.1`.
2. With the phone: scan the first QR (join "Tele"). If Android asks about
   "no internet", tell it to stay connected.
3. Scan the second QR and send photos.
4. For SSH: connect your computer to "Tele" and `ssh user@10.42.0.1`.

Boot test as in the other house (without your wifi):

```sh
nmcli -t -f NAME,TYPE connection show    # note your wifi's name
sudo nmcli connection modify "YOUR_WIFI" connection.autoconnect no
sudo reboot
```

After the reboot the "Tele" network should come up by itself, and the TV show
the QR codes. To go back to your wifi (computer connected to "Tele", SSH to 10.42.0.1):

```sh
sudo nmcli connection modify "YOUR_WIFI" connection.autoconnect yes
sudo nmcli connection down tvcast-ap     # it rejoins your wifi by itself
```

## 6b. Admin panel and keyboard

Everything you may need on the box is in a hidden panel on the TV:

- **Keyboard** (USB or 2.4 GHz dongle): **Ctrl+Alt+A**.
- **Remote** (6c): **Heart, Up, Heart, Down** within 5 seconds.

Move with the arrows, **OK** (Enter) to choose, **Back** (Esc) to go back,
**Home** to close. It closes by itself after 2 minutes without keys. While
it is open the slideshow is paused.

| Item | What it does |
|---|---|
| Status (right) | IP and network (home wifi or "Tele"), photos, free space, uptime, remote and its battery, version |
| Show / hide the QR | Pins the QR full screen (you'll see it when you close the panel) |
| Pause / resume | What the slideshow does when you close the panel |
| Network | Home wifi or "Tele" (runs the box's `casa` / `tele`, with their automatic revert), wifi log |
| Logs | tvcast, wifi, remote and Bluetooth, admin actions, kernel |
| Photos | Delete all photos |
| New phone code and QR | Every phone has to scan again |
| Pair the remote again | Forgets the old pairing and pairs the remote nearby |
| Language | Español / English (TV screen and boot image) |
| System | Restart tvcast, reboot, power off, open the console (login on tty2) |

Everything that changes the box asks for confirmation (No / Yes, on No by
default). Without a panel, the keyboard's arrows change photo, Enter pauses
and Esc goes back to the slideshow, like the remote.

### Install

tvcast runs without privileges. The actions that need root go through
systemd: tvcast starts `tvcast-admin@<action>.service`, and a polkit rule lets
the `tvcast` user start those units and nothing else (no sudo).

```sh
sudo apt install -y triggerhappy polkitd   # if missing (needs internet: do it at home)
# from deploy/admin, deploy/systemd and deploy/triggerhappy, copied to /tmp:
sudo install -m755 /tmp/tvcast-admin /usr/local/sbin/
sudo install -m644 /tmp/tvcast-admin@.service /etc/systemd/system/
sudo install -m644 /tmp/50-tvcast-admin.rules /etc/polkit-1/rules.d/
sudo install -m644 /tmp/tvcast.service /etc/systemd/system/   # now reads the journal (logs in the panel)
sudo cp /tmp/tvcast.conf /etc/triggerhappy/triggers.d/
sudo systemctl daemon-reload && sudo systemctl restart tvcast triggerhappy
# Check the rule: this must work as the tvcast user, and stop must not.
sudo -u tvcast systemctl start tvcast-admin@restart-tvcast.service
```

The network items call the box's own `casa` and `tele` commands (not in the
repo yet): if they aren't there, the panel says so in the log.

The old shortcuts (Ctrl+Alt+Q/S/C/T) and `curl -X POST
'http://127.0.0.1/api/tv/qr?v=1'` still work.

## 6c. Official Sunday remote (optional)

The box's original remote is Bluetooth LE, not infrared, and Linux doesn't see
it as a keyboard: `tvcast-remote` (a separate service) listens to it through
BlueZ and calls tvcast's local API.

| Button | What it does |
|---|---|
| **Right / Left** | Next / previous photo |
| **OK** or **Play/Pause** | Pause or resume the slideshow |
| **Home** | Show the QR full screen and pin it; press again to go back |
| **Back** | Back to the slideshow (drops the pinned QR and resumes) |
| **Heart, Up, Heart, Down** | Opens the admin panel (6b); inside it, the arrows, OK, Back and Home drive the menu |
| Vol+/−, Mic, Power | Nothing yet (they show up in the log) |

Power does nothing on purpose, and the console and restarts are only in the
admin panel, behind the sequence and a confirmation, so a family member can't
leave the TV on a login by accident.

**The internal Bluetooth is a risk.** There is no USB adapter in this box, so
the remote uses the internal chip (BCM4334: same chip, radio and antenna as
the wifi). With it on, the box hung twice: once copying 192 MB over wifi and
once with the remote connected. The worst case is the real use: the box as
"Tele", a phone uploading photos and the remote connected. So the steps below
first make a hang recover by itself, then test in phases at home. If it hangs
in the tests: a USB adapter (about €5; the CSR `0a12:0001` is tested) or no
remote.

### 1. Update tvcast first

From the repo, on the computer (`make arm64` first):

```sh
scp bin/tvcast-linux-arm64 bin/tvcast-remote-linux-arm64 deploy/systemd/tvcast-remote.service \
    deploy/bluetooth/tvcast-bt-guard deploy/bluetooth/tvcast-bt-guard.service \
    deploy/bluetooth/watchdog.conf deploy/network/tvcast-netwd \
    deploy/network/tvcast-netwd.service deploy/network/tvcast-netwd.timer tvbox:/tmp/
```

The current binary defaults to English: before installing it on a box that
was in Spanish, add `TVCAST_LANG=es` to `/etc/tvcast.env`. Then copy
`tvcast-linux-arm64` as in step 4 and `sudo systemctl restart tvcast`.

### 2. Safety net (before turning Bluetooth on)

```sh
# Hardware watchdog: a frozen kernel never panics, so kernel.panic=10 alone
# doesn't help; with the watchdog the box resets itself in ~20 s.
ls /dev/watchdog*                      # must exist; if not, stop here
sudo mkdir -p /etc/systemd/system.conf.d
sudo cp /tmp/watchdog.conf /etc/systemd/system.conf.d/tvcast-watchdog.conf
# Guard: after 2 unclean boots in a row it turns Bluetooth off by itself.
sudo install -m755 /tmp/tvcast-bt-guard /usr/local/sbin/
sudo cp /tmp/tvcast-bt-guard.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable tvcast-bt-guard
sudo touch /etc/tvcast-bt-guard.enable
sudo reboot
# After the reboot:
systemctl show -p RuntimeWatchdogUSec  # 20s, not 0
ls /var/log/journal                     # persistent journal (to read a hang afterwards)
```

The files are in `deploy/bluetooth/`. Undo: delete them and reboot.

### Network watchdog

With the internal Bluetooth on and photos uploading, the box lost the wifi
while Linux kept running. The hardware watchdog only catches a frozen kernel,
so it never fired and the box stayed unreachable. `tvcast-netwd` checks the
wifi every 30 s (AP mode: interface in AP mode with an IP; joined to a wifi:
IP and the gateway answers a ping) and reboots after 2 minutes without it.
It waits 5 minutes after boot, and stops after 2 reboots in a row without the
network coming back (no point looping if the router is off).

```sh
# from deploy/network, copied to /tmp:
sudo install -m755 /tmp/tvcast-netwd /usr/local/sbin/
sudo cp /tmp/tvcast-netwd.service /tmp/tvcast-netwd.timer /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now tvcast-netwd.timer
journalctl -u tvcast-netwd -f        # silent while the network is OK
```

Turn off: `sudo systemctl disable --now tvcast-netwd.timer`, or
`TVCAST_NETWD_DISABLE=1` in `/etc/tvcast.env`.

### 3. Look at the chip before using it

```sh
sudo dmesg | grep -iE 'bluetooth|hci|bcm'
```

- A line like `BCM: Patch brcm/BCM4334B0.hcd` (or similar) means the
  Bluetooth firmware patch loaded. If it says the `.hcd` was **not found**,
  the chip runs its old ROM firmware, a likely cause of hangs: look for the
  `.hcd` in the Android backup (`/vendor/firmware`, `/system/etc/firmware`)
  and copy it to `/lib/firmware/brcm/` with the name the log asks for.
- `grep -i btc /lib/firmware/brcm/brcmfmac4334*.txt`: the wifi/Bluetooth
  coexistence settings (`btc_mode`...). If there are none and the Android
  nvram in the backup has them, try those. One change at a time.

### 4. Pair the remote

It talks to one host at a time: first remove it on the computer it is paired
with (`bluetoothctl remove <address>` there, or turn its Bluetooth off).

Pairing needs an agent **in the same bluetoothctl session**: a separate
`bluetoothctl pair <address>` fails with `Authentication Failed (0x05)`.

```sh
sudo systemctl enable --now bluetooth
bluetoothctl            # then, all in this one session:
#   agent NoInputNoOutput
#   default-agent
#   scan on             (wait for "Sunday")
#   pair <address>
#   trust <address>
#   scan off
#   quit
```

No PIN. If it doesn't show up, take the batteries out for 10 s. Later on,
the admin panel's "Pair the remote again" does the same.

If `bluetoothctl list` shows no controller, the internal chip didn't come up
(`dmesg`: `Failed to set baudrate`, error -110). Reloading the driver brings
it back: `sudo modprobe -r hci_uart btbcm && sudo modprobe hci_uart`.

### 5. Install tvcast-remote

```sh
sudo install -m755 /tmp/tvcast-remote-linux-arm64 /usr/local/bin/tvcast-remote
sudo /usr/local/bin/tvcast-remote -dry-run   # press buttons, Ctrl+C to stop
sudo cp /tmp/tvcast-remote.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now tvcast-remote
journalctl -u tvcast-remote -f                # one line per key
```

It reconnects by itself with the stored pairing (after a reboot, the remote
going to sleep or out of range) and needs no name or address in any config.
After reconnecting, about ten "mic (not mapped)" lines are normal.

### 6. Test in phases, at home, with the guard on

1. One hour idle with the remote connected, pressing keys now and then.
2. On the home wifi: upload 30–50 photos from a phone while pressing keys.
   Three times.
3. In "Tele" mode, the same upload from a phone joined to "Tele".
4. A night switched on.

After each phase: `uptime` (no surprise reboot) and
`journalctl -k -b | grep -iE 'brcmf|bluetooth|hci'`.

- **All good**: before taking it to the other house, `sudo rm
  /etc/tvcast-bt-guard.enable` (there it may be unplugged every night, and
  each power cut would count as a hang). The watchdog stays.
- **It hung**: the watchdog reboots it; after two in a row the guard turns
  Bluetooth off and the box works as before without the remote. What
  happened just before: `journalctl -b -1 -k -n 50`. Then: USB adapter.
- To turn the remote off by hand: `sudo systemctl disable --now tvcast-remote bluetooth`.

## 7. In the other house

1. Plug in HDMI and power. Turn the TV on **before** or together with the box.
2. After about a minute the two QR codes show up.
3. For the family: "scan the first one to join the TV's wifi; then the second
   one and pick photos". The phone that joins will say "no internet": that's normal.
4. Once photos are sent, the QR is still at hand: there's a small one with a
   6-digit code at the bottom right, and every 12 photos the full QR screen
   shows for a moment. Whoever opens `http://10.42.0.1` without the QR can type that code.

For maintenance there: connect your computer to "Tele" and `ssh user@10.42.0.1`.
To update tvcast: copy the new binary as in step 4 (to 10.42.0.1) and
`sudo systemctl restart tvcast`.

## Common problems

| Symptom | What to check |
|---|---|
| The TV shows nothing | Was the TV turned on after the box? `sudo systemctl restart tvcast`. Log: `journalctl -u tvcast -n 50` |
| The "Tele" network doesn't appear | `nmcli connection show --active`, `journalctl -u NetworkManager -n 50`. Did step 3 show `* AP`? |
| The phone is on "Tele" but the page won't load | Android may route traffic over mobile data: turn data off for a moment or accept "stay connected" |
| "HEIC is not supported" | Send from the phone's browser (Safari converts it) or on the iPhone: Settings → Camera → Formats → Most Compatible |
| A video stutters | Expected with 1080p HEVC or 4K videos (see DESIGN §5) |
| Videos without sound | `mpv --audio-device=help` to find the HDMI output and add it in `-mpv-args` (`--audio-device=alsa/...`) |
| Wrong time on the box | Normal without internet; tvcast doesn't care |
| The remote does nothing | `journalctl -u tvcast-remote -n 30`: "no paired device" means pair it again (6c); "retrying" lines while you press keys mean it can't connect: batteries, or take them out for 10 s |
| TV in the wrong language | Set `TVCAST_LANG` in `/etc/tvcast.env` and `sudo systemctl restart tvcast` |
