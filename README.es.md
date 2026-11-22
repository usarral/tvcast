# tvcast

*[Read in English](README.md). La versión en inglés es la de referencia; esta
se mantiene para quien instala la caja en español.*

Envía fotos (y vídeos) desde el móvil a la tele. Pensado para una TV box
reciclada (Sunday Box, RK3318) con Armbian: la caja sirve una web, la tele
muestra un QR y cualquiera en el salón puede mandar fotos que salen a
pantalla completa.

- Un binario Go sin dependencias (+ `mpv` en la caja).
- Las fotos se reducen a 1080p en el propio móvil antes de subir.
- Pase de diapositivas automático, borrar, anterior/siguiente/pausa.
- Acceso con el QR de la tele o con el código de 6 cifras que muestra.
- Funciona sin internet: la caja crea su propia wifi.
- En inglés y en español (pantalla de la tele, web del móvil, imagen de arranque).

No he encontrado documentación previa de reutilizar la Sunday Box así, pero
eso no quiere decir que nadie lo haya hecho, y la idea de mandar fotos a la
tele no es nueva. Ver [docs/LESSONS.md](docs/LESSONS.md#about-nobody-has-done-it) (en inglés).

Instalación en español: [docs/es/INSTALL.md](docs/es/INSTALL.md).
En inglés: diseño ([docs/DESIGN.md](docs/DESIGN.md)), lo aprendido en el
hardware real ([docs/LESSONS.md](docs/LESSONS.md)), pendiente
([TODO.md](TODO.md)) y comprobaciones extra ([docs/CHECKS.md](docs/CHECKS.md)).

## Probar en local (Mac o Linux)

Requiere Go 1.23+.

```sh
make test
make run            # visor web: abre http://localhost:8080/tv y, en otra pestaña, http://localhost:8080
```

Con mpv instalado (`brew install mpv`) puedes probar el visor real:
`./bin/tvcast -data ./data` (abre una ventana de mpv a pantalla completa;
pulsa `q` en ella y tvcast la volverá a abrir; Ctrl+C para salir).

Desde el móvil, en la misma wifi, escanea el QR o abre la URL con `?k=` que
sale en el log.

## Instalar en la caja

Paso a paso, desde Armbian recién instalado: [docs/es/INSTALL.md](docs/es/INSTALL.md).

## Idioma

- **Pantalla de la tele** (QR y QR de esquina): `-lang en|es` o `TVCAST_LANG`
  (por defecto `en`). **En una caja para una casa en español, pon
  `TVCAST_LANG=es` en `/etc/tvcast.env`**; si no, la tele saldrá en inglés.
- **Web del móvil**: sale en el idioma del móvil si es español o inglés; si
  no, en el de `-lang`.
- **Imagen de arranque**: `go run ./cmd/splash -lang es ...` (ver `deploy/splash`).

## Opciones principales

| Flag | Por defecto | |
|---|---|---|
| `-addr` | `:8080` | puerto HTTP |
| `-data` | `./data` | fotos, índice, token y pantalla de espera |
| `-display` | `mpv` | `mpv` o `web` (solo visor `/tv`) |
| `-mpv-args` | | extra para mpv, p. ej. `--vo=drm` |
| `-interval` | `10s` | tiempo por foto |
| `-max-file-mb` / `-max-total-gb` / `-max-items` | 300 / 4 / 500 | límites |
| `-iface` | | interfaz para la IP del QR (`wlan0`) |
| `-wifi-ssid` (o `TVCAST_WIFI_SSID`) + `TVCAST_WIFI_PASS` | | añade a la tele el QR para unirse a la wifi |
| `-lang` (o `TVCAST_LANG`) | `en` | idioma de la tele y por defecto de la web: `en`, `es` |
| `-corner-qr` | `true` | QR pequeño con el código en una esquina durante el pase |
| `-qr-every` | `12` | intercala la pantalla de QR cada N fotos (0 = nunca) |
| `-no-auth` | `false` | sin token (solo desarrollo) |

## API

Todas piden la cookie del token salvo desde 127.0.0.1. La cookie se consigue
abriendo la URL del QR (`/?k=<token>`) o con el código de 6 cifras de la tele
(`POST /api/login`, `pin=...`; 10 fallos en 10 min bloquean el login).

| Método | Ruta | |
|---|---|---|
| GET | `/api/state` | estado y lista |
| GET | `/api/events` | lo mismo por SSE en cada cambio |
| POST | `/api/upload` | multipart: `thumb` (opcional, JPEG) y luego `file` |
| POST | `/api/show/{id}` · `/api/next` · `/api/prev` | qué se ve |
| POST | `/api/pause?v=1\|0` | pausar o seguir el pase |
| DELETE | `/api/items/{id}` | borrar |
| POST | `/api/tv/qr?v=1\|0\|toggle` | solo desde la caja: fijar o soltar la pantalla de QR (atajos Ctrl+Alt+Q / Ctrl+Alt+S) |

Ejemplo para un mando o script en la propia caja:
`curl -X POST localhost:8080/api/next`.
