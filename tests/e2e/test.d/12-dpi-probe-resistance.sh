#!/usr/bin/env bash

source "$(dirname "$0")/../helpers.sh"

begin_test "REST surface returns proper status codes"

start_server

# Helper: issue a curl request and capture the HTTP status code.
http_status() {
    server_ssh "curl -s -o /dev/null -w '%{http_code}' $*"
}

# Helper: issue a curl request and capture body.
http_body() {
    server_ssh "curl -s $*"
}

check_status() {
    local desc="$1"
    local expected="$2"
    local got="$3"
    if [ "${got}" != "${expected}" ]; then
        fail "${desc}: expected HTTP ${expected}, got ${got}"
    fi
}

check_json_error() {
    local desc="$1"
    local body="$2"
    if ! echo "${body}" | grep -q '"error"'; then
        fail "${desc}: response body missing \"error\" field"
    fi
    if ! echo "${body}" | grep -q '"request_id"'; then
        fail "${desc}: response body missing \"request_id\" field"
    fi
}

# 1. GET / → 200 JSON envelope with status:ok
status=$(http_status "http://localhost:80/")
check_status "GET /" 200 "${status}"
body=$(http_body "http://localhost:80/")
if ! echo "${body}" | grep -q '"status":"ok"'; then
    fail "GET /: expected status:ok in body"
fi

# 2. POST /api/send with random garbage body → 400
status=$(http_status "-X POST -d 'random garbage' http://localhost:80/api/send")
check_status "POST /api/send with garbage" 400 "${status}"
body=$(http_body "-X POST -d 'random garbage' http://localhost:80/api/send")
check_json_error "POST /api/send with garbage" "${body}"

# 3. GET /api/feed without token → 401
status=$(http_status "http://localhost:80/api/feed")
check_status "GET /api/feed no token" 401 "${status}"
body=$(http_body "http://localhost:80/api/feed")
check_json_error "GET /api/feed no token" "${body}"

# 4. GET /api/feed with bogus token → 401
status=$(http_status "'http://localhost:80/api/feed?t=bogus'")
check_status "GET /api/feed bogus token" 401 "${status}"

# 5. HEAD / → no SSE content type
resp=$(server_ssh "curl -s -i -I http://localhost:80/")
if echo "${resp}" | grep -qi "text/event-stream"; then
    fail "HEAD /: got SSE content type"
fi

# 6. GET /some/random/path → 404
status=$(http_status "http://localhost:80/some/random/path")
check_status "GET /some/random/path" 404 "${status}"
body=$(http_body "http://localhost:80/some/random/path")
check_json_error "GET /some/random/path" "${body}"

# 7. POST /api/feed → 405 with Allow: GET
resp=$(server_ssh "curl -s -i -X POST http://localhost:80/api/feed")
if ! echo "${resp}" | head -1 | grep -q " 405"; then
    fail "POST /api/feed: expected HTTP 405"
fi
if ! echo "${resp}" | grep -qi "^allow:.*GET"; then
    fail "POST /api/feed: missing Allow: GET header"
fi

# 8. GET /api/send → 405 with Allow: POST
resp=$(server_ssh "curl -s -i http://localhost:80/api/send")
if ! echo "${resp}" | head -1 | grep -q " 405"; then
    fail "GET /api/send: expected HTTP 405"
fi
if ! echo "${resp}" | grep -qi "^allow:.*POST"; then
    fail "GET /api/send: missing Allow: POST header"
fi

stop_server
pass
