// Package remote turns the official Sunday Box Bluetooth LE remote into
// tvcast actions.
//
// The remote is not a standard HID device: its keys arrive as GATT
// notifications on a custom service (0xFF00), so the kernel creates no
// /dev/input and triggerhappy never sees them. This package decodes those
// notifications (decode.go), maps buttons to calls to tvcast's local API
// (actions.go), keeps the link up (run.go) and talks to BlueZ over D-Bus
// (bluez.go). Everything except bluez.go is tested without hardware.
// Byte layout: docs/LESSONS.md, "Original remote".
package remote

import "fmt"

// Button is a key on the remote.
type Button int

const (
	None Button = iota
	Up
	Down
	Left
	Right
	OK
	Back
	Home
	PlayPause
	Heart
	Power
	VolUp
	VolDown
	Mic
)

var buttonNames = [...]string{"none", "up", "down", "left", "right", "ok", "back", "home",
	"playpause", "heart", "power", "volup", "voldown", "mic"}

func (b Button) String() string {
	if b >= 0 && int(b) < len(buttonNames) {
		return buttonNames[b]
	}
	return fmt.Sprintf("button(%d)", int(b))
}

// Source says which characteristic a notification came from.
type Source int

const (
	// Report is a 0x2a4d characteristic of service 0xFF00. There are several
	// (keyboard and consumer); they are told apart by length, not by handle.
	Report Source = iota
	// Voice is f000b001: 04 when Mic is pressed, 00 when released. The audio
	// itself goes to f000b002, which we never subscribe to.
	Voice
)

// Notification is one GATT value notification from the remote.
type Notification struct {
	From  Source
	Value []byte
}

func (n Notification) String() string {
	src := "report"
	if n.From == Voice {
		src = "voice"
	}
	return fmt.Sprintf("%s % x", src, n.Value)
}

// Keyboard report (8 bytes): standard HID usages in byte 2.
var keyboard = map[byte]Button{
	0x52: Up,
	0x51: Down,
	0x50: Left,
	0x4f: Right,
	0x28: OK,
}

// Consumer report (1 byte): the remote's own codes, not a bitmask.
var consumer = map[byte]Button{
	0x0b: Back,
	0x05: Home,
	0x02: PlayPause,
	0x3c: Heart,
	0x04: Power,
	0x09: VolUp,
	0x0a: VolDown,
}

// Event is a decoded notification.
type Event struct {
	Button  Button // None on a release
	Release bool   // all zeros: the key was let go
	Unknown bool   // a shape or code we don't know (logged, not acted on)
}

// Decode turns a notification into an event. It never fails: anything odd is
// reported as Unknown so the caller can log it and keep going.
func Decode(n Notification) Event {
	v := n.Value
	switch {
	case n.From == Voice && len(v) == 1:
		switch v[0] {
		case 0x00:
			return Event{Release: true}
		case 0x04:
			return Event{Button: Mic}
		}
	case n.From == Report && len(v) == 8:
		if allZero(v) {
			return Event{Release: true}
		}
		// Only byte 2 is used; a second simultaneous key (byte 3...) or a
		// modifier (byte 0) is not something this remote sends.
		if b, ok := keyboard[v[2]]; ok && allZero(v[3:]) && v[0] == 0 {
			return Event{Button: b}
		}
	case n.From == Report && len(v) == 1:
		if v[0] == 0 {
			return Event{Release: true}
		}
		if b, ok := consumer[v[0]]; ok {
			return Event{Button: b}
		}
	}
	return Event{Unknown: true}
}

func allZero(b []byte) bool {
	for _, x := range b {
		if x != 0 {
			return false
		}
	}
	return true
}
