#!/usr/bin/env bash

source "$(dirname "$0")/../helpers.sh"

begin_test "Client reconnection after server restart"

start_server
start_client

wait_for_tunnel 15 || fail "initial tunnel did not establish"

server_ssh "sudo pkill -9 obftun"
sleep 3

server_ssh "sudo rm -f ${SERVER_LOG}; nohup sudo obftun --secret '${SECRET}' --script /usr/local/bin/ifconfig-server.sh --bind :80 --verbose > ${SERVER_LOG} 2>&1 &"
sleep 2

wait_for_tunnel 30 || fail "client did not reconnect after server restart"

client_ssh "sudo ip addr add 10.10.0.2/16 dev br-client 2>/dev/null"
sleep 5

client_ssh "sudo ip neigh flush dev br-client 2>/dev/null"
server_ssh "sudo ip neigh flush dev br-obftun 2>/dev/null"
sleep 2

client_ssh "ping -c 5 -W 3 10.10.0.1" &>/dev/null || fail "ping failed after reconnect"

stop_all
client_ssh "sudo ip addr del 10.10.0.2/16 dev br-client 2>/dev/null"
pass
