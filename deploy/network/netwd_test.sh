#!/bin/sh
# Tests tvcast-netwd with fake ip/iw/ping/systemctl/journalctl.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
T=$(mktemp -d)
trap 'rm -rf "$T"' EXIT
mkdir "$T/bin"
# The fakes read the scenario from files in $T.
cat >"$T/bin/ip" <<F
#!/bin/sh
case "\$*" in
*addr*) [ -e "$T/has_ip" ] && echo "3: wlan0    inet 192.168.1.50/24 brd 192.168.1.255 scope global wlan0" ;;
*route*) [ -e "$T/has_ip" ] && echo "default via 192.168.1.1 dev wlan0 proto dhcp" ;;
esac
exit 0
F
cat >"$T/bin/iw" <<F
#!/bin/sh
[ -e "$T/ap" ] && printf 'Interface wlan0\n\tssid Tele\n\ttype AP\n' || printf 'Interface wlan0\n\ttype managed\n'
F
cat >"$T/bin/ping" <<F
#!/bin/sh
[ -e "$T/gw_ok" ]
F
printf '#!/bin/sh\necho "$*" >>"%s/reboots_done"\n' "$T" >"$T/bin/systemctl"
printf '#!/bin/sh\nexit 0\n' >"$T/bin/journalctl"
chmod +x "$T/bin/"*
export PATH="$T/bin:$PATH" RUN="$T/run" STATE="$T/state" UPTIME_FILE="$T/uptime" FAILS=3 MIN_UPTIME=300 MAX_REBOOTS=2
fail() { echo "FAIL: $*" >&2; exit 1; }
check() { sh "$here/tvcast-netwd" >/dev/null; }
reboots() { [ -e "$T/reboots_done" ] && wc -l <"$T/reboots_done" | tr -d ' ' || echo 0; }
boot() { rm -rf "$T/run"; } # /run is cleared at every boot

echo 100.5 >"$T/uptime"
check; check; check; check
[ "$(reboots)" = 0 ] || fail "acted before MIN_UPTIME"

echo 400.0 >"$T/uptime"
touch "$T/has_ip" "$T/gw_ok"
check; check; check; check
[ "$(reboots)" = 0 ] || fail "rebooted with the network OK"

rm "$T/gw_ok"                                 # joined, but the gateway is gone
check; check
[ "$(reboots)" = 0 ] || fail "rebooted before FAILS"
touch "$T/gw_ok"; check                       # it came back: count resets
rm "$T/gw_ok"; check; check
[ "$(reboots)" = 0 ] || fail "count not reset when the network came back"
check
[ "$(reboots)" = 1 ] || fail "no reboot after FAILS in a row"

boot; rm "$T/has_ip"                          # still dead after the reboot
check; check; check
[ "$(reboots)" = 2 ] || fail "second reboot missing"
boot; check; check; check; check; check
[ "$(reboots)" = 2 ] || fail "rebooted more than MAX_REBOOTS"

touch "$T/has_ip" "$T/ap"                     # AP mode ("Tele"): no gateway needed
check
[ "$(cat "$T/state/reboots")" = 0 ] || fail "reboot count not reset in AP mode"
rm "$T/ap"; check; check; check
[ "$(reboots)" = 3 ] || fail "count did not start again after recovering"

TVCAST_NETWD_DISABLE=1 check; boot; TVCAST_NETWD_DISABLE=1 check; TVCAST_NETWD_DISABLE=1 check; TVCAST_NETWD_DISABLE=1 check
[ "$(reboots)" = 3 ] || fail "acted while disabled"
echo "netwd_test: ok"
