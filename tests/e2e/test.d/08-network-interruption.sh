#!/usr/bin/env bash

source "$(dirname "$0")/../helpers.sh"

begin_test "Client survives brief network interruption"

start_server
start_client

wait_for_tunnel 15 || fail "tunnel did not establish"

client_ssh "sudo ip addr add 10.10.0.2/16 dev br-client 2>/dev/null"
sleep 3

client_ssh "ping -c 2 -W 2 10.10.0.1" &>/dev/null || fail "initial ping failed before interruption"

server_ssh "sudo iptables -A INPUT -p tcp --dport 80 -j DROP; sudo iptables -A OUTPUT -p tcp --sport 80 -j DROP"
sleep 10
server_ssh "sudo iptables -D INPUT -p tcp --dport 80 -j DROP; sudo iptables -D OUTPUT -p tcp --sport 80 -j DROP"

sleep 15
server_ssh "sudo ip neigh flush dev br-obftun 2>/dev/null"
client_ssh "sudo ip neigh flush dev br-client 2>/dev/null"
sleep 3

client_ssh "ping -c 5 -W 3 10.10.0.1" &>/dev/null || fail "ping failed after network recovery"

stop_all
client_ssh "sudo ip addr del 10.10.0.2/16 dev br-client 2>/dev/null"
pass
