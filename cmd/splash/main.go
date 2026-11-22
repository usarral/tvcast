// splash draws the box's boot image as raw pixels (BGRX, 32 bits), ready to
// be copied as is to /dev/fb0 as soon as systemd starts:
//
//	go run ./cmd/splash -w 2560 -h 1440 -lang es -o splash.raw
//
// The size must match the framebuffer (/sys/class/graphics/fb0/virtual_size).
package main

import (
	"flag"
	"image"
	"image/color"
	"image/draw"
	"log"
	"os"

	"github.com/usarral/tvcast/internal/i18n"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

var (
	bg     = color.RGBA{0x14, 0x17, 0x1c, 0xff} // same colors as the QR screen
	fg     = color.RGBA{0xf2, 0xf2, 0xf2, 0xff}
	muted  = color.RGBA{0xa0, 0xa6, 0xb0, 0xff}
	accent = color.RGBA{0xff, 0xb3, 0x47, 0xff}
)

func main() {
	w := flag.Int("w", 2560, "framebuffer width")
	h := flag.Int("h", 1440, "framebuffer height")
	out := flag.String("o", "splash.raw", "output file (raw BGRX)")
	langFlag := flag.String("lang", string(i18n.Default), "text language: en | es")
	flag.Parse()
	lang, ok := i18n.Parse(*langFlag)
	if !ok {
		log.Fatalf("unknown language %q (supported: %v)", *langFlag, i18n.Supported)
	}

	img := image.NewRGBA(image.Rect(0, 0, *w, *h))
	draw.Draw(img, img.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)

	scale := float64(*h) / 1080
	title := face(gobold.TTF, 110*scale)
	body := face(goregular.TTF, 44*scale)
	small := face(goregular.TTF, 30*scale)

	cy := *h / 2
	centered(img, title, fg, cy, "tvcast")
	bar := int(8 * scale)
	bw := int(220 * scale)
	draw.Draw(img, image.Rect(*w/2-bw/2, cy+int(40*scale), *w/2+bw/2, cy+int(40*scale)+bar),
		&image.Uniform{accent}, image.Point{}, draw.Src)
	centered(img, body, muted, cy+int(140*scale), i18n.T(lang, "splash.booting"))
	centered(img, small, muted, *h-int(70*scale), "by Usarral")

	// RGBA -> BGRX (XRGB8888 little endian), which is what rockchipdrmfb expects.
	p := img.Pix
	for i := 0; i < len(p); i += 4 {
		p[i], p[i+2], p[i+3] = p[i+2], p[i], 0
	}
	if err := os.WriteFile(*out, p, 0o644); err != nil {
		log.Fatal(err)
	}
}

func face(ttf []byte, size float64) font.Face {
	f, err := opentype.Parse(ttf)
	if err != nil {
		log.Fatal(err)
	}
	fc, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		log.Fatal(err)
	}
	return fc
}

func centered(dst *image.RGBA, f font.Face, c color.Color, y int, s string) {
	d := &font.Drawer{Dst: dst, Src: &image.Uniform{c}, Face: f}
	wd := d.MeasureString(s).Ceil()
	d.Dot = fixed.P((dst.Bounds().Dx()-wd)/2, y)
	d.DrawString(s)
}
