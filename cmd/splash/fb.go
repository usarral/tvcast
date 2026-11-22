package main

import (
	"fmt"
	"image"
)

// field is where one color sits inside a pixel (struct fb_bitfield).
type field struct{ Offset, Length uint32 }

// FB describes the framebuffer as the kernel reports it.
type FB struct {
	W, H                int    // visible resolution
	BPP                 int    // bits per pixel: 16, 24 or 32
	Stride              int    // bytes per line (may be more than W*BPP/8)
	XOff, YOff          int    // visible area inside the virtual one
	Red, Green, Blue, A field  // Length 0 = no such channel
	Name                string // driver id, for the log
}

// encode packs img into the framebuffer's format, line by line with its
// stride, covering only the visible area.
func encode(img *image.RGBA, fb FB) ([]byte, error) {
	bpp := fb.BPP / 8
	if fb.BPP%8 != 0 || bpp < 2 || bpp > 4 {
		return nil, fmt.Errorf("unsupported %d bits per pixel", fb.BPP)
	}
	if fb.Stride < fb.W*bpp {
		return nil, fmt.Errorf("line length %d is shorter than %d pixels", fb.Stride, fb.W)
	}
	if fb.Red.Length == 0 || fb.Green.Length == 0 || fb.Blue.Length == 0 {
		return nil, fmt.Errorf("unknown pixel layout")
	}
	pack := func(v uint8, f field) uint32 {
		if f.Length == 0 {
			return 0
		}
		return uint32(v) >> (8 - f.Length) << f.Offset
	}
	out := make([]byte, fb.Stride*fb.H)
	for y := 0; y < fb.H; y++ {
		row := out[y*fb.Stride:]
		for x := 0; x < fb.W; x++ {
			i := img.PixOffset(x, y)
			p := pack(img.Pix[i], fb.Red) | pack(img.Pix[i+1], fb.Green) | pack(img.Pix[i+2], fb.Blue) | pack(0xff, fb.A)
			o := x * bpp
			for b := 0; b < bpp; b++ { // little endian, like the hardware
				row[o+b] = byte(p >> (8 * b))
			}
		}
	}
	return out, nil
}
