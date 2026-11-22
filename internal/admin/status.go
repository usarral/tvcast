package admin

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/usarral/tvcast/internal/i18n"
)

// Info is the raw material of the status lines.
type Info struct {
	Version   string
	IP        string
	Network   string // from NetworkMode
	Photos    int
	FreeBytes int64 // -1 = unknown
	Uptime    time.Duration
	Remote    Remote
	Now       time.Time
}

// StatusLines turns Info into the lines shown at the top of the panel.
func StatusLines(in Info, lang i18n.Lang) []string {
	T := func(k string) string { return i18n.T(lang, k) }
	free := "?"
	if in.FreeBytes >= 0 {
		free = fmt.Sprintf("%.1f GB", float64(in.FreeBytes)/(1<<30))
	}
	remote := T("adm.st_remote_never")
	switch {
	case in.Remote.Connected && in.Remote.Battery >= 0:
		remote = fmt.Sprintf(T("adm.st_remote_bat"), in.Remote.Battery)
	case in.Remote.Connected:
		remote = T("adm.st_remote_on")
	case !in.Remote.Seen.IsZero():
		remote = fmt.Sprintf(T("adm.st_remote_off"), short(in.Now.Sub(in.Remote.Seen)))
	}
	return []string{
		fmt.Sprintf(T("adm.st_net"), in.IP, in.Network),
		fmt.Sprintf(T("adm.st_photos"), in.Photos, free),
		fmt.Sprintf(T("adm.st_uptime"), short(in.Uptime)),
		remote,
		fmt.Sprintf(T("adm.st_version"), in.Version),
	}
}

// short formats a duration as "2 h 05 min" or "7 min".
func short(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h, m := int(d.Hours()), int(d.Minutes())%60
	if h > 0 {
		return fmt.Sprintf("%d h %02d min", h, m)
	}
	return fmt.Sprintf("%d min", m)
}

// NetworkMode reads `iw dev <iface> info`: the box's own wifi ("AP") or the
// home wifi it joined ("ssid ...").
func NetworkMode(iwInfo string) (mode, ssid string) {
	sc := bufio.NewScanner(strings.NewReader(iwInfo))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) >= 2 && f[0] == "ssid" {
			ssid = strings.Join(f[1:], " ")
		}
		if len(f) >= 2 && f[0] == "type" {
			mode = f[1]
		}
	}
	return mode, ssid
}

// NetworkLabel is the human version of NetworkMode, run against the system.
func NetworkLabel(iface string, lang i18n.Lang) string {
	if iface == "" {
		iface = "wlan0"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "iw", "dev", iface, "info").Output()
	if err != nil {
		return "?"
	}
	mode, ssid := NetworkMode(string(out))
	switch {
	case mode == "AP":
		return fmt.Sprintf(i18n.T(lang, "adm.st_net_ap"), ssid)
	case ssid != "":
		return fmt.Sprintf(i18n.T(lang, "adm.st_net_home"), ssid)
	}
	return i18n.T(lang, "adm.st_net_none")
}

// Uptime reads /proc/uptime (the clock may be wrong without NTP; this isn't).
func Uptime() time.Duration {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	f := strings.Fields(string(b))
	if len(f) == 0 {
		return 0
	}
	s, _ := strconv.ParseFloat(f[0], 64)
	return time.Duration(s * float64(time.Second))
}

// FreeBytes is the free space where the photos live.
func FreeBytes(dir string) int64 {
	var s syscall.Statfs_t
	if syscall.Statfs(dir, &s) != nil {
		return -1
	}
	return int64(s.Bavail) * int64(s.Bsize)
}
