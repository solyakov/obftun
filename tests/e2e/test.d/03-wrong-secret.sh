#!/usr/bin/env bash

source "$(dirname "$0")/../helpers.sh"

begin_test "Wrong secret rejected"

start_server

client_ssh "sudo rm -f ${CLIENT_LOG}; nohup sudo obftun --secret 'wrong-secret' --script /usr/local/bin/ifconfig-client.sh --dial '${SERVER_IP}:80' --verbose > ${CLIENT_LOG} 2>&1 &"
sleep 5

if server_ssh "grep -q 'connected (interface' ${SERVER_LOG}"; then
    fail "server should NOT have accepted client with wrong secret"
fi

if server_ssh "ip link show tap0 2>/dev/null"; then
    fail "server should NOT have created tap0 for wrong-secret client"
fi

stop_all
pass
