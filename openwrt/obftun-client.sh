#!/bin/bash
#
# obftun client wrapper for OpenWrt
# This script is called by /etc/init.d/obftund
#

set -e

cd "$(dirname "$0")"

declare -r bridge="br-client"
declare -r wifi_iface="phy1-ap0"

function setup_bridge_if_not_exists() {
    if ip link show "$bridge" > /dev/null 2>&1; then
        echo "Bridge $bridge already exists, skipping setup"
        return
    fi
    
    ip link add name "$bridge" type bridge
    ip link set "$bridge" mtu 1420
    ip link set "$bridge" up
    
    ip link set "$wifi_iface" master "$bridge"
    ip link set "$wifi_iface" up
    
    echo "Bridge $bridge configured with $wifi_iface"
}

declare -r dial="<server-ip>:8443"
declare -r obftun_bin="./obftun"
declare -r script="./ifconfig-client.sh"
declare -r certificate="./client.crt"
declare -r key="./client.key"
declare -r ca="./ca.crt"

function run_client_forever() {
    while true; do
        echo "Connecting to $dial"
        "$obftun_bin" \
            --dial "$dial" \
            --script "$script" \
            --certificate "$certificate" \
            --key "$key" \
            --ca "$ca"
    done
}

setup_bridge_if_not_exists
run_client_forever
