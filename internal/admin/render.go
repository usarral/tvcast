package admin

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"sync"

	"github.com/usarral/tvcast/internal/i18n"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// W and H are the panel size: the whole 1080p screen.
const W, H = 1920, 1080

var (
	bg     = color.RGBA{0x10, 0x12, 0x16, 0xff}
	panel  = color.RGBA{0x1c, 0x20, 0x27, 0xff}
	fg     = color.RGBA{0xf2, 0xf2, 0xf2, 0xff}
	muted  = color.RGBA{0xa0, 0xa6, 0xb0, 0xff}
	accent = color.RGBA{0xff, 0xb3, 0x47, 0xff}
	dark   = color.RGBA{0x14, 0x17, 0x1c, 0xff}
	danger = color.RGBA{0xe0, 0x55, 0x4a, 0xff}
)

type faces struct{ title, body, small, mono font.Face }

var (
	facesOnce sync.Once
	fcs       faces
	facesErr  error
)

func loadFaces() (faces, error) {
	facesOnce.Do(func() {
		mk := func(ttf []byte, size float64) font.Face {
			if facesErr != nil {
				return nil
			}
			f, err := opentype.Parse(ttf)
			if err != nil {
				facesErr = err
				return nil
			}
			fc, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
			if err != nil {
				facesErr = err
			}
			return fc
		}
		fcs = faces{mk(gobold.TTF, 56), mk(goregular.TTF, 40), mk(goregular.TTF, 30), mk(gomono.TTF, 24)}
	})
	return fcs, facesErr
}

// Render draws the panel. status are the lines at the top of the main menu.
func Render(v View, status []string, lang i18n.Lang) (*image.RGBA, error) {
	f, err := loadFaces()
	if err != nil {
		return nil, err
	}
	T := func(k string) string { return i18n.T(lang, k) }
	img := image.NewRGBA(image.Rect(0, 0, W, H))
	fill(img, img.Bounds(), bg)

	title := T("adm.title")
	if v.Title != "" {
		title += "  ›  " + T(v.Title)
	}
	if v.Log != nil {
		title += "  ›  " + T(v.LogTitle)
	}
	text(img, f.title, accent, 100, 120, title)

	y := 200
	if v.Log != nil {
		fill(img, image.Rect(80, 170, W-80, H-150), panel)
		y = 215
		lines := v.Log
		if len(lines) == 0 {
			lines = []string{T("adm.log_empty")}
		}
		for _, l := range lines {
			text(img, f.mono, fg, 110, y, clip(f.mono, l, W-220))
			y += 34
			if y > H-170 {
				break
			}
		}
		footer(img, f, T("adm.hint_log"))
		return img, nil
	}

	if v.Title == "" && len(status) > 0 { // status in a column on the right
		fill(img, image.Rect(1180, 170, W-80, 230+len(status)*48), panel)
		sy := 225
		for _, s := range status {
			text(img, f.small, fg, 1210, sy, clip(f.small, s, W-80-1240))
			sy += 48
		}
	}
	y = 240
	for i, k := range v.Items {
		label := T(k)
		if i == v.Cursor {
			fill(img, image.Rect(80, y-48, 1100, y+18), accent)
			text(img, f.body, dark, 110, y, "› "+label)
		} else {
			text(img, f.body, fg, 110, y, "  "+label)
		}
		y += 68
	}
	if v.Message != "" {
		text(img, f.small, accent, 100, H-110, clip(f.small, v.Message, W-200))
	}
	footer(img, f, T("adm.hint"))

	if v.Confirm != "" {
		box := image.Rect(460, 340, W-460, 740)
		fill(img, box, panel)
		border(img, box, danger)
		q := fmt.Sprintf(T("adm.confirm"), T(v.Confirm))
		center(img, f.body, fg, 450, clip(f.body, q, box.Dx()-60))
		no, yes := T("adm.no"), T("adm.yes")
		bx := func(x int, label string, on bool) {
			r := image.Rect(x, 560, x+300, 640)
			if on {
				fill(img, r, accent)
				text(img, f.body, dark, x+(300-font.MeasureString(f.body, label).Ceil())/2, 615, label)
			} else {
				border(img, r, muted)
				text(img, f.body, fg, x+(300-font.MeasureString(f.body, label).Ceil())/2, 615, label)
			}
		}
		bx(W/2-340, no, !v.Yes)
		bx(W/2+40, yes, v.Yes)
	}
	return img, nil
}

func footer(img *image.RGBA, f faces, s string) {
	text(img, f.small, muted, 100, H-50, s)
}

func fill(img *image.RGBA, r image.Rectangle, c color.Color) {
	draw.Draw(img, r, &image.Uniform{c}, image.Point{}, draw.Src)
}

func border(img *image.RGBA, r image.Rectangle, c color.Color) {
	const t = 4
	fill(img, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+t), c)
	fill(img, image.Rect(r.Min.X, r.Max.Y-t, r.Max.X, r.Max.Y), c)
	fill(img, image.Rect(r.Min.X, r.Min.Y, r.Min.X+t, r.Max.Y), c)
	fill(img, image.Rect(r.Max.X-t, r.Min.Y, r.Max.X, r.Max.Y), c)
}

func text(dst *image.RGBA, f font.Face, c color.Color, x, y int, s string) {
	d := font.Drawer{Dst: dst, Src: image.NewUniform(c), Face: f, Dot: fixed.P(x, y)}
	d.DrawString(s)
}

func center(dst *image.RGBA, f font.Face, c color.Color, y int, s string) {
	text(dst, f, c, (W-font.MeasureString(f, s).Ceil())/2, y, s)
}

// clip shortens s with "…" so it fits in max pixels.
func clip(f font.Face, s string, max int) string {
	if font.MeasureString(f, s).Ceil() <= max {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && font.MeasureString(f, string(r)+"…").Ceil() > max {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

// BGRA converts an opaque image to the raw format of mpv's overlay-add.
func BGRA(img *image.RGBA) []byte {
	n := img.Bounds().Dx() * img.Bounds().Dy()
	raw := make([]byte, n*4)
	for i := 0; i < n; i++ {
		raw[i*4+0] = img.Pix[i*4+2]
		raw[i*4+1] = img.Pix[i*4+1]
		raw[i*4+2] = img.Pix[i*4+0]
		raw[i*4+3] = 0xff
	}
	return raw
}
