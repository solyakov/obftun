#!/bin/bash
# Usage: ifconfig-client.sh <interface_name>

set -e

declare -r tun_iface="$1"
declare -r tun_local_ip="10.10.0.2"  # OpenWrt tunnel endpoint
declare -r tun_remote_ip="10.10.0.1" # EC2 tunnel endpoint
declare -i tun_mask=24
declare -i tun_mtu=1420
declare -r wifi_iface="br-proxy" # OpenWrt LAN interface (adjust to your setup)
declare -ri fwmark=100
declare -ri tunnel_table=100

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

echo "Setting up iptables marking"

iptables -t mangle -D PREROUTING -i "$wifi_iface" -j MARK --set-mark "$fwmark" 2>/dev/null || true
iptables -t mangle -A PREROUTING -i "$wifi_iface" -j MARK --set-mark "$fwmark"

echo "Creating routing table entry"

if ! grep -q "^$tunnel_table.*tunnel" /etc/iproute2/rt_tables 2>/dev/null; then
    echo "$tunnel_table tunnel" >> /etc/iproute2/rt_tables
fi

echo "Setting up ip rules"

ip rule del fwmark "$fwmark" table "$tunnel_table" 2>/dev/null || true
ip rule add fwmark "$fwmark" table "$tunnel_table" prio 100

echo "Setting up routes in tunnel table"

ip route flush table "$tunnel_table" 2>/dev/null || true
ip route add default via "$tun_remote_ip" dev "$tun_iface" table "$tunnel_table"

ip route flush cache

echo "Setting up NAT"

iptables -t nat -D POSTROUTING -o "$tun_iface" -j MASQUERADE 2>/dev/null || true
iptables -t nat -A POSTROUTING -o "$tun_iface" -j MASQUERADE

echo "Setting up forwarding rules"

iptables -D FORWARD -i "$wifi_iface" -o "$tun_iface" -j ACCEPT 2>/dev/null || true
iptables -A FORWARD -i "$wifi_iface" -o "$tun_iface" -j ACCEPT

iptables -D FORWARD -i "$tun_iface" -o "$wifi_iface" -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT 2>/dev/null || true
iptables -A FORWARD -i "$tun_iface" -o "$wifi_iface" -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT

echo "Interface $tun_iface configured"
