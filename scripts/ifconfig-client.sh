#!/bin/bash
#
# Usage: ifconfig-client.sh <interface_name> <up|down> [peer_addr]
#
# Network Architecture (OpenWrt Client):
# ┌─────────────────────────────────────────────────┐
# │                OpenWrt Router                   │
# │                                                 │
# │  ┌──────────────────────────────────────────┐   │
# │  │           Bridge (br-client)             │   │
# │  │         (no IP, pure L2 switch)          │   │
# │  │         pre-configured by admin          │   │
# │  └───┬──────────────────────────────────┬───┘   │
# │      │                                  │       │
# │  ┌───┴────┐                        ┌────┴────┐  │
# │  │  WIFI  │                        │   TAP   │  │
# │  │phy1-ap0│                        │  (tap0) │  │
# │  │(static)│                        │(dynamic)│  │
# │  └───┬────┘                        └────┬────┘  │
# │      │                                  │       │
# └──────┼──────────────────────────────────┼───────┘
#        │                                  |
#   WiFi Clients                     Tunnel to Server

set -e

declare -r tap_iface="$1"
declare -r action="$2"
declare -r peer_addr="$3"

if [ -z "$tap_iface" ] || [ -z "$action" ]; then
    echo "Usage: $0 <tap_iface> <up|down> [peer_addr]"
    exit 1
fi

declare -r bridge_name="br-client"

action_up() {
    echo "Attaching interface ${tap_iface} to bridge ${bridge_name}"
    
    if [ -n "$peer_addr" ]; then
        ip link set "$tap_iface" alias "$peer_addr" 2>/dev/null || true
    fi
    
    ip link set "$tap_iface" up
    ip link set "$tap_iface" master "$bridge_name"
    
    echo "Interface ${tap_iface} attached to bridge ${bridge_name}"
}

action_down() {
    echo "Detaching interface ${tap_iface} from bridge ${bridge_name}"
    
    ip link set "$tap_iface" nomaster 2>/dev/null || true
    ip link set "$tap_iface" down 2>/dev/null || true
    
    echo "Interface ${tap_iface} detached from bridge ${bridge_name}"
}

case "$action" in
    "up")
        action_up
        ;;
    "down")
        action_down
        ;;
    *)
        echo "Unknown action: $action"
        exit 1
        ;;
esac
