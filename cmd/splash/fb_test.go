package main

import (
	"image"
	"image/color"
	"testing"

	"github.com/usarral/tvcast/internal/i18n"
)

func px(c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

var orange = color.RGBA{0xff, 0xb3, 0x47, 0xff}

func TestEncodeXRGB8888WithStride(t *testing.T) {
	fb := FB{W: 3, H: 2, BPP: 32, Stride: 16, // 4 bytes of padding per line
		Red: field{16, 8}, Green: field{8, 8}, Blue: field{0, 8}}
	b, err := encode(px(orange), fb)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 32 {
		t.Fatalf("len %d", len(b))
	}
	// BGRX in memory; padding untouched; second line starts at the stride.
	want := []byte{0x47, 0xb3, 0xff, 0x00}
	for _, o := range []int{0, 4, 8, 16, 20, 24} {
		if string(b[o:o+4]) != string(want) {
			t.Fatalf("pixel at %d: % x", o, b[o:o+4])
		}
	}
	if b[12] != 0 || b[15] != 0 {
		t.Fatal("padding written")
	}
}

func TestEncodeRGB565(t *testing.T) {
	fb := FB{W: 3, H: 2, BPP: 16, Stride: 6, Red: field{11, 5}, Green: field{5, 6}, Blue: field{0, 5}}
	b, err := encode(px(orange), fb)
	if err != nil {
		t.Fatal(err)
	}
	v := uint16(b[0]) | uint16(b[1])<<8
	if v != (0x1f<<11 | (0xb3>>2)<<5 | 0x47>>3) {
		t.Fatalf("got %#04x", v)
	}
}

func TestEncodeABGR(t *testing.T) {
	// Another common layout: R in the low byte, with alpha.
	fb := FB{W: 3, H: 2, BPP: 32, Stride: 12, Red: field{0, 8}, Green: field{8, 8}, Blue: field{16, 8}, A: field{24, 8}}
	b, _ := encode(px(orange), fb)
	if string(b[:4]) != string([]byte{0xff, 0xb3, 0x47, 0xff}) {
		t.Fatalf("% x", b[:4])
	}
}

func TestEncodeRejectsOdd(t *testing.T) {
	for _, fb := range []FB{
		{W: 3, H: 2, BPP: 8, Stride: 3, Red: field{0, 3}, Green: field{3, 3}, Blue: field{6, 2}},
		{W: 3, H: 2, BPP: 32, Stride: 8, Red: field{16, 8}, Green: field{8, 8}, Blue: field{0, 8}},
		{W: 3, H: 2, BPP: 32, Stride: 12},
	} {
		if _, err := encode(px(orange), fb); err == nil {
			t.Errorf("%+v accepted", fb)
		}
	}
}

// The screens seen so far: the TV (2560x1440 framebuffer), a 1080p portable
// monitor, and odd sizes.
func TestRenderSizes(t *testing.T) {
	for _, s := range [][2]int{{2560, 1440}, {1920, 1080}, {1280, 720}, {1024, 768}, {800, 1280}} {
		img := render(s[0], s[1], i18n.ES)
		if img.Bounds().Dx() != s[0] || img.Bounds().Dy() != s[1] {
			t.Fatalf("%v: %v", s, img.Bounds())
		}
	}
}
