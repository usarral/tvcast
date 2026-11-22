// splash paints the box's boot image on the framebuffer as soon as systemd
// starts (deploy/splash/tvcast-splash.service):
//
//	tvcast-splash -fb /dev/fb0 -lang es
//
// It asks the framebuffer for its resolution, pixel format and line length
// and draws the image for that screen at every boot, so it works on any TV
// or monitor. (It used to be a raw file made beforehand for one resolution,
// which came out garbled on a screen with a different one.)
//
// For a preview on a computer: tvcast-splash -png splash.png -w 1920 -h 1080
package main

import (
	"flag"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"time"

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
	fbPath := flag.String("fb", "", "framebuffer to paint (e.g. /dev/fb0)")
	pngOut := flag.String("png", "", "write a PNG preview instead")
	w := flag.Int("w", 1920, "width for -png")
	h := flag.Int("h", 1080, "height for -png")
	langFlag := flag.String("lang", os.Getenv("TVCAST_LANG"), "text language: en | es (or TVCAST_LANG)")
	flag.Parse()
	lang := i18n.Default
	if *langFlag != "" {
		l, ok := i18n.Parse(*langFlag)
		if !ok {
			log.Fatalf("unknown language %q (supported: %v)", *langFlag, i18n.Supported)
		}
		lang = l
	}

	switch {
	case *pngOut != "":
		f, err := os.Create(*pngOut)
		if err != nil {
			log.Fatal(err)
		}
		if err := png.Encode(f, render(*w, *h, lang)); err != nil {
			log.Fatal(err)
		}
		f.Close()
	case *fbPath != "":
		start := time.Now()
		info, err := fbInfo(*fbPath)
		if err != nil {
			log.Fatalf("splash: %v", err)
		}
		buf, err := encode(render(info.W, info.H, lang), info)
		if err != nil {
			log.Fatalf("splash: %v", err)
		}
		if err := paint(*fbPath, info, buf); err != nil {
			log.Fatalf("splash: %v", err)
		}
		log.Printf("splash: %dx%d, %d bpp, line %d bytes, in %v", info.W, info.H, info.BPP, info.Stride, time.Since(start).Round(time.Millisecond))
	default:
		flag.Usage()
		os.Exit(2)
	}
}

// render draws the image at the screen's size; text scales with the height.
func render(w, h int, lang i18n.Lang) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)

	// Scale by the height, but don't let wide text overflow a narrow screen.
	scale := float64(h) / 1080
	if s := float64(w) / 1920; s < scale {
		scale = s
	}
	title := face(gobold.TTF, 110*scale)
	body := face(goregular.TTF, 44*scale)
	small := face(goregular.TTF, 30*scale)

	cy := h / 2
	centered(img, title, fg, cy, "tvcast")
	bar := int(8 * scale)
	bw := int(220 * scale)
	draw.Draw(img, image.Rect(w/2-bw/2, cy+int(40*scale), w/2+bw/2, cy+int(40*scale)+bar),
		&image.Uniform{accent}, image.Point{}, draw.Src)
	centered(img, body, muted, cy+int(140*scale), i18n.T(lang, "splash.booting"))
	centered(img, small, muted, h-int(70*scale), "by Usarral")
	return img
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
