//go:build !linux

package main

import "errors"

func fbInfo(string) (FB, error)      { return FB{}, errors.New("framebuffer only on Linux") }
func paint(string, FB, []byte) error { return errors.New("framebuffer only on Linux") }
