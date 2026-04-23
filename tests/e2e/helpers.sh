#!/usr/bin/env bash
#
# helpers.sh — shared functions for e2e tests
#
# Sourced by test.sh and individual test scripts.

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
VAGRANT="vagrant"
VAGRANT_DIR="${REPO_ROOT}"

SERVER_LOG="/tmp/obftun-server.log"
CLIENT_LOG="/tmp/obftun-client.log"

SECRET="e2e-test-secret"

# --- SSH wrappers ---

server_ssh() {
    cd "${VAGRANT_DIR}" && ${VAGRANT} ssh server -c "$1"
}

client_ssh() {
    cd "${VAGRANT_DIR}" && ${VAGRANT} ssh client -c "$1"
}

discover_ips() {
    local i
    for i in 1 2 3; do
        SERVER_IP=$(server_ssh "hostname -I | awk '{print \$1}'")
        CLIENT_IP=$(client_ssh "hostname -I | awk '{print \$1}'")
        if [ -n "${SERVER_IP}" ] && [ -n "${CLIENT_IP}" ]; then
            export SERVER_IP CLIENT_IP
            echo "  Server IP: ${SERVER_IP}"
            echo "  Client IP: ${CLIENT_IP}"
            return 0
        fi
        sleep $i
    done
    echo "ERROR: Could not discover VM IPs after 3 attempts."
    return 1
}

rebuild() {
    echo "Syncing and rebuilding on both VMs..."
    cd "${VAGRANT_DIR}"
    ${VAGRANT} rsync server
    ${VAGRANT} rsync client
    server_ssh "cd /vagrant && sudo /usr/local/go/bin/go build -o /usr/local/bin/obftun ./cmd/obftun/"
    client_ssh "cd /vagrant && sudo /usr/local/go/bin/go build -o /usr/local/bin/obftun ./cmd/obftun/"
    echo "Build complete."
}

start_server() {
    local extra_args="${1:-}"
    server_ssh "sudo rm -f ${SERVER_LOG}; nohup sudo obftun --secret '${SECRET}' --script /usr/local/bin/ifconfig-server.sh --bind :80 --verbose ${extra_args} > ${SERVER_LOG} 2>&1 &"
    sleep 2
}

start_client() {
    local extra_args="${1:-}"
    client_ssh "sudo rm -f ${CLIENT_LOG}; nohup sudo obftun --secret '${SECRET}' --script /usr/local/bin/ifconfig-client.sh --dial '${SERVER_IP}:80' --verbose ${extra_args} > ${CLIENT_LOG} 2>&1 &"
    sleep 2
}

stop_server() {
    server_ssh "sudo pkill -TERM obftun"
    sleep 1
}

stop_client() {
    client_ssh "sudo pkill -TERM obftun"
    sleep 1
}

stop_all() {
    stop_client
    stop_server
}

kill_all() {
    client_ssh "sudo pkill -9 obftun"
    server_ssh "sudo pkill -9 obftun"
    sleep 1
}

wait_for_connect() {
    local timeout="${1:-15}"
    local i
    for i in $(seq 1 "$timeout"); do
        if server_ssh "grep -q 'connected (interface' ${SERVER_LOG}"; then
            return 0
        fi
        sleep 1
    done
    echo "TIMEOUT: tunnel did not connect within ${timeout}s"
    echo "--- Server log ---"
    server_ssh "cat ${SERVER_LOG}"
    echo "--- Client log ---"
    client_ssh "cat ${CLIENT_LOG}"
    return 1
}

wait_for_disconnect() {
    local timeout="${1:-15}"
    local i
    for i in $(seq 1 "$timeout"); do
        if server_ssh "grep -q 'disconnected' ${SERVER_LOG}"; then
            return 0
        fi
        sleep 1
    done
    echo "TIMEOUT: tunnel did not disconnect within ${timeout}s"
    return 1
}

wait_for_tunnel() { wait_for_connect "$@"; }

DNSMASQ_LEASES="/tmp/dnsmasq.leases"

start_dnsmasq() {
    server_ssh "sudo pkill dnsmasq; \
        sudo mkdir -p /var/lib/misc; \
        sudo dnsmasq \
            --interface=br-obftun \
            --bind-interfaces \
            --listen-address=10.10.0.1 \
            --port=0 \
            --dhcp-range=10.10.0.10,10.10.255.254,30m \
            --dhcp-option=option:router,10.10.0.1 \
            --dhcp-option=option:dns-server,8.8.8.8,8.8.4.4 \
            --dhcp-leasefile=${DNSMASQ_LEASES} \
            --log-dhcp"
    sleep 1
    if ! server_ssh "pgrep dnsmasq > /dev/null"; then
        fail "dnsmasq failed to start"
    fi
}

stop_dnsmasq() {
    server_ssh "sudo pkill dnsmasq"
    sleep 1
}

create_wifi_client() {
    client_ssh "sudo ip link add veth-dev type veth peer name veth-br; \
        sudo ip link set veth-br master br-client; \
        sudo ip link set veth-br up; \
        sudo ip link set veth-dev up"
}

destroy_wifi_client() {
    client_ssh "sudo dhclient -r veth-dev; sudo ip link del veth-dev" 2>/dev/null
}

wifi_dhcp_request() {
    client_ssh "sudo dhclient -v veth-dev 2>&1"
}

wifi_dhcp_release() {
    client_ssh "sudo dhclient -r veth-dev" 2>/dev/null
}

wifi_client_ip() {
    client_ssh "ip -4 addr show veth-dev | grep -oP 'inet \K[0-9.]+'"
}

generate_feed_token() {
    go run "${REPO_ROOT}/cmd/test-token" "${SECRET}"
}

cleanup_interfaces() {
    server_ssh "sudo ip link del tap0" 2>/dev/null
    client_ssh "sudo ip link del tap0" 2>/dev/null
}

flush_tables() {
    server_ssh "sudo iptables -F" 2>/dev/null
    client_ssh "sudo iptables -F" 2>/dev/null
}

full_cleanup() {
    destroy_wifi_client
    stop_dnsmasq
    kill_all
    cleanup_interfaces
    flush_tables
}

CURRENT_TEST=""

begin_test() {
    CURRENT_TEST="$1"
    echo ""
    echo "=== TEST: ${CURRENT_TEST} ==="
    full_cleanup
}

pass() {
    echo "  PASS: ${CURRENT_TEST}"
}

fail() {
    local reason="${1:-}"
    echo "  FAIL: ${CURRENT_TEST} — ${reason}"
    echo "  --- Server log (last 20 lines) ---"
    server_ssh "tail -20 ${SERVER_LOG}"
    echo "  --- Client log (last 20 lines) ---"
    client_ssh "tail -20 ${CLIENT_LOG}"
    exit 1
}

if [ -z "${SERVER_IP:-}" ] || [ -z "${CLIENT_IP:-}" ]; then
    discover_ips
fi
