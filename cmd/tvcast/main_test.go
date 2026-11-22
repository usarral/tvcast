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
		t.Fatalf("notified without an IP change: %s", b)
	case <-time.After(50 * time.Millisecond):
	}
	mu.Lock()
	ip = "10.42.0.1" // "Tele" comes up
	mu.Unlock()
	select {
	case b := <-got:
		if b != "http://10.42.0.1" {
			t.Fatalf("unexpected URL %s", b)
		}
	case <-time.After(time.Second):
		t.Fatal("IP change not detected")
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
		t.Fatalf("PIN changed on restart: %s -> %s", a, b)
	}
}
