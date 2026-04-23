#!/usr/bin/env bash

source "$(dirname "$0")/../helpers.sh"

begin_test "Tunnel cleanup on disconnect"

start_server
start_client

wait_for_tunnel 15 || fail "tunnel did not establish"

server_ssh "ip link show tap0 2>/dev/null" >/dev/null || fail "tap0 missing on server before disconnect"
client_ssh "ip link show tap0 2>/dev/null" >/dev/null || fail "tap0 missing on client before disconnect"

client_ssh "sudo pkill -9 obftun"
sleep 5

if server_ssh "ip link show tap0 2>/dev/null"; then
    fail "server tap0 still exists after client disconnect"
fi

if server_ssh "bridge link show 2>/dev/null | grep -q tap0"; then
    fail "tap0 still in server bridge after client disconnect"
fi

if client_ssh "ip link show tap0 2>/dev/null"; then
    fail "client tap0 still exists after client kill"
fi

stop_server
pass
