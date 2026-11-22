# Checks on the box

More thorough checks than the ones in INSTALL.md (video, remote, CEC).
Everything is read-only except where it says **(changes something)**.

## 1. System and memory

```sh
uname -a; cat /etc/armbian-release | head -5
free -h                     # 1 or 2 GB? decides whether Chromium is even an option
df -h /                     # room for photos (the default quota is 4 GB)
cat /proc/device-tree/model
```

## 2. Wifi: which chip, and can it be an access point

```sh
nmcli device show wlan0 | head -20
readlink -f /sys/class/net/wlan0/device/driver     # brcmfmac / rtl8xxx / ssv6051...
dmesg | grep -iE 'brcm|wlan|sdio|firmware' | tail -30
iw list | grep -A10 'Supported interface modes'    # is "* AP" there?
iw list | grep -A8 'valid interface combinations'  # AP and managed at once? on how many channels?
iw dev wlan0 info                                  # current channel of the home wifi
```

What to look for:
- `* AP` among the supported modes: an access point is possible.
- A combination like `#{ managed } <= 1, #{ AP } <= 1, total <= 2, #channels <= 1`
  means AP+client is possible only on the router's channel. Not needed in a
  house without wifi.

## 3. Display, GPU and video

```sh
cat /sys/class/drm/card*-HDMI-A-1/modes | head   # resolutions the TV offers
ls /dev/dri/                                    # card0 and renderD128 = GPU (lima) available
dmesg | grep -iE 'lima|hdmi' | tail
ls /dev/video* /dev/media* 2>/dev/null
for d in /dev/video*; do v4l2-ctl -d $d --info 2>/dev/null | grep -E 'Driver|Card'; done
```

**(changes something)** install mpv and tools: `sudo apt install mpv v4l-utils ir-keytable evtest`

Video test (with the TV in front; works over SSH, mpv draws on the TV):

```sh
# Copy a real video from your phones to the box as ~/test.mp4
mpv --vo=gpu --gpu-context=drm --hwdec=auto-safe ~/test.mp4
# Look for "Dropped" or "Audio/Video desynchronisation" in the output; CPU with htop in another session.
ffprobe -hide_banner ~/test.mp4 2>&1 | grep Video   # codec and resolution
```

## 4. Original remote

```sh
ls /sys/class/rc/ 2>/dev/null && cat /sys/class/rc/rc*/protocols
ir-keytable                       # is there an IR receiver? which driver?
sudo ir-keytable -c -p all -t     # (changes protocols until reboot) press each button: note the scancode
sudo evtest                       # lists /dev/input/*: if the remote shows up as a keyboard, press and watch the keys
lsusb                             # if the remote has a USB dongle
bluetoothctl devices 2>/dev/null  # if it were Bluetooth
```

Count the buttons and write down scancode → function (up, down, OK, back...).
(On the Sunday Box it turned out to be Bluetooth LE, not IR: see LESSONS.md.)

## 5. HDMI-CEC (alternative to the original remote)

```sh
ls /dev/cec* 2>/dev/null
sudo apt install v4l-utils        # (changes something) provides cec-ctl
cec-ctl -d /dev/cec0 --playback -S          # does it see the TV?
cec-ctl -d /dev/cec0 -M                     # press arrows on the TV's remote: do they arrive?
```

CEC may need to be enabled in the TV's menu.

## 6. Install

See [INSTALL.md](INSTALL.md).
