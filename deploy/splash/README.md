# Arranque sin consola

Objetivo: que quien encienda la tele no vea nunca texto de Linux.

| Tramo (desde enchufar) | Qué se ve |
| --- | --- |
| U-Boot (~2 s) | negro (no se toca: cambiarlo exige recompilar U-Boot) |
| Kernel, hasta ~7 s | negro: sin `console=tty1`, sin logo y sin cursor |
| systemd, ~7 s | `tvcast-splash` pinta `splash.raw` en `/dev/fb0` |
| ~30-35 s | mpv toma la pantalla (DRM) y sale tvcast |

## Instalar

```sh
go run ./cmd/splash -w 2560 -h 1440 -o splash.raw   # tamaño de /sys/class/graphics/fb0/virtual_size
scp splash.raw deploy/splash/* caja:/tmp/
# en la caja:
sudo cp /boot/armbianEnv.txt /boot/armbianEnv.txt.antes-splash
sudo sed -i 's/^console=.*/console=serial/' /boot/armbianEnv.txt
echo 'extraargs=quiet vt.global_cursor_default=0 logo.nologo' | sudo tee -a /boot/armbianEnv.txt
sudo install -Dm644 /tmp/splash.raw /usr/share/tvcast/splash.raw
sudo install -m644 /tmp/tvcast-splash.service /etc/systemd/system/
sudo install -m755 /tmp/tvcast-consola /usr/local/sbin/
sudo systemctl disable getty@tty1 && sudo systemctl enable getty@tty2 tvcast-splash
```

Ctrl+Alt+C llama a `/usr/local/sbin/tvcast-consola` (para tvcast y cambia a tty2).

## Volver atrás

```sh
sudo cp /boot/armbianEnv.txt.antes-splash /boot/armbianEnv.txt
sudo systemctl disable tvcast-splash getty@tty2 && sudo systemctl enable getty@tty1
```
