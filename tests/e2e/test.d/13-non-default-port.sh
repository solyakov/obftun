#!/usr/bin/env bash

source "$(dirname "$0")/../helpers.sh"

begin_test "Server on non-default port"

# Start server on port 8080 instead of default 80.
server_ssh "sudo rm -f ${SERVER_LOG}; nohup sudo obftun --secret '${SECRET}' --script /usr/local/bin/ifconfig-server.sh --bind :8080 --verbose > ${SERVER_LOG} 2>&1 &"
sleep 2

# Start client dialing port 8080.
client_ssh "sudo rm -f ${CLIENT_LOG}; nohup sudo obftun --secret '${SECRET}' --script /usr/local/bin/ifconfig-client.sh --dial '${SERVER_IP}:8080' --verbose > ${CLIENT_LOG} 2>&1 &"
sleep 2

wait_for_tunnel 15 || fail "tunnel did not establish on port 8080"

client_ssh "sudo ip addr add 10.10.0.2/16 dev br-client 2>/dev/null"
sleep 3

client_ssh "ping -c 3 -W 2 10.10.0.1" &>/dev/null || fail "ping failed through tunnel on port 8080"

stop_all
client_ssh "sudo ip addr del 10.10.0.2/16 dev br-client 2>/dev/null"
pass
