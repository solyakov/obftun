#!/bin/bash
#
# Usage: ifconfig-client.sh <interface_name> <up|down> [peer_addr]
#
# Network Architecture (OpenWrt Client):
#
#     Tunnel to Server
#            |
# ┌──────────┼──────────────────────────────────────┐
# │          │       OpenWrt Router                 │
# │          │                                      │
# │  ┌───────┴───────┐                              │
# │  │ obftun-client │                              │
# │  └───────┬───────┘                              │
# │          │                                      │
# |         tap0                                    │
# |          │                                      │
# |  ┌───────┴────────┐                             │
# |  │ Bridge (no IP) |                             │
# |  └───────┬────────┘                             │
# |          │                                      │
# |      phy1-ap0                                   │
# └──────────┼──────────────────────────────────────┘
#            |
#      5G WiFi Clients

set -e

declare -r tap_iface="$1" # tap0
declare -r action="$2"    # up|down
declare -r peer_addr="$3" # 1.2.3.4:5678

if [ -z "$tap_iface" ] || [ -z "$action" ]; then
    echo "Usage: $0 <tap_iface> <up|down> [peer_addr]"
    exit 1
fi

declare -r bridge_name="br-client"

action_up() {
    echo "Attaching interface ${tap_iface} to bridge ${bridge_name}"
    
    if [ -n "$peer_addr" ]; then
        # Set the server address as an alias to the interface.
        # Makes little sense for the client, but it is here for completeness.
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
