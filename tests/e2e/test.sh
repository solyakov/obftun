#!/usr/bin/env bash

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_DIR="${SCRIPT_DIR}/test.d"

source "${SCRIPT_DIR}/helpers.sh"

preflight() {
    echo "Pre-flight checks..."

    cd "${VAGRANT_DIR}"
    local running
    running=$(${VAGRANT} status --machine-readable 2>/dev/null | grep -c ",state,running")
    if [ "${running}" -lt 2 ]; then
        echo "Starting VMs..."
        VAGRANT_DEFAULT_PROVIDER=vmware_desktop ${VAGRANT} up
    fi

    discover_ips

    if [ -z "${SERVER_IP}" ] || [ -z "${CLIENT_IP}" ]; then
        echo "ERROR: Could not discover VM IPs."
        exit 1
    fi

    rebuild
    echo "Pre-flight OK."
}

main() {
    preflight

    local pass_count=0
    local fail_count=0

    local tests
    tests=$(find "${TEST_DIR}" -maxdepth 1 -type f -perm +111 -name '*.sh' | sort)

    if [ -z "${tests}" ]; then
        echo "No tests found in ${TEST_DIR}"
        exit 1
    fi

    for test_script in ${tests}; do
        full_cleanup

        if bash "${test_script}"; then
            pass_count=$((pass_count + 1))
        else
            fail_count=$((fail_count + 1))
        fi
    done

    full_cleanup

    local total=$((pass_count + fail_count))
    echo ""
    echo "========================================"
    echo "  Results: ${total} tests, ${pass_count} passed, ${fail_count} failed"
    echo "========================================"

    [ "${fail_count}" -eq 0 ]
}

main "$@"
