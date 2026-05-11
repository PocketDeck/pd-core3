#!/usr/bin/env bash
set -u
# No set -e because we want to handle errors gracefully

# PocketDeck WebSocket System Tests
# Uses websocat to test the server end-to-end

SERVER_BIN="./pd-server"
SERVER_HOST="localhost"
SERVER_PORT="18080"
SERVER_URL="ws://${SERVER_HOST}:${SERVER_PORT}/"
PASS=0
FAIL=0
TOTAL=0

cleanup() {
    if [ -n "${SERVER_PID:-}" ]; then
        kill "$SERVER_PID" 2>/dev/null || true
        wait "$SERVER_PID" 2>/dev/null || true
    fi
    rm -f /tmp/pd_test_in /tmp/pd_test_out
}
trap cleanup EXIT

# --- Utilities ---

# Single-shot: connect, send one message, read one response, disconnect
ws_once() {
    local msg="$1"
    echo "$msg" | websocat -n1 "$SERVER_URL" 2>/dev/null || echo '{"action":"error","error":"no_response"}'
}

# Persistent session: start websocat in background reading from a FIFO
# Usage: ws_session_start
# Then: ws_session_send <msg>   (sends a message)
# Then: ws_session_recv         (reads one response line)
# Then: ws_session_stop

ws_session_start() {
    local name="${1:-default}"
    rm -f "/tmp/pd_in_$name" "/tmp/pd_out_$name"
    mkfifo "/tmp/pd_in_$name" "/tmp/pd_out_$name"

    # Start websocat reading from fifo and writing to fifo
    websocat -n "ws://${SERVER_HOST}:${SERVER_PORT}/" </tmp/pd_in_$name >"/tmp/pd_out_$name" 2>/dev/null &
    eval "WS_PID_$name=\$!"
    eval "exec {FD_IN_$name}>/tmp/pd_in_$name"
    eval "exec {FD_OUT_$name}</tmp/pd_out_$name"

    sleep 0.05
}

ws_session_send() {
    local name="${1:-default}"
    local msg="$2"
    eval "echo '${msg}' >&\${FD_IN_$name}"
}

ws_session_recv() {
    local name="${1:-default}"
    local timeout="${2:-2}"
    eval "read -t $timeout -r resp <&\${FD_OUT_$name}" 2>/dev/null && echo "$resp" || echo ""
}

ws_session_stop() {
    local name="${1:-default}"
    eval "exec {FD_IN_$name}>&-"
    eval "exec {FD_OUT_$name}>&-"
    eval "kill \${WS_PID_$name} 2>/dev/null || true"
    rm -f "/tmp/pd_in_$name" "/tmp/pd_out_$name"
}

check_action() {
    local resp="$1"
    local expected="$2"
    local label="$3"
    TOTAL=$((TOTAL + 1))

    action=$(echo "$resp" | python3 -c "import sys,json; print(json.loads(sys.stdin.read()).get('action',''))" 2>/dev/null || echo "")
    if [ "$action" = "$expected" ]; then
        echo "  PASS: $label"
        PASS=$((PASS + 1))
    else
        echo "  FAIL: $label (expected '$expected', got '$action')"
        echo "    Response: $resp"
        FAIL=$((FAIL + 1))
    fi
}

check_error() {
    local resp="$1"
    local expected="$2"
    local label="$3"
    TOTAL=$((TOTAL + 1))

    err=$(echo "$resp" | python3 -c "import sys,json; d=json.loads(sys.stdin.read()); print(d.get('error',''))" 2>/dev/null || echo "")
    action=$(echo "$resp" | python3 -c "import sys,json; d=json.loads(sys.stdin.read()); print(d.get('action',''))" 2>/dev/null || echo "")
    if [ "$action" = "error" ] && [ "$err" = "$expected" ]; then
        echo "  PASS: $label"
        PASS=$((PASS + 1))
    else
        echo "  FAIL: $label (expected error '$expected', got '$err')"
        echo "    Response: $resp"
        FAIL=$((FAIL + 1))
    fi
}

# --- Test Cases ---

test_create_room() {
    echo ""
    echo "=== Test: Create Room ==="
    resp=$(ws_once '{"action":"create","name":"Alice","game":"uno"}')
    check_action "$resp" "joined" "create room"
    room_id=$(echo "$resp" | python3 -c "import sys,json; print(json.loads(sys.stdin.read()).get('roomID',''))" 2>/dev/null || echo "")
    if [ -n "$room_id" ]; then
        echo "  PASS: roomID=$room_id"
        PASS=$((PASS + 1))
        TOTAL=$((TOTAL + 1))
    else
        echo "  FAIL: no roomID"
        FAIL=$((FAIL + 1))
        TOTAL=$((TOTAL + 1))
    fi
}

test_validation_errors() {
    echo ""
    echo "=== Test: Validation Errors ==="

    resp=$(ws_once '{"action":"create","name":"Alice"}')
    check_error "$resp" "missing_game" "missing game"

    resp=$(ws_once '{"action":"create","name":"Alice","game":"pokemon"}')
    check_error "$resp" "invalid_game" "invalid game"

    resp=$(ws_once '{"action":"join","name":"Alice","roomID":"ZZZZZZ"}')
    check_error "$resp" "room_not_found" "room not found"

    resp=$(ws_once 'not valid json')
    check_error "$resp" "invalid_json" "invalid JSON"

    resp=$(ws_once '{"action":"dance"}')
    check_error "$resp" "unknown_action" "unknown action"

    resp=$(ws_once '{"name":"nobody"}')
    check_error "$resp" "missing_action" "missing action"

    resp=$(ws_once '{"action":"ready"}')
    check_error "$resp" "not_bound" "ready while stray"
}

test_full_flow() {
    echo ""
    echo "=== Test: Full Room Lifecycle ==="

    ws_session_start "alice"

    # Create room
    ws_session_send "alice" '{"action":"create","name":"Alice","game":"uno"}'
    resp=$(ws_session_recv "alice")
    check_action "$resp" "joined" "Alice creates room"
    room_id=$(echo "$resp" | python3 -c "import sys,json; print(json.loads(sys.stdin.read()).get('roomID',''))" 2>/dev/null || echo "")

    # Read navigate broadcast
    resp=$(ws_session_recv "alice" 1)

    # Read players broadcast
    resp=$(ws_session_recv "alice" 1)

    # Status
    ws_session_send "alice" '{"action":"status"}'
    resp=$(ws_session_recv "alice")
    players=$(echo "$resp" | python3 -c "
import sys,json
d=json.loads(sys.stdin.read())
print(d.get('action',''), len(d.get('players',[])))
" 2>/dev/null || echo "")
    if echo "$players" | grep -q "status 1"; then
        echo "  PASS: status shows 1 player"
        PASS=$((PASS + 1))
        TOTAL=$((TOTAL + 1))
    else
        # Might have gotten players broadcast, try again
        ws_session_send "alice" '{"action":"status"}'
        resp=$(ws_session_recv "alice")
        players=$(echo "$resp" | python3 -c "
import sys,json
d=json.loads(sys.stdin.read())
print(d.get('action',''), len(d.get('players',[])))
" 2>/dev/null || echo "")
        if echo "$players" | grep -q "status 1"; then
            echo "  PASS: status shows 1 player"
            PASS=$((PASS + 1))
            TOTAL=$((TOTAL + 1))
        else
            echo "  FAIL: status (got '$players')"
            FAIL=$((FAIL + 1))
            TOTAL=$((TOTAL + 1))
        fi
    fi

    # Leave
    ws_session_send "alice" '{"action":"leave"}'
    resp=$(ws_session_recv "alice")
    check_action "$resp" "left" "Alice leaves"

    ws_session_stop "alice"
}

test_two_player_ready_and_start() {
    echo ""
    echo "=== Test: Two-Player Ready and Start ==="

    ws_session_start "alice"
    ws_session_start "bob"

    # Alice creates room
    ws_session_send "alice" '{"action":"create","name":"Alice","game":"uno"}'
    resp=$(ws_session_recv "alice")
    check_action "$resp" "joined" "Alice creates room"
    room_id=$(echo "$resp" | python3 -c "import sys,json; print(json.loads(sys.stdin.read()).get('roomID',''))" 2>/dev/null || echo "")
    ws_session_recv "alice" 0.5  # drain: navigate broadcast from create
    ws_session_recv "alice" 0.5  # drain: players broadcast from create

    # Bob joins
    ws_session_send "bob" "{\"action\":\"join\",\"name\":\"Bob\",\"roomID\":\"$room_id\"}"
    resp=$(ws_session_recv "bob")
    check_action "$resp" "joined" "Bob joins room"

    # Drain navigate and players broadcasts from Bob joining (Bob gets navigate+players, Alice gets players)
    ws_session_recv "bob" 0.5   # drain: navigate
    ws_session_recv "bob" 0.5   # drain: players
    ws_session_recv "alice" 0.5 # drain: players

    # Alice readies
    ws_session_send "alice" '{"action":"ready"}'
    resp=$(ws_session_recv "alice")
    check_action "$resp" "ready" "Alice ready"
    ws_session_recv "alice" 0.5  # drain: players broadcast (Alice ready) to Alice
    ws_session_recv "bob" 0.5    # drain: players broadcast (Alice ready) to Bob

    # Bob readies (should trigger start for both)
    ws_session_send "bob" '{"action":"ready"}'
    resp=$(ws_session_recv "bob")
    check_action "$resp" "ready" "Bob ready"

    # Players broadcast + start from Bob's AllReady
    ws_session_recv "alice" 0.5  # drain: players broadcast (Bob ready) to Alice
    ws_session_recv "bob" 0.5    # drain: players broadcast (Bob ready) to Bob

    resp=$(ws_session_recv "alice" 2)
    check_action "$resp" "start" "Alice receives start"
    resp=$(ws_session_recv "bob" 2)
    check_action "$resp" "start" "Bob receives start"
    ws_session_recv "alice" 0.5  # drain: navigate broadcast
    ws_session_recv "bob" 0.5    # drain: navigate broadcast

    ws_session_stop "alice"
    ws_session_stop "bob"
}

test_reconnect() {
    echo ""
    echo "=== Test: Reconnect ==="

    # Alice creates room (single-shot)
    resp=$(ws_once '{"action":"create","name":"Alice","game":"uno"}')
    room_id=$(echo "$resp" | python3 -c "import sys,json; print(json.loads(sys.stdin.read()).get('roomID',''))" 2>/dev/null || echo "")

    # Alice reconnects with a new session
    ws_session_start "alice2"
    ws_session_send "alice2" "{\"action\":\"join\",\"name\":\"Alice\",\"roomID\":\"$room_id\"}"
    resp=$(ws_session_recv "alice2")
    check_action "$resp" "joined" "Alice reconnects"

    ws_session_stop "alice2"
}

test_duplicate_name_denied() {
    echo ""
    echo "=== Test: Duplicate Name Denied ==="

    ws_session_start "first"
    ws_session_start "second"

    ws_session_send "first" '{"action":"create","name":"Alice","game":"uno"}'
    resp=$(ws_session_recv "first")
    room_id=$(echo "$resp" | python3 -c "import sys,json; print(json.loads(sys.stdin.read()).get('roomID',''))" 2>/dev/null || echo "")

    ws_session_send "second" "{\"action\":\"join\",\"name\":\"Alice\",\"roomID\":\"$room_id\"}"
    resp=$(ws_session_recv "second")
    check_error "$resp" "name_taken" "duplicate name denied"

    ws_session_stop "first"
    ws_session_stop "second"
}

# --- Main ---

echo "============================================"
echo "  PocketDeck WebSocket System Tests"
echo "============================================"

# Build server
echo ""
echo "Building server..."
go build -o "$SERVER_BIN" ./cmd/server/ 2>&1

# Start server
echo "Starting server on $SERVER_URL..."
"$SERVER_BIN" -host "$SERVER_HOST" -port "$SERVER_PORT" &
SERVER_PID=$!
sleep 1

if ! kill -0 "$SERVER_PID" 2>/dev/null; then
    echo "ERROR: Server failed to start"
    exit 1
fi
echo "Server running with PID $SERVER_PID"

# Run tests
test_create_room
test_validation_errors
test_full_flow
test_two_player_ready_and_start
test_reconnect
test_duplicate_name_denied

# Stop server
echo ""
echo "Stopping server..."
kill "$SERVER_PID" 2>/dev/null || true
wait "$SERVER_PID" 2>/dev/null || true

# Results
echo ""
echo "============================================"
echo "  Results: $PASS / $TOTAL passed, $FAIL failed"
echo "============================================"

if [ "$FAIL" -gt 0 ]; then
    exit 1
fi
