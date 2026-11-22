# Booting without a console

Goal: whoever turns on the TV never sees Linux text.

| Stage (from plugging in) | What shows |
| --- | --- |
| U-Boot (~2 s) | black (left alone: changing it means rebuilding U-Boot) |
| Kernel, up to ~7 s | black: no `console=tty1`, no logo and no cursor |
| systemd, ~7 s | `tvcast-splash` draws the image for the screen's resolution on `/dev/fb0` |
| ~30-35 s | mpv takes the screen (DRM) and tvcast shows up |

## Install

`tvcast-splash` asks the framebuffer for its resolution, bits per pixel,
pixel layout and line length, and draws the image for that screen at every
boot (about a second). The text follows `TVCAST_LANG` in `/etc/tvcast.env`.

```sh
make arm64        # builds bin/tvcast-splash-linux-arm64 too
scp bin/tvcast-splash-linux-arm64 deploy/splash/* box:/tmp/
# on the box:
sudo cp /boot/armbianEnv.txt /boot/armbianEnv.txt.antes-splash
sudo sed -i 's/^console=.*/console=serial/' /boot/armbianEnv.txt
echo 'extraargs=quiet vt.global_cursor_default=0 logo.nologo' | sudo tee -a /boot/armbianEnv.txt
sudo install -m755 /tmp/tvcast-splash-linux-arm64 /usr/local/bin/tvcast-splash
sudo install -m644 /tmp/tvcast-splash.service /etc/systemd/system/
sudo install -m755 /tmp/tvcast-consola /usr/local/sbin/
sudo systemctl daemon-reload
sudo systemctl disable getty@tty1 && sudo systemctl enable getty@tty2 tvcast-splash
```

### Updating a box that has the old splash.raw

The first version copied a raw image made for one resolution (2560x1440),
which came out garbled on any other screen. To switch:

```sh
sudo install -m755 /tmp/tvcast-splash-linux-arm64 /usr/local/bin/tvcast-splash
sudo install -m644 /tmp/tvcast-splash.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl restart tvcast-splash && journalctl -u tvcast-splash -n 3
# "splash: 1920x1080, 32 bpp, line 7680 bytes, in ..." and the image on screen
sudo rm /usr/share/tvcast/splash.raw   # no longer used
```

Preview on a computer: `go run ./cmd/splash -png splash.png -w 1920 -h 1080 -lang es`.

Ctrl+Alt+C runs `/usr/local/sbin/tvcast-consola` (stops tvcast and switches
to tty2). The script keeps its Spanish name because boxes already installed
call it by that path.

## Undo

```sh
sudo cp /boot/armbianEnv.txt.antes-splash /boot/armbianEnv.txt
sudo systemctl disable tvcast-splash getty@tty2 && sudo systemctl enable getty@tty1
```
