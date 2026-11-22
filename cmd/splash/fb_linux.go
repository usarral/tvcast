//go:build linux

package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// Linux framebuffer ioctls (linux/fb.h).
const (
	fbioGetVScreenInfo = 0x4600
	fbioGetFScreenInfo = 0x4602
)

type fbBitfield struct{ Offset, Length, MsbRight uint32 }

type fbVarScreenInfo struct {
	XRes, YRes, XResVirtual, YResVirtual uint32
	XOffset, YOffset                     uint32
	BitsPerPixel, Grayscale              uint32
	Red, Green, Blue, Transp             fbBitfield
	Nonstd, Activate, Height, Width      uint32
	AccelFlags, Pixclock                 uint32
	LeftMargin, RightMargin              uint32
	UpperMargin, LowerMargin             uint32
	HsyncLen, VsyncLen, Sync, Vmode      uint32
	Rotate, Colorspace                   uint32
	Reserved                             [4]uint32
}

type fbFixScreenInfo struct {
	ID                 [16]byte
	SmemStart          uintptr // unsigned long
	SmemLen, Type      uint32
	TypeAux, Visual    uint32
	XPanStep, YPanStep uint16
	YWrapStep          uint16
	LineLength         uint32
	MmioStart          uintptr
	MmioLen, Accel     uint32
	Capabilities       uint16
	Reserved           [2]uint16
}

func ioctl(fd uintptr, req uintptr, p unsafe.Pointer) error {
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(p)); e != 0 {
		return e
	}
	return nil
}

func fbInfo(path string) (FB, error) {
	f, err := os.Open(path)
	if err != nil {
		return FB{}, err
	}
	defer f.Close()
	var v fbVarScreenInfo
	var x fbFixScreenInfo
	if err := ioctl(f.Fd(), fbioGetVScreenInfo, unsafe.Pointer(&v)); err != nil {
		return FB{}, fmt.Errorf("%s: FBIOGET_VSCREENINFO: %w", path, err)
	}
	if err := ioctl(f.Fd(), fbioGetFScreenInfo, unsafe.Pointer(&x)); err != nil {
		return FB{}, fmt.Errorf("%s: FBIOGET_FSCREENINFO: %w", path, err)
	}
	bf := func(b fbBitfield) field { return field{b.Offset, b.Length} }
	return FB{
		W: int(v.XRes), H: int(v.YRes), BPP: int(v.BitsPerPixel), Stride: int(x.LineLength),
		XOff: int(v.XOffset), YOff: int(v.YOffset),
		Red: bf(v.Red), Green: bf(v.Green), Blue: bf(v.Blue), A: bf(v.Transp),
		Name: strings.TrimRight(string(x.ID[:]), "\x00"),
	}, nil
}

// paint writes the encoded lines where the visible area starts.
func paint(path string, fb FB, buf []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	off := int64(fb.YOff*fb.Stride + fb.XOff*fb.BPP/8)
	if _, err := f.WriteAt(buf, off); err != nil {
		return err
	}
	return nil
}
