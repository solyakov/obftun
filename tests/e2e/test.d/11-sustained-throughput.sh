#!/usr/bin/env bash

source "$(dirname "$0")/../helpers.sh"

begin_test "Sustained data throughput"

start_server
start_client

wait_for_tunnel 15 || fail "tunnel did not establish"

client_ssh "sudo ip addr add 10.10.0.2/16 dev br-client 2>/dev/null"
sleep 3

# Verify basic connectivity first.
client_ssh "ping -c 2 -W 2 10.10.0.1" &>/dev/null || fail "ping failed before throughput test"

# Generate a 1MB random file on the server and compute its checksum.
server_ssh "dd if=/dev/urandom of=/tmp/testdata bs=1024 count=1024 2>/dev/null"
srv_md5=$(server_ssh "md5sum /tmp/testdata | awk '{print \$1}'")

# Serve the file via netcat on the server (tunnel IP).
server_ssh "nohup bash -c 'nc -l -p 9999 -q 1 < /tmp/testdata' > /dev/null 2>&1 &"
sleep 1

# Receive on the client and compute checksum.
cli_md5=$(client_ssh "nc -w 5 10.10.0.1 9999 | md5sum | awk '{print \$1}'")

if [ -z "${cli_md5}" ]; then
    fail "client received no data via netcat"
fi

if [ "${srv_md5}" != "${cli_md5}" ]; then
    fail "checksum mismatch: server=${srv_md5} client=${cli_md5}"
fi

# Cleanup.
server_ssh "rm -f /tmp/testdata"
stop_all
client_ssh "sudo ip addr del 10.10.0.2/16 dev br-client 2>/dev/null"
pass
