#!/usr/bin/env bash

source "$(dirname "$0")/../helpers.sh"

begin_test "Client graceful shutdown"

start_server
start_client

wait_for_tunnel 15 || fail "tunnel did not establish"

client_ssh "sudo pkill -TERM obftun"
sleep 3

if client_ssh "pgrep obftun"; then
    fail "client process still running after SIGTERM"
fi

if client_ssh "grep -qiE 'panic|SIGSEGV|runtime error' ${CLIENT_LOG}"; then
    fail "client log contains crash output"
fi

sleep 2
server_ssh "grep -q 'disconnected' ${SERVER_LOG}" || fail "server did not log client disconnect"

stop_server
pass
