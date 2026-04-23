#!/usr/bin/env bash

source "$(dirname "$0")/../helpers.sh"

begin_test "L2 data flow through tunnel"

start_server
start_client

wait_for_tunnel 15 || fail "tunnel did not establish"

client_ssh "sudo ip addr add 10.10.0.2/16 dev br-client 2>/dev/null"
sleep 3

client_ssh "ping -c 3 -W 2 10.10.0.1" &>/dev/null || fail "client cannot ping server (10.10.0.1) through tunnel"
server_ssh "ping -c 3 -W 2 10.10.0.2" &>/dev/null || fail "server cannot ping client (10.10.0.2) through tunnel"

stop_all
client_ssh "sudo ip addr del 10.10.0.2/16 dev br-client 2>/dev/null"
pass
