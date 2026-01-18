#!/usr/bin/env bash
# Test helper functions for E2E installation tests

# Color codes for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'  # No Color

# Log functions
log_info() {
    echo -e "${YELLOW}ℹ️  $1${NC}"
}

log_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

log_error() {
    echo -e "${RED}❌ $1${NC}" >&2
}

# Assertion functions
assert_command_exists() {
    local cmd=$1
    if ! command -v "$cmd" &> /dev/null; then
        log_error "Command not found: $cmd"
        exit 1
    fi
    log_success "Command found: $cmd"
}

assert_file_exists() {
    local file=$1
    if [ ! -f "$file" ]; then
        log_error "File not found: $file"
        exit 1
    fi
    log_success "File exists: $file"
}

assert_exit_code() {
    local code=$?
    local expected=${1:-0}
    if [ $code -ne $expected ]; then
        log_error "Exit code $code (expected $expected)"
        exit 1
    fi
}

assert_output_contains() {
    local output=$1
    local expected=$2
    if ! echo "$output" | grep -q "$expected"; then
        log_error "Output does not contain: $expected"
        log_info "Actual output: $output"
        exit 1
    fi
    log_success "Output contains: $expected"
}
