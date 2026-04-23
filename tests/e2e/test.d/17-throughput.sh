#!/usr/bin/env bash

source "$(dirname "$0")/../helpers.sh"

begin_test "Throughput measurement (iperf3)"

MIN_BYTES=$((1 << 20))  # 1 MiB in obftun server log during a 10s run.
DURATION=10

start_server
start_client
wait_for_tunnel 15 || fail "tunnel did not establish"

client_ssh "sudo ip addr add 10.10.0.2/16 dev br-client 2>/dev/null"
sleep 3
client_ssh "ping -c 2 -W 2 10.10.0.1" &>/dev/null || fail "ping failed before throughput test"

logged_bytes() {
    server_ssh "grep -oP '\\[\\K[0-9]+(?=\\])' ${SERVER_LOG} | awk '{s+=\$1} END {print s+0}'"
}

measure() {
    local label="$1"
    local reverse="$2"
    local before after bytes

    before=$(logged_bytes)

    server_ssh "sudo pkill iperf3 2>/dev/null; nohup iperf3 -s -1 -B 10.10.0.1 > /tmp/iperf3-srv.log 2>&1 &"
    sleep 1

    echo "--- ${label} ---"
    client_ssh "iperf3 -c 10.10.0.1 -t ${DURATION} ${reverse}" || fail "${label}: iperf3 failed"

    after=$(logged_bytes)
    bytes=$((after - before))
    echo "  obftun logged ${bytes} B through tunnel"
    [ "${bytes}" -lt "${MIN_BYTES}" ] && fail "${label}: only ${bytes} B via tunnel (min ${MIN_BYTES})"
}

measure "Upload (client => server)"   ""
measure "Download (server => client)" "-R"

server_ssh "sudo pkill iperf3 2>/dev/null" || true
stop_all
client_ssh "sudo ip addr del 10.10.0.2/16 dev br-client 2>/dev/null"
pass
