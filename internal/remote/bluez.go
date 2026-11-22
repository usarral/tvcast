package remote

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// UUIDs as BlueZ reports them (lower case, 128-bit).
const (
	uuidReport = "00002a4d-0000-1000-8000-00805f9b34fb"
	uuidVoice  = "f000b001-0451-4000-b000-000000000000"

	ifDevice = "org.bluez.Device1"
	ifChar   = "org.bluez.GattCharacteristic1"
	ifProps  = "org.freedesktop.DBus.Properties"
)

// DefaultNames are the names the remote advertises (from the original app).
var DefaultNames = []string{"Sunday", "4MOD 45 BLE"}

// BlueZ is a Link over BlueZ's D-Bus API. It never pairs or scans: the remote
// must be paired and trusted once with bluetoothctl (docs/INSTALL.md). Then
// Device1.Connect reconnects with the stored bond, even when the remote isn't
// in BlueZ's recent scan results (unlike bleak, which needs to see it advertise).
type BlueZ struct {
	Conn *dbus.Conn
	// Address picks the remote by address (AA:BB:...). Empty: the first
	// paired device whose name is in Names, on any adapter. Leaving it empty
	// keeps addresses out of config files.
	Address string
	Names   []string
	// ConnectTimeout bounds one Device1.Connect call.
	ConnectTimeout time.Duration
	Log            *log.Logger
}

type objects = map[dbus.ObjectPath]map[string]map[string]dbus.Variant

func (b *BlueZ) objects(ctx context.Context) (objects, error) {
	var objs objects
	err := b.Conn.Object("org.bluez", "/").CallWithContext(ctx,
		"org.freedesktop.DBus.ObjectManager.GetManagedObjects", 0).Store(&objs)
	if err != nil {
		return nil, fmt.Errorf("bluez: %w (is bluetooth.service running?)", err)
	}
	return objs, nil
}

// Listen implements Link.
func (b *BlueZ) Listen(ctx context.Context, emit func(Notification)) error {
	objs, err := b.objects(ctx)
	if err != nil {
		return err
	}
	dev, err := findDevice(objs, b.Address, b.Names)
	if err != nil {
		return err
	}
	devObj := b.Conn.Object("org.bluez", dev)

	// Subscribe to signals before connecting so no change is missed.
	sigs := make(chan *dbus.Signal, 64)
	b.Conn.Signal(sigs)
	defer b.Conn.RemoveSignal(sigs)
	match := []dbus.MatchOption{
		dbus.WithMatchInterface(ifProps),
		dbus.WithMatchMember("PropertiesChanged"),
		dbus.WithMatchPathNamespace(dev),
	}
	if err := b.Conn.AddMatchSignalContext(ctx, match...); err != nil {
		return fmt.Errorf("bluez: match: %w", err)
	}
	defer b.Conn.RemoveMatchSignal(match...)

	if !boolProp(objs[dev][ifDevice], "Connected") {
		cctx, cancel := context.WithTimeout(ctx, b.ConnectTimeout)
		err := devObj.CallWithContext(cctx, ifDevice+".Connect", 0).Err
		cancel()
		if err != nil {
			return fmt.Errorf("bluez: connect: %w", err)
		}
	}
	if err := b.waitResolved(ctx, devObj); err != nil {
		return err
	}

	// Services are resolved: the characteristics now exist on the bus.
	if objs, err = b.objects(ctx); err != nil {
		return err
	}
	chars := findChars(objs, dev)
	if len(chars) == 0 {
		return errors.New("bluez: connected, but no 0x2a4d/f000b001 characteristic found")
	}
	for path := range chars {
		if err := b.Conn.Object("org.bluez", path).CallWithContext(ctx, ifChar+".StartNotify", 0).Err; err != nil {
			// "In Progress"/"Already notifying" are fine; others are logged
			// and the rest of the characteristics still work.
			b.Log.Printf("remote: StartNotify %s: %v", path, err)
		}
	}
	defer func() {
		for path := range chars {
			b.Conn.Object("org.bluez", path).Call(ifChar+".StopNotify", 0)
		}
	}()
	b.Log.Printf("remote: connected (%s), listening on %d characteristics", dev, len(chars))

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case s, ok := <-sigs:
			if !ok {
				return errors.New("bluez: D-Bus connection closed")
			}
			n, drop, ok := parseSignal(s, dev, chars)
			if drop {
				return errors.New("remote disconnected")
			}
			if ok {
				emit(n)
			}
		}
	}
}

// waitResolved waits until BlueZ has read the remote's GATT database.
func (b *BlueZ) waitResolved(ctx context.Context, dev dbus.BusObject) error {
	deadline := time.Now().Add(b.ConnectTimeout)
	for {
		v, err := dev.GetProperty(ifDevice + ".ServicesResolved")
		if err == nil {
			if r, _ := v.Value().(bool); r {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return errors.New("bluez: connected, but services not resolved")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// findDevice picks the remote among BlueZ's objects: paired, and either the
// given address or one of the names. Only paired devices count, so a stranger
// advertising as "Sunday" next door is never used.
func findDevice(objs objects, addr string, names []string) (dbus.ObjectPath, error) {
	var best dbus.ObjectPath
	for path, ifs := range objs {
		p, ok := ifs[ifDevice]
		if !ok || !boolProp(p, "Paired") {
			continue
		}
		match := false
		if addr != "" {
			match = strings.EqualFold(strProp(p, "Address"), addr)
		} else {
			for _, n := range names {
				if strProp(p, "Name") == n || strProp(p, "Alias") == n {
					match = true
				}
			}
		}
		// Deterministic when there are two (two adapters, or two remotes).
		if match && (best == "" || path < best) {
			best = path
		}
	}
	if best == "" {
		if addr != "" {
			return "", fmt.Errorf("bluez: no paired device with that address; pair it with bluetoothctl")
		}
		return "", fmt.Errorf("bluez: no paired device named %s; pair it with bluetoothctl", strings.Join(names, " or "))
	}
	return best, nil
}

// findChars returns the characteristics under dev that carry keys and that
// can notify. f000b002 (audio) is left out on purpose.
func findChars(objs objects, dev dbus.ObjectPath) map[dbus.ObjectPath]Source {
	out := map[dbus.ObjectPath]Source{}
	prefix := string(dev) + "/"
	for path, ifs := range objs {
		c, ok := ifs[ifChar]
		if !ok || !strings.HasPrefix(string(path), prefix) || !canNotify(c) {
			continue
		}
		switch strings.ToLower(strProp(c, "UUID")) {
		case uuidReport:
			out[path] = Report
		case uuidVoice:
			out[path] = Voice
		}
	}
	return out
}

// parseSignal reads a PropertiesChanged signal: a new Value on one of our
// characteristics (notification) or Connected=false on the device (drop).
func parseSignal(s *dbus.Signal, dev dbus.ObjectPath, chars map[dbus.ObjectPath]Source) (n Notification, drop, ok bool) {
	if s.Name != ifProps+".PropertiesChanged" || len(s.Body) < 2 {
		return n, false, false
	}
	iface, _ := s.Body[0].(string)
	changed, _ := s.Body[1].(map[string]dbus.Variant)
	switch {
	case iface == ifDevice && s.Path == dev:
		if v, has := changed["Connected"]; has {
			if c, _ := v.Value().(bool); !c {
				return n, true, false
			}
		}
	case iface == ifChar:
		src, mine := chars[s.Path]
		v, has := changed["Value"]
		if !mine || !has {
			return n, false, false
		}
		b, isBytes := v.Value().([]byte)
		if !isBytes {
			return n, false, false
		}
		return Notification{From: src, Value: append([]byte(nil), b...)}, false, true
	}
	return n, false, false
}

func boolProp(p map[string]dbus.Variant, k string) bool {
	v, _ := p[k].Value().(bool)
	return v
}

func strProp(p map[string]dbus.Variant, k string) string {
	v, _ := p[k].Value().(string)
	return v
}

func canNotify(p map[string]dbus.Variant) bool {
	flags, _ := p["Flags"].Value().([]string)
	for _, f := range flags {
		if f == "notify" || f == "indicate" {
			return true
		}
	}
	return false
}
