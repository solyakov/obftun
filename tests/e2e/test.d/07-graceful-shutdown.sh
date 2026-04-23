#!/usr/bin/env bash

source "$(dirname "$0")/../helpers.sh"

begin_test "Graceful server shutdown"

start_server
start_client

wait_for_tunnel 15 || fail "tunnel did not establish"

server_ssh "sudo pkill -TERM obftun"
sleep 3

server_ssh "grep -q 'Bye bye' ${SERVER_LOG}" || fail "server log missing 'Bye bye!' (not graceful shutdown)"

if server_ssh "pgrep obftun"; then
    fail "server process still running after SIGTERM"
fi

if ! client_ssh "grep -qE 'Lost connection|SSE.*error|SSE.*closed|connection refused' ${CLIENT_LOG}"; then
    fail "client did not detect server shutdown"
fi

stop_client
pass
