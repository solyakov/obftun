#!/usr/bin/env bash

source "$(dirname "$0")/../helpers.sh"

begin_test "Max clients enforcement"

start_server "--max-clients 1"

server_ssh "nohup curl -sS -N 'http://localhost:80/api/feed?t=$(generate_feed_token)' >/dev/null 2>&1 &"

wait_for_connect 15 || fail "first authenticated feed did not connect"

status=$(server_ssh "curl -s -m 5 -o /dev/null -w '%{http_code}' 'http://localhost:80/api/feed?t=$(generate_feed_token)' 2>/dev/null")
if [ "${status}" != "503" ]; then
    fail "extra authenticated client got HTTP ${status}, expected 503"
fi

stop_all
pass