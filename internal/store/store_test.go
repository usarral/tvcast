package store

import (
	"bytes"
	"errors"
	"os"
	"testing"
)

var jpegHead = []byte{0xff, 0xd8, 0xff, 0xe0, 0, 0x10, 'J', 'F', 'I', 'F', 0}

func TestSniff(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want string
		err  error
	}{
		{"jpeg", jpegHead, "image/jpeg", nil},
		{"png", []byte("\x89PNG\r\n\x1a\n0000"), "image/png", nil},
		{"mp4", []byte("\x00\x00\x00\x18ftypisom\x00\x00\x02\x00"), "video/mp4", nil},
		{"mov", []byte("\x00\x00\x00\x14ftypqt  \x00\x00\x00\x00"), "video/quicktime", nil},
		{"heic", []byte("\x00\x00\x00\x18ftypheic\x00\x00\x00\x00"), "", ErrHEIC},
		{"exe", []byte("MZ\x90\x00\x03\x00"), "", ErrUnsupported},
		{"html", []byte("<html><script>alert(1)</script>"), "", ErrUnsupported},
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), "", ErrUnsupported},
	}
	for _, c := range cases {
		got, err := Sniff(c.in)
		if got != c.want || !errors.Is(err, c.err) {
			t.Errorf("%s: got %q,%v want %q,%v", c.name, got, err, c.want, c.err)
		}
	}
}

func TestSaveLimitsAndQuota(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir, 1000, 2500, 0)
	if err != nil {
		t.Fatal(err)
	}
	img := func(n int) *bytes.Reader {
		return bytes.NewReader(append(append([]byte{}, jpegHead...), make([]byte, n)...))
	}

	if _, err := s.Save("x.jpg", img(2000), nil); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("want ErrTooLarge, got %v", err)
	}
	var ids []string
	for i := 0; i < 3; i++ {
		it, err := s.Save("../../etc/passwd", img(900), nil)
		if err != nil {
			t.Fatal(err)
		}
		if it.Name != "passwd" {
			t.Errorf("name not sanitized: %q", it.Name)
		}
		ids = append(ids, it.ID)
	}
	// 3 x ~911 bytes > 2500: the oldest must have been deleted.
	if _, ok := s.Get(ids[0]); ok {
		t.Error("the quota did not delete the oldest")
	}
	if len(s.List()) != 2 {
		t.Errorf("want 2 items, got %d", len(s.List()))
	}
	entries, _ := os.ReadDir(s.mediaDir())
	if len(entries) != 2 {
		t.Errorf("%d files left on disk (temp files not cleaned?)", len(entries))
	}
	// Reopening keeps the index.
	s2, err := Open(dir, 1000, 2500, 0)
	if err != nil || len(s2.List()) != 2 {
		t.Fatalf("reopen: %v, %d", err, len(s2.List()))
	}
	if err := s2.Delete(ids[2]); err != nil {
		t.Fatal(err)
	}
	if err := s2.Delete(ids[2]); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete twice: %v", err)
	}
}

func TestValidID(t *testing.T) {
	for _, id := range []string{"", "../etc/passwd", "0123456789abcdeg", "0123456789abcdef0"} {
		if ValidID(id) {
			t.Errorf("%q should not be valid", id)
		}
	}
	if !ValidID("0123456789abcdef") {
		t.Error("valid id rejected")
	}
}
