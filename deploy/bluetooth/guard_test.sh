#!/bin/sh
# Tests tvcast-bt-guard with fake systemctl/rfkill and a temporary root.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
T=$(mktemp -d)
trap 'rm -rf "$T"' EXIT
sed -e "s|/var/lib/tvcast-bt-guard|$T/state|; s|/etc/tvcast-bt-guard.enable|$T/enable|" \
	"$here/tvcast-bt-guard" >"$T/g"
mkdir "$T/bin"
printf '#!/bin/sh\necho "$(basename $0) $*" >>"%s/calls"\n' "$T" >"$T/bin/systemctl"
cp "$T/bin/systemctl" "$T/bin/rfkill"
chmod +x "$T/bin/systemctl" "$T/bin/rfkill"
PATH="$T/bin:$PATH"
g() { sh "$T/g" "$@" >/dev/null; }
fail() { echo "FAIL: $*" >&2; exit 1; }

g start; [ ! -e "$T/state" ] || fail "acts while not enabled"
touch "$T/enable"
g start; g stop; g start; g stop          # clean reboots
[ "$(cat "$T/state/unclean")" = 0 ] || fail "clean reboots counted"
g start; g start                          # one hang
[ ! -e "$T/calls" ] || fail "tripped after one unclean boot"
g stop; g start; g start                  # clean shutdown resets; then one more hang
[ ! -e "$T/calls" ] || fail "count not reset by a clean shutdown"
g start                                   # second hang in a row
grep -q "systemctl disable bluetooth.service tvcast-remote.service" "$T/calls" || fail "not disabled"
grep -q "rfkill block bluetooth" "$T/calls" || fail "radio not blocked"
[ ! -e "$T/enable" ] || fail "guard still enabled after tripping"
echo "guard_test: ok"
