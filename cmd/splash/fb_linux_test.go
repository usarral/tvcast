//go:build linux

package main

import (
	"testing"
	"unsafe"
)

// The ioctl structs must match linux/fb.h byte for byte.
func TestStructSizes(t *testing.T) {
	if s := unsafe.Sizeof(fbVarScreenInfo{}); s != 160 {
		t.Errorf("fb_var_screeninfo: %d bytes, want 160", s)
	}
	want := uintptr(68) // 32-bit
	if unsafe.Sizeof(uintptr(0)) == 8 {
		want = 80
	}
	if s := unsafe.Sizeof(fbFixScreenInfo{}); s != want {
		t.Errorf("fb_fix_screeninfo: %d bytes, want %d", s, want)
	}
}
