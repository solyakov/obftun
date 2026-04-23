#!/usr/bin/env bash

source "$(dirname "$0")/../helpers.sh"

begin_test "Tunnel establishment"

start_server
start_client

wait_for_tunnel 15 || fail "tunnel did not establish"

server_ssh "ip link show tap0 >/dev/null 2>&1" || fail "tap0 not found on server"
client_ssh "ip link show tap0 >/dev/null 2>&1" || fail "tap0 not found on client"

if ! server_ssh "bridge link show 2>/dev/null | grep -q 'tap0.*br-obftun'"; then
    fail "tap0 not attached to br-obftun on server"
fi

if ! client_ssh "bridge link show 2>/dev/null | grep -q 'tap0.*br-client'"; then
    fail "tap0 not attached to br-client on client"
fi

stop_all
pass
