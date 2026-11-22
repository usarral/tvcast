# Lo aprendido con la Sunday Box (noviembre 2026)

Notas de lo que funcionó y lo que no en el hardware real, para no repetirlo.

## Hardware

- Rockchip RK3318, placa `RK3328_8D4_V1.1`, 1,9 GB de RAM útiles.
- Wifi: módulo Murata (marcado HK6334Q), driver `brcmfmac` (familia BCM4334).
  `iw list` muestra `* AP`: **sí puede crear una red**.
- Sin Ethernet. El conector pequeño es salida óptica de audio.
- Armbian trixie (imagen Rk3318-box), kernel 6.18.

## Punto de acceso

- **NetworkManager en modo AP con WPA2 no funcionó** con este chip: la red
  aparecía pero los móviles no completaban la contraseña (problema de
  PMF/PSK con `brcmfmac`). Sin contraseña sí conectaba.
- **hostapd sí funciona** con WPA2-PSK (CCMP): un iPhone conectó y recibió IP
  por DHCP. Por eso hostapd es el camino (ver TODO.md: falta traer al repo la
  configuración exacta de la caja).
- Arranque sin la wifi de casa: hay que esperar unos 40 s a que NetworkManager
  encuentre la wifi conocida; si no aparece, levantar "Tele". La primera
  versión no la levantaba sola en el arranque.

## tvcast

- Con fotos ya subidas la tele solo pasaba fotos y el QR no se veía nunca,
  pero la web pedía escanearlo. Arreglado: QR pequeño con código de 6 cifras
  en la esquina, pantalla de QR cada 12 fotos y login con el código.
- Al arrancar antes que la red, el QR salía con 127.0.0.1 hasta un minuto.
  Ahora se comprueba la IP cada 3 s.
- Sin internet no hay NTP ni pila de reloj: no depender de la hora.
- La tele enseñaba la consola unos segundos en cada arranque en frío. No era
  la red: tvcast esperaba solo 5 s a que mpv creara su socket IPC y, con la
  CPU a tope al arrancar, mpv tarda más en abrir DRM. tvcast lo mataba (se ve
  la consola) y lo relanzaba a los 2 s. El journal lo delata con
  `mpv: sin socket IPC` justo tras el arranque. Ahora espera hasta 30 s
  mientras el proceso mpv siga vivo.
- Ctrl+Alt+C para tvcast a propósito (consola). Si "la tele se reinicia" tras
  tocar el teclado, mirar `journalctl -t thd` antes de buscar otra causa.
- Sin pila de reloj, `journalctl --list-boots` y las horas del log mienten
  hasta que llega NTP: usar `-o short-monotonic` para ordenar el arranque.

## Arranque sin consola

- Plymouth no compensa en esta caja: hay que meterlo en el initramfs y al
  terminar deja un hueco (negro o el login) hasta que mpv pinta, que en frío
  tarda unos 8 s. Más simple: `console=serial` en `armbianEnv.txt` (el kernel
  y systemd escriben solo al puerto serie), `vt.global_cursor_default=0
  logo.nologo`, y una imagen en crudo copiada a `/dev/fb0` en cuanto arranca
  systemd. Detalles en `deploy/splash/README.md`.
- El framebuffer de consola va a 2560x1440 (modo preferido de la tele), no a
  1920x1080 como mpv: la imagen tiene que medir lo que diga
  `/sys/class/graphics/fb0/virtual_size`, en BGRX.
- El login en tty1 pintaría encima de la imagen: se mueve a tty2 y Ctrl+Alt+C
  hace `chvt 2`. Lo de U-Boot (primeros segundos) no se puede tapar sin
  recompilarlo.

## Mando oficial (en curso)

- **No es infrarrojo.** La placa tiene receptor IR (`gpio_ir_recv`, rc0,
  `/dev/lirc0`) con la tabla vacía; con todos los protocolos activados y
  `ir-keytable -t` no llega nada al pulsar el mando.
- **Es Bluetooth LE**: se anuncia siempre como `Sunday` (en la caja de cartón:
  "advertising always on", LED UUID 0xFED0). No hace falta combinación para
  emparejar: `bluetoothctl pair/trust/connect <MAC>` con el mando cerca.
- No es un HID estándar (no tiene el servicio 0x1812), así que Linux no crea
  ningún `/dev/input`. Del backup de Android (app `Sunrise.apk`, paquete
  `love.sunday.sunrise.remotecontrol`, decompilada con jadx) salen los UUID:
  botones en el servicio propio `0xFF00`, característica `0x2a4d` (1 byte, o
  informe de 8 bytes con la tecla en el byte 2); voz en
  `f000b000-0451-4000-b000-000000000000` (`b001` inicio/fin, `b002` audio);
  LED en `0xFED0` (`fed1` color, `fed2` periodo, `fed3` efecto); batería
  `0x180F`. Nombres aceptados por la app: `Sunday` y `4MOD 45 BLE`.
- Mantener Mic enciende la luz del mando, igual que con Android (lo hace el
  propio mando; no se la hemos encendido nosotros).
- Probado desde el Mac con `tools/mando/sunday_remote.py` (Python + bleak):
  conecta sin emparejar, lee batería (`2a19`, %) y firmware (`v1.19`,
  fabricante "4MOD TECHNOLOGY", modelo `4MOD3401`), y deja suscribirse a todo,
  pero **no llega ninguna tecla** y escribir en la luz (`fed1..3`) no hace
  nada visible. El servicio `0xFF00` es un HID por GATT camuflado (información
  HID `11 01 00 01`, modo de protocolo, punto de control, tres informes
  `2a4d` con descriptor `2908`), pero su "report map" (`2a4b`) devuelve
  basura (memoria del firmware) y leer los informes da "Invalid Offset".
  Hipótesis: el mando solo envía teclas por un enlace cifrado (emparejado),
  y Core Bluetooth en macOS no deja forzar el emparejamiento desde un script
  ("Pairing is not available in Core Bluetooth"). Con BlueZ sí se empareja
  (`Paired/Bonded: yes`), así que la prueba buena es en Linux, con `btmon`.
- Si un mando BLE deja de anunciarse tras cortes raros: quitar las pilas
  10 s y pulsar un botón mientras se busca.
- Para leer el backup sin tocarlo: la tabla GPT dice dónde está cada
  partición; se recortan `vendor`/`system` de `gunzip -c` y se leen con
  `debugfs` (e2fsprogs) en el Mac, sin montar.
- **Cuidado: la caja se colgó entera dos veces con el Bluetooth del chip
  interno (BCM4334, wifi y Bluetooth en el mismo chip) activo**: una copiando
  192 MB por wifi y otra con el mando conectado. El journal se cortaba de
  golpe. Por eso `bluetooth.service` queda desactivado al arrancar,
  `journald` sincroniza cada 2 s y `kernel.panic=10` reinicia si el kernel
  falla. En un tercer intento conectado 2,5 min no se colgó, pero tampoco
  llegó ninguna notificación de botones: falta mirarlo con `btmon`. Si el
  chip interno no es fiable, plan B: adaptador Bluetooth USB.

## Sobre "nadie lo ha hecho"

No he encontrado documentación previa de reutilizar la Sunday Box con
Armbian. Eso no quiere decir que nadie lo haya hecho. Y la idea de mandar
fotos del móvil a la tele no es nueva: hay marcos de fotos caseros
(TeleFrame por Telegram, photoframe con Google Fotos, Pimmich con Immich,
picframe…), cartelería digital (Obscreen) y envío de ficheros
entre dispositivos (PairDrop). Lo que aporta tvcast es una combinación concreta:
funciona **sin internet** (la caja crea su propia wifi), **sin instalar ninguna
app** (el navegador del móvil y un QR) y en una **TV box reciclada de 4 €**
con un solo binario de Go y mpv.
