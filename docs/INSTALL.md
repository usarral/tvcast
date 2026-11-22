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

## 6b. Keyboard shortcuts (optional)

With a USB keyboard or 2.4 GHz dongle plugged into the box:

| Shortcut | What it does |
|---|---|
| **Ctrl+Alt+Q** | Shows the QR full screen and pins it, even if there are photos or new ones arrive (they are still saved) |
| **Ctrl+Alt+S** | Back to the slideshow |
| **Ctrl+Alt+C** | Stops tvcast and shows the local login (see `deploy/splash`) |
| **Ctrl+Alt+T** | Starts or restarts tvcast |

```sh
sudo apt install -y triggerhappy        # if you didn't in step 1
sudo cp /tmp/tvcast.conf /etc/triggerhappy/triggers.d/   # copied from deploy/triggerhappy/
sudo systemctl restart triggerhappy
```

Without a keyboard, the same over SSH: `curl -X POST 'http://127.0.0.1/api/tv/qr?v=1'`
(and `v=0` to go back). It only works from the box itself.

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
| TV in the wrong language | Set `TVCAST_LANG` in `/etc/tvcast.env` and `sudo systemctl restart tvcast` |
