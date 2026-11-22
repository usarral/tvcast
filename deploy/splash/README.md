# Booting without a console

Goal: whoever turns on the TV never sees Linux text.

| Stage (from plugging in) | What shows |
| --- | --- |
| U-Boot (~2 s) | black (left alone: changing it means rebuilding U-Boot) |
| Kernel, up to ~7 s | black: no `console=tty1`, no logo and no cursor |
| systemd, ~7 s | `tvcast-splash` paints `splash.raw` on `/dev/fb0` |
| ~30-35 s | mpv takes the screen (DRM) and tvcast shows up |

## Install

```sh
# Size from /sys/class/graphics/fb0/virtual_size. -lang en|es sets the text.
go run ./cmd/splash -w 2560 -h 1440 -lang es -o splash.raw
scp splash.raw deploy/splash/* box:/tmp/
# on the box:
sudo cp /boot/armbianEnv.txt /boot/armbianEnv.txt.antes-splash
sudo sed -i 's/^console=.*/console=serial/' /boot/armbianEnv.txt
echo 'extraargs=quiet vt.global_cursor_default=0 logo.nologo' | sudo tee -a /boot/armbianEnv.txt
sudo install -Dm644 /tmp/splash.raw /usr/share/tvcast/splash.raw
sudo install -m644 /tmp/tvcast-splash.service /etc/systemd/system/
sudo install -m755 /tmp/tvcast-consola /usr/local/sbin/
sudo systemctl disable getty@tty1 && sudo systemctl enable getty@tty2 tvcast-splash
```

Ctrl+Alt+C runs `/usr/local/sbin/tvcast-consola` (stops tvcast and switches
to tty2). The script keeps its Spanish name because boxes already installed
call it by that path.

## Undo

```sh
sudo cp /boot/armbianEnv.txt.antes-splash /boot/armbianEnv.txt
sudo systemctl disable tvcast-splash getty@tty2 && sudo systemctl enable getty@tty1
```
