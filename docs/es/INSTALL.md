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

## 6b. Panel de administración y teclado

Todo lo que puedas necesitar en la caja está en un panel oculto en la tele:

- **Teclado** (USB o receptor 2,4 GHz): **Ctrl+Alt+A**.
- **Mando** (6c): **Corazón, Arriba, Corazón, Abajo** en menos de 5 segundos.

Te mueves con las flechas, **OK** (Intro) elige, **Atrás** (Esc) vuelve,
**Home** cierra. Se cierra solo tras 2 minutos sin pulsar nada. Mientras está
abierto, el pase se queda en pausa.

| Opción | Qué hace |
|---|---|
| Estado (derecha) | IP y red (wifi de casa o "Tele"), fotos, espacio libre, tiempo encendida, mando y su batería, versión |
| Mostrar / quitar el QR | Fija el QR a pantalla completa (lo verás al cerrar el panel) |
| Pausar / seguir el pase | Lo que hará el pase al cerrar el panel |
| Red | Wifi de casa o "Tele" (usa los comandos `casa` / `tele` de la caja, con su reversión automática), registro de la wifi |
| Registros | tvcast, wifi, mando y Bluetooth, acciones de administración, kernel |
| Fotos | Borrar todas las fotos |
| Código y QR nuevos | Todos los móviles tendrán que escanear otra vez |
| Emparejar el mando otra vez | Olvida el emparejamiento viejo y empareja el mando que esté cerca |
| Idioma | Español / English (pantalla de la tele e imagen de arranque) |
| Sistema | Reiniciar tvcast, reiniciar, apagar, abrir la consola (login en tty2) |

Todo lo que cambia la caja pide confirmación (No / Sí, con No por defecto).
Con el panel cerrado, las flechas del teclado cambian de foto, Intro pausa y
Esc vuelve al pase, igual que el mando.

### Instalación

tvcast va sin privilegios. Lo que necesita root pasa por systemd: tvcast
arranca `tvcast-admin@<acción>.service` y una regla de polkit deja al usuario
`tvcast` arrancar esas unidades y nada más (sin sudo).

```sh
sudo apt install -y triggerhappy polkitd   # si faltan (necesita internet: hazlo en casa)
# desde deploy/admin, deploy/systemd y deploy/triggerhappy, copiados a /tmp:
sudo install -m755 /tmp/tvcast-admin /usr/local/sbin/
sudo install -m644 /tmp/tvcast-admin@.service /etc/systemd/system/
sudo install -m644 /tmp/50-tvcast-admin.rules /etc/polkit-1/rules.d/
sudo install -m644 /tmp/tvcast.service /etc/systemd/system/   # ahora lee el registro (para el panel)
sudo cp /tmp/tvcast.conf /etc/triggerhappy/triggers.d/
sudo systemctl daemon-reload && sudo systemctl restart tvcast triggerhappy
# Comprueba la regla: esto tiene que funcionar como usuario tvcast (y stop no).
sudo -u tvcast systemctl start tvcast-admin@restart-tvcast.service
```

Las opciones de red llaman a los comandos `casa` y `tele` de la caja (aún no
están en el repo): si no existen, el panel lo dice en el registro.

Los atajos antiguos (Ctrl+Alt+Q/S/C/T) y `curl -X POST
'http://127.0.0.1/api/tv/qr?v=1'` siguen funcionando.

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
| **Corazón, Arriba, Corazón, Abajo** | Abre el panel de administración (6b); dentro, flechas, OK, Atrás y Home manejan el menú |
| Vol +/−, Mic, Encendido | Nada de momento (salen en el registro) |

Encendido no hace nada a propósito, y la consola y los reinicios solo están en
el panel, detrás de la secuencia y de una confirmación, para que nadie de la
familia deje la tele en un login sin querer.

**El Bluetooth interno es un riesgo.** Esta caja no lleva adaptador USB, así
que el mando usa el chip interno (BCM4334: el mismo chip, radio y antena que
la wifi). Con él encendido la caja se colgó dos veces: una copiando 192 MB por
wifi y otra con el mando conectado. El peor caso es el uso real: la caja como
"Tele", un móvil subiendo fotos y el mando conectado. Por eso los pasos de
abajo primero hacen que un cuelgue se arregle solo y luego prueban por fases
en casa. Si se cuelga en las pruebas: un adaptador USB (unos 5 €; el CSR
`0a12:0001` está probado) o sin mando.

### 1. Actualiza antes tvcast

Desde el repo, en el ordenador (antes `make arm64`):

```sh
scp bin/tvcast-linux-arm64 bin/tvcast-remote-linux-arm64 deploy/systemd/tvcast-remote.service \
    deploy/bluetooth/tvcast-bt-guard deploy/bluetooth/tvcast-bt-guard.service \
    deploy/bluetooth/watchdog.conf deploy/network/tvcast-netwd \
    deploy/network/tvcast-netwd.service deploy/network/tvcast-netwd.timer tvbox:/tmp/
```

El binario actual sale en inglés por defecto: antes de instalarlo en una caja
que estaba en español, añade `TVCAST_LANG=es` a `/etc/tvcast.env`. Luego copia
`tvcast-linux-arm64` como en el paso 4 y `sudo systemctl restart tvcast`.

### 2. Red de seguridad (antes de encender el Bluetooth)

```sh
# Watchdog de hardware: un kernel congelado no entra en pánico, así que
# kernel.panic=10 solo no basta; con el watchdog la caja se reinicia en ~20 s.
ls /dev/watchdog*                      # tiene que existir; si no, para aquí
sudo mkdir -p /etc/systemd/system.conf.d
sudo cp /tmp/watchdog.conf /etc/systemd/system.conf.d/tvcast-watchdog.conf
# Guardián: tras 2 arranques seguidos sin apagado limpio, apaga el Bluetooth solo.
sudo install -m755 /tmp/tvcast-bt-guard /usr/local/sbin/
sudo cp /tmp/tvcast-bt-guard.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable tvcast-bt-guard
sudo touch /etc/tvcast-bt-guard.enable
sudo reboot
# Tras reiniciar:
systemctl show -p RuntimeWatchdogUSec  # 20s, no 0
ls /var/log/journal                     # registro persistente (para leer un cuelgue después)
```

Los ficheros están en `deploy/bluetooth/`. Para deshacer: bórralos y reinicia.

### Watchdog de red

Con el Bluetooth interno encendido y fotos subiendo, la caja perdió la wifi
con Linux todavía vivo. El watchdog de hardware solo salta si el kernel se
congela, así que no hizo nada y la caja se quedó inaccesible. `tvcast-netwd`
comprueba la wifi cada 30 s (modo AP: interfaz en modo AP y con IP; conectada
a una wifi: IP y el router responde al ping) y reinicia tras 2 minutos sin
ella. Espera 5 minutos tras arrancar y se rinde tras 2 reinicios seguidos sin
recuperar la red (no tiene sentido reiniciar en bucle si el router está apagado).

```sh
# desde deploy/network, copiados a /tmp:
sudo install -m755 /tmp/tvcast-netwd /usr/local/sbin/
sudo cp /tmp/tvcast-netwd.service /tmp/tvcast-netwd.timer /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now tvcast-netwd.timer
journalctl -u tvcast-netwd -f        # en silencio mientras la red va bien
```

Para apagarlo: `sudo systemctl disable --now tvcast-netwd.timer`, o
`TVCAST_NETWD_DISABLE=1` en `/etc/tvcast.env`.

### 3. Mira el chip antes de usarlo

```sh
sudo dmesg | grep -iE 'bluetooth|hci|bcm'
```

- Una línea tipo `BCM: Patch brcm/BCM4334B0.hcd` (o parecida) quiere decir
  que cargó el parche de firmware del Bluetooth. Si dice que **no encuentra**
  el `.hcd`, el chip va con su firmware de ROM antiguo, una causa probable de
  cuelgues: busca el `.hcd` en el backup de Android (`/vendor/firmware`,
  `/system/etc/firmware`) y cópialo a `/lib/firmware/brcm/` con el nombre que
  pide el registro.
- `grep -i btc /lib/firmware/brcm/brcmfmac4334*.txt`: los ajustes de
  convivencia wifi/Bluetooth (`btc_mode`...). Si no hay y el nvram de Android
  del backup los tiene, prueba esos. Un cambio cada vez.

### 4. Empareja el mando

Solo habla con un equipo a la vez: primero quítalo del ordenador con el que
está emparejado (`bluetoothctl remove <dirección>` allí, o apaga su Bluetooth).

Emparejar necesita un agente **en la misma sesión de bluetoothctl**: un
`bluetoothctl pair <dirección>` suelto falla con `Authentication Failed (0x05)`.

```sh
sudo systemctl enable --now bluetooth
bluetoothctl            # y dentro, todo en esta misma sesión:
#   agent NoInputNoOutput
#   default-agent
#   scan on             (espera a "Sunday")
#   pair <dirección>
#   trust <dirección>
#   scan off
#   quit
```

Sin PIN. Si no aparece, quítale las pilas 10 s. Más adelante, "Emparejar el
mando otra vez" del panel de administración hace lo mismo.

Si `bluetoothctl list` no muestra ningún controlador, el chip interno no ha
arrancado (`dmesg`: `Failed to set baudrate`, error -110). Recargar el driver
lo arregla: `sudo modprobe -r hci_uart btbcm && sudo modprobe hci_uart`.

### 5. Instala tvcast-remote

```sh
sudo install -m755 /tmp/tvcast-remote-linux-arm64 /usr/local/bin/tvcast-remote
sudo /usr/local/bin/tvcast-remote -dry-run   # pulsa botones, Ctrl+C para salir
sudo cp /tmp/tvcast-remote.service /etc/systemd/system/
sudo systemctl daemon-reload && sudo systemctl enable --now tvcast-remote
journalctl -u tvcast-remote -f                # una línea por tecla
```

Se reconecta solo con el emparejamiento guardado (tras reiniciar, si el mando
se duerme o se aleja) y no necesita nombre ni dirección en ninguna
configuración. Tras reconectar, unas diez líneas "mic (not mapped)" son normales.

### 6. Prueba por fases, en casa, con el guardián puesto

1. Una hora en reposo con el mando conectado, pulsando teclas de vez en cuando.
2. En la wifi de casa: sube 30–50 fotos desde un móvil mientras pulsas
   teclas. Tres veces.
3. En modo "Tele", la misma subida desde un móvil conectado a "Tele".
4. Una noche encendida.

Tras cada fase: `uptime` (sin reinicios por sorpresa) y
`journalctl -k -b | grep -iE 'brcmf|bluetooth|hci'`.

- **Todo bien**: antes de llevarla a la otra casa, `sudo rm
  /etc/tvcast-bt-guard.enable` (allí puede que la desenchufen cada noche y
  cada corte contaría como cuelgue). El watchdog se queda.
- **Se colgó**: el watchdog la reinicia; tras dos seguidos el guardián apaga
  el Bluetooth y la caja sigue como antes, sin mando. Qué pasó justo antes:
  `journalctl -b -1 -k -n 50`. Siguiente paso: adaptador USB.
- Para apagar el mando a mano: `sudo systemctl disable --now tvcast-remote bluetooth`.

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
