# tvcast: diseño

Sustituto casero del software de la Sunday Box: desde el móvil se eligen
fotos (o vídeos) en una web servida por la propia caja y salen a pantalla
completa en la tele.

Caja: Rockchip RK3318 (familia RK3328, 4× Cortex-A53, GPU Mali-450), 1-2 GB
de RAM, solo wifi (módulo Murata, marcado HK6334Q), Armbian sin entorno
gráfico, HDMI a la tele. **La casa donde va no tiene wifi ni internet**: la
caja crea su propia wifi ("Tele", 10.42.0.1).

## 1. Recomendación en una frase

**Un binario Go en la caja que sirve la web y controla `mpv` por su socket
IPC, sin entorno gráfico.** La caja crea su propia wifi con NetworkManager y
la tele muestra dos QR: uno para unirse a la wifi y otro para abrir la web.
Vídeo se acepta, pero solo "si cabe": no prometo que un vídeo de móvil actual
se vea fluido.

## 2. Arquitectura

```
 Móvil (navegador)                     Caja (Armbian)
 ┌──────────────────┐   HTTP :8080   ┌───────────────────────────────┐
 │ página web       │ ─────────────▶ │ tvcast (Go, ~7 MB, 1 proceso) │
 │ - elige fotos    │  subida        │  ├─ store: ficheros + índice   │
 │ - reduce a 1080p │  multipart     │  ├─ player: cola y pase        │
 │ - mando ◀ ⏸ ▶    │ ◀───────────── │  ├─ web: API + SSE + páginas   │
 │ - borrar         │  SSE (estado)  │  └─ idle: PNG con QR           │
 └──────────────────┘                │        │ JSON IPC (socket unix)│
                                     │        ▼                       │
                                     │  mpv a pantalla completa ── HDMI ──▶ Tele
                                     └───────────────────────────────┘
```

- **Servidor (Go)**: un binario estático para arm64, sin dependencias en la
  caja salvo mpv. Se compila en el Mac con `make arm64`. Gasta pocos MB.
- **Visor (mpv)**: un solo proceso muestra fotos y vídeos, se controla por un
  socket (`loadfile ... replace`) y avisa cuando acaba un vídeo (`end-file`).
  Si mpv muere, tvcast lo relanza y vuelve a poner lo que había.
- **Visor web `/tv`** (alternativo): la misma pantalla en HTML, para probar en
  local o si mpv diera guerra. En la caja sería Chromium en modo kiosco.
- **Pantalla de espera**: PNG 1920×1080 que genera tvcast con el QR de la web
  (y el QR de la wifi si la caja hace de punto de acceso). Se regenera si
  cambia la IP.

### Flujo de uso

1. La tele muestra la pantalla de espera con el QR.
2. El móvil lo escanea: `http://10.42.0.1/?k=<token>`. La web guarda el
   token en una cookie y quita el `?k=` de la barra.
3. "Elegir fotos o vídeos" abre la galería. Cada foto **se reduce en el móvil**
   a 1080p (aplicando la rotación EXIF) y se sube con una miniatura de 320 px.
   En iPhone, Safari convierte de paso HEIC a JPEG.
4. Lo último que llega sale al momento en la tele. Si nadie envía nada, rota
   en pase de diapositivas (10 s por foto; los vídeos hasta que acaban).
5. La web muestra la lista en vivo (SSE) con la que se está viendo marcada.
   Tocar una la pone; ✕ la borra; hay ◀ ⏸ ▶.
6. Sin fotos, vuelve la pantalla de espera.
7. Con fotos, el QR no desaparece: hay uno pequeño en la esquina con un código
   de 6 cifras, y cada 12 fotos se intercala la pantalla de QR completa
   durante un intervalo. Quien llegue a la web sin QR puede teclear el código.
   Con teclado en la caja, Ctrl+Alt+Q fija el QR y Ctrl+Alt+S vuelve al pase.

Las fotos se guardan (sobreviven a reinicios) con cuota: 4 GB / 500
elementos por defecto. Al pasarse se borran las más antiguas.

## 3. Decisiones y alternativas descartadas

| Decisión | Elegido | Descartado y por qué |
|---|---|---|
| Lenguaje servidor | **Go** | Python+Flask: hay que instalar dependencias en la caja y arranca más pesado. Node: más RAM. Go da un binario que se copia y listo. |
| Visor | **mpv por IPC** | Chromium kiosco: 300-500 MB de RAM y la GPU Mali-450 (driver lima) acelera mal el navegador. feh: solo imágenes y sin control fino. |
| Redimensionado | **En el móvil** | En la caja: decodificar 12-48 Mpx en un A53 tarda segundos y come RAM. El móvil lo hace en milisegundos y además sube 10× menos datos. |
| Tiempo real | **SSE** | WebSocket: más código para nada; solo el servidor empuja. |
| Base de datos | **index.json** | SQLite: innecesario para cientos de elementos. |
| Red principal | **Wifi de casa** | Ver §4. |

**Sin X ni i3.** mpv pinta directamente en la tele por DRM/KMS
(`--vo=gpu --gpu-context=drm`, o `--vo=drm` si la GPU falla). Ahorra RAM y
piezas que pueden fallar (salvapantallas, apagado de pantalla, ventanas que
roban el foco). Si algún día hace falta algo gráfico, `cage` (kiosco Wayland
mínimo) antes que un gestor de ventanas.

## 4. Red: la caja crea su propia wifi

La casa no tiene wifi, así que **el punto de acceso es el modo principal**, no
una opción. (Antes recomendaba la wifi de casa; con este dato ya no aplica.)

- **Cómo**: NetworkManager con un perfil en modo AP y `ipv4.method shared`,
  que levanta dnsmasq (DHCP y DNS) en 10.42.0.1. Se crea con
  `deploy/network/hotspot.sh`. Elegido frente a hostapd + dnsmasq a mano
  porque es un solo perfil, sin ficheros de configuración que mantener.
- **Arranque**: el perfil AP tiene `autoconnect` con prioridad baja. Si la caja
  ve una wifi conocida con más prioridad (la de tu casa, mientras la preparas)
  se conecta a ella; en la casa sin wifi levanta "Tele" sola.
- **Web en el puerto 80**: la dirección es `http://10.42.0.1`, fácil de
  teclear si el QR falla.
- **Bloqueante: el chip tiene que soportar modo AP.** El HK6334Q parece un
  módulo tipo AP6334 (Broadcom BCM4334, driver `brcmfmac`), que normalmente lo
  soporta. **No está comprobado**: hay que verlo con `iw list` (INSTALL.md,
  paso 3). Si no lo soporta, o va mal (cortes, pocos clientes), la salida es un
  **pincho wifi USB con soporte AP en el kernel**: por ejemplo con chip
  MediaTek MT7601U (2,4 GHz, barato) o MT7612U/MT7921AU (5 GHz). Evitar
  Realtek con drivers fuera del kernel.
- **El móvil dirá "sin internet"** al conectarse a "Tele". Es normal. En
  Android puede preguntar si mantener la conexión: hay que decir que sí. Con
  datos móviles activos, Android puede intentar mandar la web por datos; si
  pasa, la solución es una regla para que la caja conteste a las comprobaciones
  de conectividad de Android (ver "Pendiente"). No lo meto de entrada porque
  entonces el móvil cree que tiene internet por la wifi y WhatsApp deja de ir
  mientras está conectado.
- **Portal cautivo** (que la web se abra sola al conectar): descartado de
  momento. El mini-navegador de portal es limitado y se cierra solo. El QR es
  más fiable.
- **Nombre en vez de IP** (`tele.local` con avahi): opcional. Los iPhone lo
  resuelven; Android no siempre. El QR lleva la IP, así que no hace falta.
- **Reloj**: sin internet no hay NTP y la caja no tiene pila de reloj. La hora
  será la del último apagado (fake-hwclock de Armbian). tvcast no depende de la
  hora: ordena por orden de llegada, no por fecha.

## 5. Vídeo y HDMI en el RK3318

- **HDMI**: el driver mainline (dw-hdmi) funciona; ya tienes imagen en la consola.
  Conviene forzar 1080p (`xrandr`) aunque la tele sea 4K: a 4K la GPU no da.
- **Decodificación por hardware**: el chip tiene VPU (H.264 hasta 1080p,
  HEVC/VP9 hasta 4K), pero en Linux mainline va por la interfaz
  V4L2 "stateless" y **mpv/ffmpeg oficiales no la usan** (LibreELEC lleva
  parches propios). En Armbian hay que contar con **decodificación por CPU**.
- Lo que eso significa (estimación, hay que medirlo en la caja):
  - H.264 720p: bien. H.264 1080p30: justo, puede perder fotogramas.
  - HEVC 1080p (por defecto en iPhone) o cualquier 4K: no se verá bien.
- Por eso: **las fotos son el núcleo y el vídeo es "si cabe"**. Hay un límite
  de 300 MB por fichero. Una mejora futura sería recomprimir el vídeo en el
  móvil a 720p H.264 (WebCodecs) antes de subirlo; no lo meto ahora porque es
  complejo y el soporte en navegadores es desigual.

## 6. Seguridad

Nada se expone a internet: el router no debe tener reenvío de puertos hacia
la caja, y tvcast no hace ninguna conexión saliente.

- **Token del QR**: sin él no se puede subir, ver ni borrar. Así, aunque la
  caja esté en la wifi de casa, solo envía quien ve la tele. Cookie
  `HttpOnly`, `SameSite=Strict` (cubre CSRF). El token vive en
  `data/token`; borrarlo invalida a todos los móviles.
- **Código de 6 cifras**: alternativa al QR para quien no puede escanearlo.
  Es más débil que el token, así que el login tiene un límite global de 10
  fallos cada 10 minutos. Vive en `data/pin`; borrarlo genera otro.
- **Validación de ficheros**: el tipo se detecta por los bytes, no por la
  extensión. Permitidos: JPEG, PNG, GIF, WebP, MP4, MOV, WebM. HEIC se
  rechaza con un mensaje claro. SVG y HTML, rechazados (evita XSS).
- **Nombres en disco**: los genera el servidor (ID aleatorio). El nombre del
  móvil solo se muestra, saneado. Los ID de la URL se validan (16 hex).
- **Límites**: tamaño por fichero (`-max-file-mb`), cuota total y número de
  elementos. Cabeceras con timeout.
- **Visor**: `/tv` y el aviso de "vídeo terminado" solo se aceptan desde la
  propia caja (127.0.0.1).
- **Punto de acceso**: WPA2 con contraseña de 8+ caracteres. El QR de la wifi
  la muestra en la tele: es intencionado (quien está en el salón puede
  conectarse) y aceptable porque esa red no da acceso a nada más.
- **Pendiente**: sin HTTPS (en una LAN con IP no hay certificado válido; el
  token viaja en claro por la wifi, que ya va cifrada con WPA2). Sin límite de
  peticiones por IP.

## 7. Fase 2: mando oficial

**No sé qué tipo de mando es** y no he encontrado documentación de la Sunday
Box. Lo más probable en una caja Android de este tipo es un **mando IR**
(protocolo NEC) con receptor en la placa. Si fuera Bluetooth o radio con
pincho USB, en Linux aparecería como teclado (HID) y sería aún más fácil.

Cómo se integraría:

1. Ver si el kernel detecta el receptor IR (`/sys/class/rc/rc0`) y leer los
   códigos de cada botón con `ir-keytable -t` (ver `CHECKS.md`).
2. Escribir un mapa de teclas (`/etc/rc_keymaps/sundaybox.toml`) que convierta
   cada código en `KEY_LEFT`, `KEY_RIGHT`, `KEY_OK`...
3. Lo más simple para conectar las teclas con tvcast es **triggerhappy**
   (`thd`): escucha `/dev/input` y lanza un comando por tecla, por ejemplo
   `KEY_RIGHT 1 curl -s -X POST localhost:8080/api/next`. No hace falta tocar
   tvcast, porque las peticiones desde la propia caja no necesitan token.
4. Si funciona, más adelante tvcast puede leer `/dev/input` directamente.

**Alternativa que puede ser mejor: HDMI-CEC.** El chip tiene CEC. Si la tele
lo soporta (Anynet+, Bravia Sync, SimpLink...), **el mando de la propia tele**
manda las flechas a la caja y además tvcast podría encender la tele al llegar
una foto. Merece la pena probarlo antes de pelearse con el mando original.

## 8. Fase 2: Google Fotos

**Comprobado en la documentación de Google**: desde el 31 de marzo de 2025 la
Library API de Google Fotos **ya no deja leer la biblioteca del usuario**,
solo lo que haya subido la propia app. Los álbumes compartidos tampoco se
pueden leer por API. Para elegir fotos existentes queda la **Picker API**:

- tvcast crea una sesión y recibe un enlace (`pickerUri`) que se puede poner
  como QR en la tele;
- el usuario abre ese enlace en el móvil y elige las fotos en la interfaz de
  Google Fotos;
- tvcast pregunta hasta que la selección está hecha y descarga cada foto con
  un token OAuth (las URLs caducan en 60 minutos, así que hay que copiarlas a
  la caja; eso encaja con el diseño).

**Mi valoración: no compensa por ahora.**

- Exige internet (solo valdría en modo wifi de casa), un proyecto en Google
  Cloud, un cliente OAuth e iniciar sesión con Google. Google pide verificar
  la app; usarla como "app en pruebas" para uso propio es posible, pero en ese
  modo los tokens de refresco caducan a los 7 días (hay que reautorizar cada
  semana).
- El inicio de sesión en una caja sin navegador es delicado: Google no admite
  redirecciones OAuth a una IP privada como 10.42.0.1. Y en una casa sin
  internet, cualquier opción en la nube queda descartada de entrada.
- **En Android ya está resuelto casi gratis**: el selector de fotos de Android
  14+ muestra también las fotos de Google Fotos en la nube. Al pulsar
  "Elegir fotos" en tvcast probablemente ya puedes coger fotos de Google
  Fotos. Hay que probarlo en vuestros móviles.
- Si tienes (o quieres) un servidor de fotos propio, **Immich** tiene API con
  clave y permitiría un "álbum de la tele" que tvcast sincroniza, sin OAuth ni
  verificación. Es mucho más sencillo que Google.
- Descartado: leer álbumes compartidos por enlace "rascando" la web de Google.
  No es una API, se rompe sin aviso y va contra sus condiciones.

Orden propuesto: probar el selector de Android → Immich si lo usas → Picker de
Google solo si de verdad hace falta.

## 9. Estructura del repositorio

```
tvcast/
├── cmd/tvcast/main.go          arranque, flags, detección de IP, QR
├── internal/
│   ├── store/                  ficheros, validación, cuota (+ tests)
│   ├── player/                 cola y pase; driver mpv (IPC)
│   ├── web/                    API, SSE, auth; static/ (móvil, /tv)
│   └── idle/                   PNG de la pantalla de espera
├── deploy/
│   ├── systemd/tvcast.service  servicio del sistema (sin X)
│   └── network/hotspot.sh      punto de acceso (NetworkManager)
├── docs/DESIGN.md, INSTALL.md, CHECKS.md
├── Makefile                    build, arm64, test, run, deploy
└── README.md
```

Cuando hagas el GitHub: añadir un workflow que haga `go vet`, `go test` y
compile para arm64 en cada push, y publicar el binario arm64 en las releases.

## 10. Hoja de ruta

1. **Ahora**: seguir `INSTALL.md` en tu casa (con internet): comprobar modo
   AP, instalar, probar con la wifi "Tele" antes de llevarla.
2. Medir vídeo real de vuestros móviles y decidir si se queda.
3. Mando: CEC o IR con triggerhappy (sin internet, todo local).
4. Fotos desde la nube: descartado en esa casa (no hay internet).
