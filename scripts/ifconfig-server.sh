#!/bin/bash
# Usage: ifconfig-server.sh <interface_name>

set -e

declare -r tun_iface="$1"
declare -r tun_local_ip="10.10.0.1"
declare -i tun_mask=24
declare -i tun_mtu=1420
declare -r external_iface="ens5" # Outgoing interface on the server

if [ -z "$tun_iface" ]; then
    echo "Usage: $0 <tun_iface>"
    exit 1
fi

echo "Configuring interface $tun_iface"

ip addr add "$tun_local_ip/$tun_mask" dev "$tun_iface"
ip link set "$tun_iface" mtu "$tun_mtu"
ip link set "$tun_iface" up

echo "Enabling IP forwarding"

echo 1 > /proc/sys/net/ipv4/ip_forward

echo "Setting up NAT"

iptables -t nat -D POSTROUTING -s "$tun_local_ip/$tun_mask" -o "$external_iface" -j MASQUERADE 2>/dev/null || true
iptables -t nat -A POSTROUTING -s "$tun_local_ip/$tun_mask" -o "$external_iface" -j MASQUERADE

echo "Setting up forwarding"

iptables -D FORWARD -i "$tun_iface" -o "$external_iface" -j ACCEPT 2>/dev/null || true
iptables -A FORWARD -i "$tun_iface" -o "$external_iface" -j ACCEPT

iptables -D FORWARD -i "$external_iface" -o "$tun_iface" -m state --state RELATED,ESTABLISHED -j ACCEPT 2>/dev/null || true
iptables -A FORWARD -i "$external_iface" -o "$tun_iface" -m state --state RELATED,ESTABLISHED -j ACCEPT

echo "Interface $tun_iface configured"