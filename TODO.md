# Pendiente

## Mañana (23 nov 2026)

- [ ] **Animación de arranque.** Que la tele no enseñe texto de consola al
  encender. Opciones: Plymouth con un tema propio (tapa desde el kernel) o,
  más simple, que tvcast/mpv muestre una imagen en cuanto haya DRM y
  `quiet loglevel=3 vt.global_cursor_default=0` en `/boot/armbianEnv.txt`
  (extraargs). Plymouth en Armbian con DRM mainline: comprobar que el kernel
  tiene `simpledrm`/`rockchipdrm` pronto o la animación sale tarde.
- [ ] **Mando oficial.** Averiguar si es IR, RF 2,4 GHz o Bluetooth
  (`ir-keytable`, `evtest`, `lsusb`, `bluetoothctl`; ver docs/CHECKS.md §4),
  mapear teclas (rc_keymaps o hwdb) y llamar a la API con triggerhappy:
  flechas → `/api/prev` `/api/next`, OK → pausa, Home → `/api/tv/qr?v=toggle`.
  Probar también HDMI-CEC (mando de la tele).
- [ ] **hostapd como modo por defecto.** Llevar al repo la configuración que
  funciona en la caja (hostapd.conf, servicio de arranque con espera a la
  wifi de casa y vuelta atrás) y sustituir `deploy/network/hotspot.sh`
  (NetworkManager en modo AP + WPA2 no funcionó con este BCM4334: ver
  docs/LESSONS.md).
- [ ] **QR con 127.0.0.1 al arrancar.** Corregido en código (tvcast mira la IP
  cada 3 s y repinta el QR; antes cada 30 s y en la caja se resolvía
  reiniciando el servicio). Desplegar y comprobar en un arranque en frío; si va
  bien, quitar el reinicio de tvcast que hace el script de "Tele".

## Más adelante

- [ ] Workflow de GitHub Actions: `go vet`, `go test` y build arm64 en cada push;
      binario en Releases.
- [ ] Decidir licencia si se publica (MIT o GPL-3.0).
- [ ] Vídeo: medir con vídeos reales de los móviles de la familia.
- [ ] Recomprimir vídeo en el móvil (WebCodecs) si el vídeo compensa.
