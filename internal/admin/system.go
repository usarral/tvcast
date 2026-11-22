package admin

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Verbs are the privileged actions tvcast may ask systemd to run as root,
// one per tvcast-admin@<verb>.service instance (deploy/admin). The script
// on the box checks the same list.
var Verbs = []string{"wifi-home", "wifi-tele", "new-code", "remote-pair",
	"lang-es", "lang-en", "restart-tvcast", "reboot", "poweroff", "console"}

// logUnits are the journal units behind each "log:..." item.
var logUnits = map[string][]string{
	"tvcast": {"-u", "tvcast"},
	"wifi":   {"-u", "NetworkManager", "-u", "hostapd", "-u", "tele-ap", "-u", "tele-dhcp", "-u", "tele-auto"},
	"remote": {"-u", "tvcast-remote", "-u", "bluetooth"},
	"admin":  {"-u", "tvcast-admin@*"},
	"kernel": {"-k", "-b"},
}

// Systemd is the real System: `systemctl start` for actions (allowed to the
// tvcast user by a polkit rule, so no sudo, and it works with
// NoNewPrivileges) and journalctl for logs (needs the systemd-journal group).
type Systemd struct{}

func validVerb(v string) bool {
	for _, x := range Verbs {
		if x == v {
			return true
		}
	}
	return false
}

func (Systemd) Run(verb string) error {
	if !validVerb(verb) {
		return fmt.Errorf("unknown action %q", verb)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "systemctl", "start", "--no-block", "tvcast-admin@"+verb+".service").CombinedOutput()
	if err != nil {
		return errors.New(firstLine(string(out), err.Error()))
	}
	return nil
}

func (Systemd) Log(which string, n int) ([]string, error) {
	u, ok := logUnits[which]
	if !ok {
		return nil, fmt.Errorf("unknown log %q", which)
	}
	args := append([]string{"--no-pager", "-q", "-o", "cat", "-n", strconv.Itoa(n)}, u...)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "journalctl", args...).CombinedOutput()
	lines := splitLines(string(out))
	if err != nil {
		return lines, errors.New(firstLine(string(out), err.Error()))
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}

func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if l != "" {
			out = append(out, strings.ReplaceAll(l, "\t", "  "))
		}
	}
	return out
}

func firstLine(s, def string) string {
	if l := splitLines(s); len(l) > 0 {
		return l[0]
	}
	return def
}
