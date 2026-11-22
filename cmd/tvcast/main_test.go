package main

import (
	"sync"
	"testing"
	"time"
)

func TestWatchURLDetectsNetworkComingUp(t *testing.T) {
	var mu sync.Mutex
	ip := "127.0.0.1"
	base := func() string { mu.Lock(); defer mu.Unlock(); return "http://" + ip }
	got := make(chan string, 4)
	stop := make(chan struct{})
	defer close(stop)
	go watchURL(stop, 10*time.Millisecond, base(), base, func(b string) { got <- b })

	select {
	case b := <-got:
		t.Fatalf("aviso sin cambio de IP: %s", b)
	case <-time.After(50 * time.Millisecond):
	}
	mu.Lock()
	ip = "10.42.0.1" // se levanta "Tele"
	mu.Unlock()
	select {
	case b := <-got:
		if b != "http://10.42.0.1" {
			t.Fatalf("URL inesperada %s", b)
		}
	case <-time.After(time.Second):
		t.Fatal("no detectó el cambio de IP")
	}
}

func TestLoadPINStable(t *testing.T) {
	p := t.TempDir() + "/pin"
	a, err := loadPIN(p)
	if err != nil || len(a) != 6 {
		t.Fatalf("pin %q, %v", a, err)
	}
	b, _ := loadPIN(p)
	if a != b {
		t.Fatalf("el PIN cambió al reiniciar: %s -> %s", a, b)
	}
}
