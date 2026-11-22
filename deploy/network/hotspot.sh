#!/bin/sh
# Crea el punto de acceso "Tele" con NetworkManager (DHCP y DNS los pone NM
# con dnsmasq). Comprueba antes que el chip soporta AP (INSTALL.md, paso 3).
#
# Arranca solo: si la caja ve una wifi conocida con más prioridad (la de tu
# casa) se conecta a ella; si no (la casa sin wifi) levanta el punto de acceso.
#
# Uso: sudo ./hotspot.sh "Tele" "contraseña-de-8-o-mas"
set -eu
SSID=${1:?SSID}
PASS=${2:?contraseña}
IFACE=${IFACE:-wlan0}
[ ${#PASS} -ge 8 ] || { echo "La contraseña necesita 8 caracteres o más"; exit 1; }

nmcli connection delete tvcast-ap >/dev/null 2>&1 || true
nmcli connection add type wifi ifname "$IFACE" con-name tvcast-ap ssid "$SSID" \
  autoconnect yes connection.autoconnect-priority -10 \
  802-11-wireless.mode ap 802-11-wireless.band bg 802-11-wireless.channel 6 \
  802-11-wireless.powersave 2 \
  ipv4.method shared ipv4.addresses 10.42.0.1/24 ipv6.method disabled \
  wifi-sec.key-mgmt wpa-psk wifi-sec.proto rsn wifi-sec.pairwise ccmp wifi-sec.group ccmp \
  wifi-sec.psk "$PASS"

# Que la wifi de casa (si existe) gane al punto de acceso.
for c in $(nmcli -t -f NAME,TYPE connection show | awk -F: '$2=="802-11-wireless" && $1!="tvcast-ap"{print $1}'); do
  nmcli connection modify "$c" connection.autoconnect-priority 10
done

echo "Creado. Probar ya:   sudo nmcli connection up tvcast-ap   (perderás el SSH por la wifi de casa)"
echo "Volver a casa:       sudo nmcli connection down tvcast-ap"
