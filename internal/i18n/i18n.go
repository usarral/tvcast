// Package i18n holds the few user-facing strings of tvcast (phone pages, TV
// screen, boot image, HTTP errors) in English and Spanish.
//
// It is deliberately a plain map, not gettext or x/text: there are a few dozen
// strings and two languages. To add a language, copy the "en" block, translate
// it and add the code to Supported. TestCatalogComplete checks no key is missing.
package i18n

import (
	"net/http"
	"strings"
)

// Lang is a language code ("en", "es").
type Lang string

const (
	EN Lang = "en"
	ES Lang = "es"
)

// Default is the language when nothing else is configured.
const Default = EN

// Supported lists the languages with a complete catalog.
var Supported = []Lang{EN, ES}

// Parse turns "es", "es_ES.UTF-8", "ES-es"... into a supported Lang.
func Parse(s string) (Lang, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.IndexAny(s, "-_.;"); i >= 0 {
		s = s[:i]
	}
	for _, l := range Supported {
		if string(l) == s {
			return l, true
		}
	}
	return "", false
}

// FromRequest picks the first supported language in the browser's
// Accept-Language header, or def. Quality values are ignored: browsers
// already list languages in order of preference.
func FromRequest(r *http.Request, def Lang) Lang {
	for _, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		if l, ok := Parse(part); ok {
			return l
		}
	}
	return def
}

// T returns the text for key in lang, falling back to English and then to
// the key itself (so a missing string is visible, not empty).
func T(lang Lang, key string) string {
	if s, ok := catalog[lang][key]; ok {
		return s
	}
	if s, ok := catalog[EN][key]; ok {
		return s
	}
	return key
}

// Strings returns the whole catalog of a language (used by the HTML templates).
func Strings(lang Lang) map[string]string {
	if m, ok := catalog[lang]; ok {
		return m
	}
	return catalog[EN]
}

var catalog = map[Lang]map[string]string{
	EN: {
		// TV screen (QR) and corner QR
		"tv.title":     "Send photos to the TV",
		"tv.join_wifi": "%d. Join the wifi",
		"tv.scan":      "%d. Scan and pick photos",
		"tv.code":      "Code: %s",
		"corner.title": "Send photos to the TV",
		"corner.code":  "Code %s",
		// Boot image
		"splash.booting": "Starting… one moment",
		// Phone pages
		"page.title":       "Photos to the TV",
		"page.heading":     "Send to the TV",
		"page.pick":        "Pick photos or videos",
		"page.prev":        "Previous",
		"page.next":        "Next",
		"page.pause":       "⏸ Pause",
		"page.resume":      "▶ Resume",
		"page.delete":      "Delete",
		"page.confirm_del": "Remove from the TV?",
		"page.sending":     "Sending {i} of {n}…",
		"page.sent":        "Sent {ok} of {n}",
		"page.sent_none":   "Nothing was sent",
		"page.no_box":      "No connection to the box",
		"locked.heading":   "Send photos to the TV",
		"locked.help":      "Type the 6-digit code shown on the TV (bottom right, next to the QR code), or scan that QR with the camera.",
		"locked.enter":     "Enter",
		"locked.no_tv":     "No connection to the TV",
		// HTTP errors shown on the phone
		"err.scan_qr":     "Scan the QR code on the TV",
		"err.local_only":  "Only from the box itself",
		"err.not_found":   "Not found",
		"err.unavailable": "Not available",
		"err.too_many":    "Too many attempts. Wait a few minutes or scan the QR code.",
		"err.bad_code":    "Wrong code",
		"err.multipart":   "Expected multipart/form-data",
		"err.no_file":     "No file received",
		"err.too_large":   "File too large",
		"err.unsupported": "Unsupported format",
		"err.heic":        "HEIC is not supported: send it from the phone's browser (it converts it to JPEG) or set the camera to 'Most Compatible'",
		"err.save":        "Could not save the file",
		"err.bad_id":      "Invalid id",
		"err.no_stream":   "Streaming not supported",
	},
	ES: {
		"tv.title":     "Envía fotos a la tele",
		"tv.join_wifi": "%d. Únete a la wifi",
		"tv.scan":      "%d. Escanea y elige fotos",
		"tv.code":      "Código: %s",
		"corner.title": "Envía fotos a la tele",
		"corner.code":  "Código %s",

		"splash.booting": "Encendiendo… un momento",

		"page.title":       "Fotos a la tele",
		"page.heading":     "Enviar a la tele",
		"page.pick":        "Elegir fotos o vídeos",
		"page.prev":        "Anterior",
		"page.next":        "Siguiente",
		"page.pause":       "⏸ Pausa",
		"page.resume":      "▶ Seguir",
		"page.delete":      "Borrar",
		"page.confirm_del": "¿Borrar de la tele?",
		"page.sending":     "Enviando {i} de {n}…",
		"page.sent":        "Enviadas {ok} de {n}",
		"page.sent_none":   "No se envió nada",
		"page.no_box":      "Sin conexión con la caja",
		"locked.heading":   "Enviar fotos a la tele",
		"locked.help":      "Escribe el código de 6 cifras que sale en la tele (abajo a la derecha, junto al código QR), o escanea ese QR con la cámara.",
		"locked.enter":     "Entrar",
		"locked.no_tv":     "Sin conexión con la tele",

		"err.scan_qr":     "Escanea el QR de la tele",
		"err.local_only":  "Solo desde la caja",
		"err.not_found":   "No encontrado",
		"err.unavailable": "No disponible",
		"err.too_many":    "Demasiados intentos. Espera unos minutos o escanea el QR.",
		"err.bad_code":    "Código incorrecto",
		"err.multipart":   "Se esperaba multipart/form-data",
		"err.no_file":     "No llegó ningún fichero",
		"err.too_large":   "Demasiado grande",
		"err.unsupported": "Formato no soportado",
		"err.heic":        "HEIC no soportado: envíala desde el navegador del móvil (la convierte a JPEG) o cambia la cámara a 'Más compatible'",
		"err.save":        "Error al guardar",
		"err.bad_id":      "Id no válido",
		"err.no_stream":   "Sin streaming",
	},
}
