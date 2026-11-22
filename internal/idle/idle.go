// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 usarral

// Package idle draws the waiting screen: a 1920x1080 PNG with the QR code
// of the web page (and, when the box runs its own wifi, the QR to join it).
package idle

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
	"github.com/usarral/tvcast/internal/i18n"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

type Info struct {
	URL      string // URL with the token, the one in the QR
	ShowURL  string // URL to read on screen (no token)
	WifiSSID string // empty = no wifi QR
	WifiPass string
	PIN      string // short code to type on the web page if the QR can't be scanned
	Lang     i18n.Lang
}

const W, H = 1920, 1080

var (
	bg     = color.RGBA{0x14, 0x17, 0x1c, 0xff}
	fg     = color.RGBA{0xf2, 0xf2, 0xf2, 0xff}
	muted  = color.RGBA{0xa0, 0xa6, 0xb0, 0xff}
	accent = color.RGBA{0xff, 0xb3, 0x47, 0xff}
)

func Render(path string, in Info) error {
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	draw.Draw(img, img.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)

	title, err := face(gobold.TTF, 72)
	if err != nil {
		return err
	}
	body, err := face(goregular.TTF, 40)
	if err != nil {
		return err
	}
	small, err := face(goregular.TTF, 30)
	if err != nil {
		return err
	}

	titleText := i18n.T(in.Lang, "tv.title")
	text(img, title, fg, (W-font.MeasureString(title, titleText).Ceil())/2, 170, titleText)

	qrSize := 460
	y := 260
	n := 1
	if in.WifiSSID != "" {
		n = 2
	}
	x := (W - (n*qrSize + (n-1)*140)) / 2 // center the QR block
	step := 1
	if in.WifiSSID != "" {
		if err := drawQR(img, wifiQR(in.WifiSSID, in.WifiPass), x, y, qrSize); err != nil {
			return err
		}
		text(img, body, accent, x, y+qrSize+60, fmt.Sprintf(i18n.T(in.Lang, "tv.join_wifi"), step))
		text(img, small, muted, x, y+qrSize+105, in.WifiSSID)
		x += qrSize + 140
		step++
	}
	if err := drawQR(img, in.URL, x, y, qrSize); err != nil {
		return err
	}
	text(img, body, accent, x, y+qrSize+60, fmt.Sprintf(i18n.T(in.Lang, "tv.scan"), step))
	text(img, small, muted, x, y+qrSize+105, in.ShowURL)
	if in.PIN != "" {
		pinText := fmt.Sprintf(i18n.T(in.Lang, "tv.code"), in.PIN)
		text(img, body, fg, (W-font.MeasureString(body, pinText).Ceil())/2, H-90, pinText)
	}

	return writePNG(path, img)
}

// Corner draws the small QR shown in a corner during the slideshow: a PNG
// (web viewer) and the same image as raw BGRA (mpv overlay). It returns the
// size in pixels.
func Corner(pngPath, bgraPath, url, pin string, lang i18n.Lang) (w, h int, err error) {
	const qrSize, pad = 260, 14
	small, err := face(goregular.TTF, 26)
	if err != nil {
		return 0, 0, err
	}
	bold, err := face(gobold.TTF, 30)
	if err != nil {
		return 0, 0, err
	}
	w = qrSize + 2*pad
	h = qrSize + pad + 82
	if pin == "" {
		h = qrSize + pad + 44
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)
	if err := drawQR(img, url, pad, pad, qrSize); err != nil {
		return 0, 0, err
	}
	dark := color.RGBA{0x14, 0x17, 0x1c, 0xff}
	center := func(f font.Face, y int, s string) {
		text(img, f, dark, (w-font.MeasureString(f, s).Ceil())/2, y, s)
	}
	center(small, qrSize+pad+28, i18n.T(lang, "corner.title"))
	if pin != "" {
		center(bold, qrSize+pad+68, fmt.Sprintf(i18n.T(lang, "corner.code"), pin))
	}
	if err := writePNG(pngPath, img); err != nil {
		return 0, 0, err
	}
	// BGRA with premultiplied alpha (opaque, so swapping R and B is enough).
	raw := make([]byte, w*h*4)
	for i := 0; i < w*h; i++ {
		raw[i*4+0] = img.Pix[i*4+2]
		raw[i*4+1] = img.Pix[i*4+1]
		raw[i*4+2] = img.Pix[i*4+0]
		raw[i*4+3] = 0xff
	}
	tmp := bgraPath + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return 0, 0, err
	}
	return w, h, os.Rename(tmp, bgraPath)
}

func writePNG(path string, img image.Image) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// wifiQR uses the standard format understood by iOS and Android cameras.
func wifiQR(ssid, pass string) string {
	esc := strings.NewReplacer(`\`, `\\`, `;`, `\;`, `,`, `\,`, `:`, `\:`, `"`, `\"`)
	if pass == "" {
		return fmt.Sprintf("WIFI:T:nopass;S:%s;;", esc.Replace(ssid))
	}
	return fmt.Sprintf("WIFI:T:WPA;S:%s;P:%s;;", esc.Replace(ssid), esc.Replace(pass))
}

func drawQR(dst *image.RGBA, content string, x, y, size int) error {
	q, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		return err
	}
	q.BackgroundColor = color.White
	q.ForegroundColor = color.Black
	draw.Draw(dst, image.Rect(x, y, x+size, y+size), q.Image(size), image.Point{}, draw.Src)
	return nil
}

func face(ttf []byte, size float64) (font.Face, error) {
	f, err := opentype.Parse(ttf)
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
}

func text(dst *image.RGBA, f font.Face, c color.Color, x, y int, s string) {
	d := font.Drawer{Dst: dst, Src: image.NewUniform(c), Face: f, Dot: fixed.P(x, y)}
	d.DrawString(s)
}
