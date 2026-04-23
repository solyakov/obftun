#!/usr/bin/env bash

source "$(dirname "$0")/../helpers.sh"

begin_test "Root endpoint returns dynamic JSON envelope"

start_server

response=$(server_ssh "curl -s http://localhost:80/ 2>/dev/null")
[ -n "${response}" ] || fail "curl got empty response"

if ! echo "${response}" | grep -q '"status":"ok"'; then
    fail "response missing status:ok"
fi
if ! echo "${response}" | grep -q '"request_id"'; then
    fail "response missing request_id"
fi

# Verify the envelope is dynamic (request_id differs across calls).
id1=$(server_ssh "curl -s http://localhost:80/ 2>/dev/null" | sed -n 's/.*"request_id":"\([^"]*\)".*/\1/p')
id2=$(server_ssh "curl -s http://localhost:80/ 2>/dev/null" | sed -n 's/.*"request_id":"\([^"]*\)".*/\1/p')
if [ -z "${id1}" ] || [ -z "${id2}" ] || [ "${id1}" = "${id2}" ]; then
    fail "request_id not dynamic (id1='${id1}' id2='${id2}')"
fi

stop_server
pass
