# Comprobaciones en la caja

Comprobaciones más a fondo que las de INSTALL.md (vídeo, mando, CEC). Todo es
de solo lectura salvo donde pone **(cambia algo)**.

## 1. Sistema y memoria

```sh
uname -a; cat /etc/armbian-release | head -5
free -h                     # ¿1 o 2 GB? decide si Chromium es opción siquiera
df -h /                     # espacio para las fotos (la cuota por defecto es 4 GB)
cat /proc/device-tree/model
```

## 2. Wifi: qué chip es y si puede hacer de punto de acceso

```sh
nmcli device show wlan0 | head -20
readlink -f /sys/class/net/wlan0/device/driver     # brcmfmac / rtl8xxx / ssv6051...
dmesg | grep -iE 'brcm|wlan|sdio|firmware' | tail -30
iw list | grep -A10 'Supported interface modes'    # ¿aparece "* AP"?
iw list | grep -A8 'valid interface combinations'  # ¿AP y managed a la vez? ¿en cuántos canales?
iw dev wlan0 info                                  # canal actual de la wifi de casa
```

Qué buscar:
- `* AP` en los modos soportados: el punto de acceso es posible.
- Una combinación tipo `#{ managed } <= 1, #{ AP } <= 1, total <= 2, #channels <= 1`
  significa que AP+cliente es posible solo en el mismo canal que el router.
  No hace falta en una casa sin wifi.

## 3. Pantalla, GPU y vídeo

```sh
cat /sys/class/drm/card*-HDMI-A-1/modes | head   # resoluciones que ofrece la tele
ls /dev/dri/                                    # card0 y renderD128 = GPU (lima) disponible
dmesg | grep -iE 'lima|hdmi' | tail
ls /dev/video* /dev/media* 2>/dev/null
for d in /dev/video*; do v4l2-ctl -d $d --info 2>/dev/null | grep -E 'Driver|Card'; done
```

**(cambia algo)** instalar mpv y herramientas: `sudo apt install mpv v4l-utils ir-keytable evtest`

Prueba de vídeo (con la tele delante; sirve por SSH, mpv pinta en la tele):

```sh
# Copia un vídeo real de vuestros móviles a la caja como ~/prueba.mp4
mpv --vo=gpu --gpu-context=drm --hwdec=auto-safe ~/prueba.mp4
# Mira en la salida si hay "Dropped" o "Audio/Video desynchronisation"; CPU con htop en otra sesión.
ffprobe -hide_banner ~/prueba.mp4 2>&1 | grep Video   # códec y resolución
```

## 4. Mando oficial

```sh
ls /sys/class/rc/ 2>/dev/null && cat /sys/class/rc/rc*/protocols
ir-keytable                       # ¿hay receptor IR? ¿qué driver?
sudo ir-keytable -c -p all -t     # (cambia protocolos hasta reiniciar) pulsa cada botón: anota el scancode
sudo evtest                       # lista /dev/input/*: si el mando sale como teclado, pulsa y mira las teclas
lsusb                             # si el mando trae pincho USB
bluetoothctl devices 2>/dev/null  # si fuera Bluetooth
```

Cuenta los botones y anota scancode → función (arriba, abajo, OK, atrás...).

## 5. HDMI-CEC (alternativa al mando original)

```sh
ls /dev/cec* 2>/dev/null
sudo apt install v4l-utils        # (cambia algo) trae cec-ctl
cec-ctl -d /dev/cec0 --playback -S          # ¿ve la tele?
cec-ctl -d /dev/cec0 -M                     # pulsa flechas en el mando de la TELE: ¿llegan?
```

Puede que haya que activar CEC en el menú de la tele.

## 6. Instalación

Ver [INSTALL.md](INSTALL.md).
