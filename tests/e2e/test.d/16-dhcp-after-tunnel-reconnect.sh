#!/usr/bin/env bash

source "$(dirname "$0")/../helpers.sh"

begin_test "DHCP works after tunnel reconnect"

start_dnsmasq
start_server
start_client

wait_for_connect 15 || fail "initial tunnel did not establish"

create_wifi_client
sleep 2

wifi_dhcp_request

ip_before=$(wifi_client_ip)
[ -n "${ip_before}" ] || fail "veth-dev did not get initial IP"
echo "  IP before reconnect: ${ip_before}"

client_ssh "ping -c 2 -W 2 -I veth-dev 10.10.0.1" &>/dev/null || fail "initial ping failed"

# Kill tunnel client. The WiFi client (veth pair) stays alive with its IP —
# just like a real phone that has no idea the tunnel dropped.
client_ssh "sudo pkill -9 obftun"
sleep 3

# Reconnect the tunnel.
server_ssh "sudo cp /dev/null ${SERVER_LOG}"
start_client

wait_for_connect 15 || fail "tunnel did not reconnect"

# The phone still has its IP. It just keeps sending packets.
# Verify traffic flows again through the restored tunnel.
sleep 2
client_ssh "ping -c 3 -W 2 -I veth-dev 10.10.0.1" &>/dev/null || fail "ping failed after tunnel reconnect"

ip_after=$(wifi_client_ip)
echo "  IP after reconnect: ${ip_after}"

destroy_wifi_client
stop_all
stop_dnsmasq
pass
