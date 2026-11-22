# Instalación desde cero

*[English](../INSTALL.md). Las rutas `deploy/...` son relativas a la raíz del repositorio.*

Desde Armbian recién instalado hasta la tele mostrando el QR, para una casa
**sin wifi ni internet**: la caja crea su propia wifi ("Tele").

**Todo se prepara en tu casa**, donde la caja tiene internet y SSH. En la otra
casa no podrás instalar nada, así que no te dejes ningún paquete.

Necesitas:
- La caja con Armbian arrancando, en tu wifi y con SSH (`ssh usuario@IP_DE_LA_CAJA`).
- La tele (o cualquier monitor HDMI) conectada para ver los resultados.
- En el Mac: el binario `tvcast-linux-arm64` y la carpeta `deploy/` del proyecto.
- Recomendado: un teclado USB, por si te quedas sin SSH al cambiar la red.

En los comandos, `usuario` es tu usuario de Armbian.

---

## 1. Puesta a punto (en la caja, por SSH)

```sh
sudo apt update && sudo apt full-upgrade -y
sudo timedatectl set-timezone Europe/Madrid
sudo apt install -y mpv iw network-manager fake-hwclock
# Para la fase 2 (mando y CEC), que allí no habrá internet:
sudo apt install -y v4l-utils ir-keytable evtest triggerhappy
```

País de la wifi (canales y potencia legales en España):

```sh
echo "options cfg80211 ieee80211_regdom=ES" | sudo tee /etc/modprobe.d/regdom.conf
```

## 2. Que NetworkManager gestione la wifi

```sh
systemctl is-active NetworkManager
nmcli device status
```

- Si sale `active` y `wlan0` aparece como `connected`: sigue al paso 3.
- Si no, la wifi la lleva otro servicio (netplan con systemd-networkd).
  **Hazlo con el teclado en la caja**, porque vas a perder el SSH un momento:

  ```sh
  sudo tee /etc/netplan/90-nm.yaml >/dev/null <<'EOF'
  network:
    version: 2
    renderer: NetworkManager
  EOF
  sudo chmod 600 /etc/netplan/90-nm.yaml
  sudo netplan apply
  sudo nmcli device wifi connect "TU_WIFI" password "TU_CONTRASEÑA"
  ```

## 3. ¿El chip wifi puede crear una red? (paso bloqueante)

```sh
readlink -f /sys/class/net/wlan0/device/driver
iw list | grep -A10 'Supported interface modes'
```

- Si en la lista aparece **`* AP`**: perfecto, sigue.
- **Si no aparece**, este chip no puede hacer de punto de acceso. No sigas con
  el paso 6. La solución es un pincho wifi USB con soporte AP en el kernel
  (por ejemplo chip MediaTek MT7601U, MT7612U o MT7921AU) y repetir este paso
  con su interfaz (`wlan1`).

Copia la salida de estos dos comandos en el hilo: así sé qué chip es.

## 4. Instalar tvcast

En el Mac, desde la carpeta del proyecto:

```sh
scp bin/tvcast-linux-arm64 deploy/systemd/tvcast.service deploy/network/hotspot.sh usuario@IP_DE_LA_CAJA:/tmp/
```

En la caja:

```sh
sudo install -m755 /tmp/tvcast-linux-arm64 /usr/local/bin/tvcast
sudo useradd --system --home-dir /var/lib/tvcast --shell /usr/sbin/nologin tvcast
sudo cp /tmp/tvcast.service /etc/systemd/system/tvcast.service

# Nombre y contraseña de la wifi que creará la caja (8 caracteres o más).
# Se usan para el QR de la tele.
sudo tee /etc/tvcast.env >/dev/null <<'EOF'
TVCAST_WIFI_SSID=Tele
TVCAST_WIFI_PASS=pon-aqui-una-contraseña
# Tele en español (por defecto sale en inglés). La web del móvil ya sale
# en el idioma del móvil.
TVCAST_LANG=es
EOF
sudo chmod 600 /etc/tvcast.env

sudo systemctl daemon-reload
sudo systemctl enable --now tvcast
journalctl -u tvcast -f        # Ctrl+C para salir
```

En la tele debe salir la pantalla con dos QR. Ahora mismo el segundo QR lleva
la IP de tu casa (IP_DE_LA_CAJA): escanéalo con el móvil en tu wifi y manda una
foto para probar.

**Si la tele se queda en negro o con texto** y el log dice algo de `gpu` o
`EGL`: la GPU no va con mpv. Cambia en `/etc/systemd/system/tvcast.service`
`--vo=gpu --gpu-context=drm` por `--vo=drm` y:

```sh
sudo systemctl daemon-reload && sudo systemctl restart tvcast
```

## 5. Crear la wifi "Tele"

> Con la Sunday Box (BCM4334) este paso **no funcionó** con contraseña: el
> punto de acceso de NetworkManager con WPA2 no deja entrar a los móviles.
> En la caja se usa hostapd (ver docs/LESSONS.md y TODO.md, pendiente de
> traerlo al repo). Este script vale para chips en los que NM sí funciona.

Usa **la misma contraseña** que en `/etc/tvcast.env`:

```sh
sudo sh /tmp/hotspot.sh Tele 'pon-aqui-una-contraseña'
```

Esto crea la red y deja la wifi de tu casa con más prioridad. Así, en tu casa
se conecta a tu wifi y en la otra casa, al no encontrarla, crea "Tele".

## 6. Probarlo en tu casa antes de llevarla

Simula la otra casa (perderás el SSH; si algo va mal, usa el teclado):

```sh
sudo nmcli connection up tvcast-ap
```

1. En unos 30 s la tele cambia el QR a `http://10.42.0.1`.
2. Con el móvil: escanea el primer QR (unirse a "Tele"). Si Android pregunta
   por "sin internet", di que mantenga la conexión.
3. Escanea el segundo QR y manda fotos.
4. Para entrar por SSH: conecta el Mac a "Tele" y `ssh usuario@10.42.0.1`.

Prueba de arranque como en la otra casa (sin tu wifi):

```sh
nmcli -t -f NAME,TYPE connection show    # anota el nombre de tu wifi
sudo nmcli connection modify "TU_WIFI" connection.autoconnect no
sudo reboot
```

Tras reiniciar debe salir sola la red "Tele" y la tele con los QR. Para
volver a tu wifi (con el Mac conectado a "Tele", por SSH a 10.42.0.1):

```sh
sudo nmcli connection modify "TU_WIFI" connection.autoconnect yes
sudo nmcli connection down tvcast-ap     # se conecta sola a tu wifi
```

## 6b. Atajos de teclado (opcional)

Con un teclado USB o el receptor 2,4 GHz enchufado a la caja:

| Atajo | Qué hace |
|---|---|
| **Ctrl+Alt+Q** | Pone el QR a pantalla completa y lo deja fijo, aunque haya fotos o lleguen nuevas (se guardan igual) |
| **Ctrl+Alt+S** | Vuelve al pase de fotos |
| **Ctrl+Alt+C** | Para tvcast y enseña el login local (ver `deploy/splash`) |
| **Ctrl+Alt+T** | Arranca o reinicia tvcast |

```sh
sudo apt install -y triggerhappy        # si no lo instalaste en el paso 1
sudo cp /tmp/tvcast.conf /etc/triggerhappy/triggers.d/   # copiado desde deploy/triggerhappy/
sudo systemctl restart triggerhappy
```

Sin teclado, lo mismo por SSH: `curl -X POST 'http://127.0.0.1/api/tv/qr?v=1'`
(y `v=0` para volver). Solo funciona desde la propia caja.

## 6c. Mando original Sunday (opcional)

El mando de la caja es Bluetooth LE, no infrarrojo, y Linux no lo ve como
teclado: `tvcast-remote` (un servicio aparte) lo escucha por BlueZ y llama a
la API local de tvcast.

| Botón | Qué hace |
|---|---|
| **Derecha / Izquierda** | Foto siguiente / anterior |
| **OK** o **Play/Pausa** | Pausa o reanuda el pase |
| **Home** | Pone el QR a pantalla completa y lo deja fijo; otra vez para volver |
| **Atrás** | Vuelve al pase (quita el QR fijo y reanuda) |
| Arriba, Abajo, Corazón, Vol +/−, Mic, Encendido | Nada de momento (salen en el registro) |

El mando nunca abre la consola ni reinicia tvcast: eso sigue en los atajos de
teclado (6b), para que nadie de la familia deje la tele en un login.

**Usa un adaptador Bluetooth USB, no el chip interno.** El interno (BCM4334,
comparte radio con la wifi) colgó la caja entera dos veces. Probado con un
adaptador CSR (id USB `0a12:0001`). Antes de encender el Bluetooth:

```sh
# Qué adaptadores hay y cómo está conectado cada uno (usb o serie/uart)
ls -l /sys/class/bluetooth/
# Si aparece el interno (no cuelga de usb), mantenlo apagado. Comprueba antes
# que hci_uart es un módulo (lsmod | grep hci_uart); se deshace borrando el fichero.
echo "blacklist hci_uart" | sudo tee /etc/modprobe.d/tvcast-no-internal-bt.conf
sudo reboot
```

Tras reiniciar, `bluetoothctl list` debe mostrar solo el adaptador USB. Luego
empareja el mando una vez (sin PIN; si no aparece, quítale las pilas 10 s):

```sh
sudo systemctl enable --now bluetooth
bluetoothctl            # y dentro: scan on, espera a "Sunday",
                        #           pair <dirección>, trust <dirección>, scan off, quit
```

Instala el servicio, primero en modo prueba para ver llegar las teclas sin
hacer nada:

```sh
sudo install -m755 /tmp/tvcast-remote-linux-arm64 /usr/local/bin/tvcast-remote
sudo /usr/local/bin/tvcast-remote -dry-run   # pulsa botones, Ctrl+C para salir
sudo cp /tmp/tvcast-remote.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now tvcast-remote
journalctl -u tvcast-remote -f                # una línea por tecla
```

Se reconecta solo con el emparejamiento guardado (tras reiniciar, si el mando
se duerme o se aleja). No hace falta poner nombre ni dirección en ninguna
configuración: usa el dispositivo emparejado que se llama "Sunday". Para
apagarlo sin tocar las fotos: `sudo systemctl disable --now tvcast-remote`.

## 7. En la otra casa

1. Conecta HDMI y corriente. Enciende la tele **antes** o a la vez que la caja.
2. Al minuto aproximado salen los dos QR.
3. Para la familia: "escanea el primero para conectarte a la wifi de la tele;
   luego el segundo y elige fotos". En el móvil que se una sale "sin internet":
   es normal.
4. Con fotos ya enviadas, el QR sigue a mano: hay uno pequeño con un código de
   6 cifras abajo a la derecha, y cada 12 fotos sale un momento la pantalla
   completa de QR. Quien abra `http://10.42.0.1` sin QR puede teclear ese
   código.

Para mantenimiento allí: conecta el Mac a "Tele" y `ssh usuario@10.42.0.1`.
Para actualizar tvcast: copia el binario nuevo igual que en el paso 4 (a
10.42.0.1) y `sudo systemctl restart tvcast`.

## Problemas típicos

| Síntoma | Qué mirar |
|---|---|
| La tele no muestra nada | ¿Se encendió la tele después de la caja? `sudo systemctl restart tvcast`. Log: `journalctl -u tvcast -n 50` |
| No aparece la red "Tele" | `nmcli connection show --active`, `journalctl -u NetworkManager -n 50`. ¿Paso 3 dio `* AP`? |
| El móvil está en "Tele" pero la web no carga | Android puede mandar el tráfico por datos móviles: desactiva los datos un momento o acepta "mantener conexión" |
| "HEIC no soportado" | Envía desde el navegador del móvil (Safari la convierte) o en el iPhone: Ajustes → Cámara → Formatos → Más compatible |
| Un vídeo va a saltos | Esperado con vídeos 1080p HEVC o 4K (ver docs/DESIGN.md §5) |
| Vídeos sin sonido | `mpv --audio-device=help` para ver la salida HDMI y añádela en `-mpv-args` (`--audio-device=alsa/...`) |
| Hora mal en la caja | Normal sin internet; no afecta a tvcast |
| El mando no hace nada | `journalctl -u tvcast-remote -n 30`: "no paired device" es que hay que emparejarlo otra vez (6c); si salen líneas "retrying" mientras pulsas, no logra conectar: pilas, o quítalas 10 s |
| La tele sale en inglés | Falta `TVCAST_LANG=es` en `/etc/tvcast.env` (y `sudo systemctl restart tvcast`) |
