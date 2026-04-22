#!/usr/bin/env bash
# Dolt verification helpers with graceful degradation

_DOLT_AVAILABLE=""
DOLT_PORT="${DOLT_PORT:-3307}"
DOLT_HOST="${DOLT_HOST:-127.0.0.1}"
DOLT_DB="${DOLT_DB:-oss}"

dolt_check_available() {
    if [[ -n "$_DOLT_AVAILABLE" ]]; then
        return "$_DOLT_AVAILABLE"
    fi
    if mysql -h "$DOLT_HOST" -P "$DOLT_PORT" -u root -e "SELECT 1" "$DOLT_DB" >/dev/null 2>&1; then
        _DOLT_AVAILABLE=0
        printf "# Dolt server available on port %s\n" "$DOLT_PORT"
    else
        _DOLT_AVAILABLE=1
        printf "# WARNING: Dolt server not available on port %s - skipping DB assertions\n" "$DOLT_PORT"
    fi
    return "$_DOLT_AVAILABLE"
}

dolt_query() {
    # Run SQL query against Dolt, return result
    # Returns empty string if Dolt unavailable
    if ! dolt_check_available; then
        echo ""
        return 0
    fi
    mysql -h "$DOLT_HOST" -P "$DOLT_PORT" -u root -N -B -e "$1" "$DOLT_DB" 2>/dev/null
}

dolt_assert_session_exists() {
    local session_name="$1"
    local label="${2:-session '$session_name' exists in Dolt}"
    if ! dolt_check_available; then
        test_skip "$label" "Dolt unavailable"
        return 0
    fi
    local result
    result=$(dolt_query "SELECT COUNT(*) FROM agm_sessions WHERE name = '$session_name'")
    if [[ "$result" -gt 0 ]]; then
        return 0
    else
        test_fail "$label" "session not found in agm_sessions table"
        return 1
    fi
}

dolt_assert_session_field() {
    local session_name="$1"
    local field="$2"
    local expected="$3"
    local label="${4:-session '$session_name' has $field='$expected'}"
    if ! dolt_check_available; then
        test_skip "$label" "Dolt unavailable"
        return 0
    fi
    local result
    result=$(dolt_query "SELECT $field FROM agm_sessions WHERE name = '$session_name'")
    if [[ "$result" == "$expected" ]]; then
        return 0
    else
        test_fail "$label" "expected $field='$expected', got '$result'"
        return 1
    fi
}

dolt_assert_session_count() {
    local expected="$1"
    local filter="${2:-1=1}"
    local label="${3:-session count is $expected}"
    if ! dolt_check_available; then
        test_skip "$label" "Dolt unavailable"
        return 0
    fi
    local result
    result=$(dolt_query "SELECT COUNT(*) FROM agm_sessions WHERE $filter")
    if [[ "$result" -eq "$expected" ]]; then
        return 0
    else
        test_fail "$label" "expected count $expected, got $result"
        return 1
    fi
}
