#!/usr/bin/env bash

source "$(dirname "$0")/../helpers.sh"

begin_test "DHCP client gets IP through tunnel"

start_dnsmasq
start_server
start_client

wait_for_connect 15 || fail "tunnel did not establish"

create_wifi_client
sleep 2

wifi_dhcp_request

ip=$(wifi_client_ip)
[ -n "${ip}" ] || fail "veth-dev did not get an IP address"

# Verify the IP is in the expected DHCP range (10.10.x.x).
if ! echo "${ip}" | grep -qE '^10\.10\.'; then
    fail "IP ${ip} is not in the 10.10.0.0/16 range"
fi
echo "  veth-dev got IP: ${ip}"

# Verify the simulated WiFi client can ping the server gateway.
client_ssh "ping -c 3 -W 2 -I veth-dev 10.10.0.1" &>/dev/null || fail "cannot ping server gateway 10.10.0.1 from veth-dev"

server_ssh "test -s ${DNSMASQ_LEASES}" || fail "dnsmasq lease file is empty"

destroy_wifi_client
stop_all
stop_dnsmasq
pass
