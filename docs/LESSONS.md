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
