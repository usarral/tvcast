# Pendiente

## Mañana (23 nov 2026)

- [x] **Pantalla de arranque.** Hecha sin Plymouth: consola del kernel solo
  por serie, cursor oculto, imagen en `/dev/fb0` al arrancar systemd y login
  en tty2 (ver `deploy/splash/README.md`). Probada en la caja.
- [x] **Consola unos segundos al arrancar.** Era tvcast matando mpv a los 5 s;
  ahora espera hasta 30 s. Probado con reinicio.
- [ ] **Mando oficial (Bluetooth LE, no IR).** Emparejado y conectado, pero
  aún no llega ninguna tecla. Siguiente: repetir la captura con `btmon` para
  ver si las notificaciones del servicio `0xFF00` llegan y por qué se cae la
  conexión. Ojo: la caja se colgó dos veces con el Bluetooth del BCM4334
  activo; `bluetooth.service` está desactivado al arrancar hasta entenderlo.
  Si el chip interno no es fiable: adaptador Bluetooth USB solo para el mando.
  Después: servicio que escuche el mando y llame a la API de tvcast
  (flechas → anterior/siguiente, OK → pausa, Home → QR). Ver docs/LESSONS.md.
  Desde el Mac (`tools/mando/sunday_remote.py`) conecta pero sin teclas: falta
  probar con enlace emparejado en Linux.
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
