#!/bin/sh
# Creates the "Tele" access point with NetworkManager (NM provides DHCP and
# DNS with dnsmasq). Check first that the chip supports AP (INSTALL.md, step 3).
#
# NOTE: with the Sunday Box's BCM4334, NM in AP mode + WPA2 did NOT work
# (phones never finished the password); hostapd does. See docs/LESSONS.md
# and TODO.md. Kept for chips where NM works.
#
# Starts by itself: if the box sees a known wifi with higher priority (your
# home's) it joins it; otherwise (house without wifi) it brings up the AP.
#
# Usage: sudo ./hotspot.sh "Tele" "password-of-8-or-more"
set -eu
SSID=${1:?SSID}
PASS=${2:?password}
IFACE=${IFACE:-wlan0}
[ ${#PASS} -ge 8 ] || { echo "The password needs 8 characters or more"; exit 1; }

nmcli connection delete tvcast-ap >/dev/null 2>&1 || true
nmcli connection add type wifi ifname "$IFACE" con-name tvcast-ap ssid "$SSID" \
  autoconnect yes connection.autoconnect-priority -10 \
  802-11-wireless.mode ap 802-11-wireless.band bg 802-11-wireless.channel 6 \
  802-11-wireless.powersave 2 \
  ipv4.method shared ipv4.addresses 10.42.0.1/24 ipv6.method disabled \
  wifi-sec.key-mgmt wpa-psk wifi-sec.proto rsn wifi-sec.pairwise ccmp wifi-sec.group ccmp \
  wifi-sec.psk "$PASS"

# Let the home wifi (if any) win over the access point.
for c in $(nmcli -t -f NAME,TYPE connection show | awk -F: '$2=="802-11-wireless" && $1!="tvcast-ap"{print $1}'); do
  nmcli connection modify "$c" connection.autoconnect-priority 10
done

echo "Created. Try it now:  sudo nmcli connection up tvcast-ap   (you will lose SSH over the home wifi)"
echo "Back to home wifi:   sudo nmcli connection down tvcast-ap"
