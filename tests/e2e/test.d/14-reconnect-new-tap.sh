#!/usr/bin/env bash

source "$(dirname "$0")/../helpers.sh"

RECONNECT_COUNT=100

begin_test "Reconnect ${RECONNECT_COUNT} times: server survives, TAPs cleaned up"

start_server

for i in $(seq 1 ${RECONNECT_COUNT}); do
    server_ssh "sudo cp /dev/null ${SERVER_LOG}"

    client_ssh "sudo rm -f ${CLIENT_LOG}; nohup sudo obftun --secret '${SECRET}' --script /usr/local/bin/ifconfig-client.sh --dial '${SERVER_IP}:80' > ${CLIENT_LOG} 2>&1 &"

    wait_for_connect 10 || fail "iteration ${i}: tunnel did not connect"

    client_ssh "sudo pkill -9 obftun"

    wait_for_disconnect 10 || fail "iteration ${i}: tunnel did not disconnect"

    if [ $((i % 10)) -eq 0 ]; then
        tap_count=$(server_ssh "ip link show type tun 2>/dev/null | grep -c 'tap'" 2>/dev/null)
        if [ "${tap_count:-0}" -gt 0 ]; then
            fail "iteration ${i}: found ${tap_count} stale TAP interface(s) on server"
        fi
        echo "  ... ${i}/${RECONNECT_COUNT} reconnects OK, 0 stale TAPs"
    fi
done

server_ssh "pgrep obftun >/dev/null" || fail "server process died during reconnect storm"

stop_all
pass
