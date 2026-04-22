#!/bin/bash
# Shared validation library for build/test integrity checks
# Used by Stop hook and /check-build skill
#
# Functions:
#   load_config() - Load .build-integrity.yaml or defaults
#   detect_test_command() - Detect test command (config or auto-detect)
#   detect_build_command() - Detect build command (config or auto-detect)
#   run_tests() - Execute test command with timeout
#   run_build() - Execute build command with timeout
#   format_results() - Format validation results with colors
#   show_remediation() - Display remediation guidance

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Default config
DEFAULT_TIMEOUT=300
DEFAULT_ENABLED=true

#######################################
# Load configuration from .build-integrity.yaml or return defaults
# Globals:
#   None
# Arguments:
#   None
# Returns:
#   JSON config string
#######################################
load_config() {
    local config_file=".build-integrity.yaml"

    # Check if config exists
    if [ ! -f "$config_file" ]; then
        # Return defaults as JSON
        echo "{\"enabled\":$DEFAULT_ENABLED,\"timeout\":$DEFAULT_TIMEOUT,\"test_command\":null,\"build_command\":null}"
        return 0
    fi

    # Check if yq is available
    if ! command -v yq &> /dev/null; then
        echo "⚠️  yq not found, using defaults (install: brew install yq or apt-get install yq)" >&2
        echo "{\"enabled\":$DEFAULT_ENABLED,\"timeout\":$DEFAULT_TIMEOUT,\"test_command\":null,\"build_command\":null}"
        return 0
    fi

    # Parse config with yq
    local config
    config=$(yq -o json "$config_file" 2>/dev/null) || {
        echo "⚠️  Invalid $config_file, using defaults" >&2
        echo "{\"enabled\":$DEFAULT_ENABLED,\"timeout\":$DEFAULT_TIMEOUT,\"test_command\":null,\"build_command\":null}"
        return 0
    }

    # Merge with defaults (config overrides)
    local timeout
    timeout=$(echo "$config" | jq -r '.timeout // 300')
    local enabled
    enabled=$(echo "$config" | jq -r 'if .enabled == null then true else .enabled end')
    local test_cmd
    test_cmd=$(echo "$config" | jq -r '.test_command // null')
    local build_cmd
    build_cmd=$(echo "$config" | jq -r '.build_command // null')

    # Return merged config
    jq -n \
        --argjson enabled "$enabled" \
        --argjson timeout "$timeout" \
        --arg test_cmd "$test_cmd" \
        --arg build_cmd "$build_cmd" \
        '{enabled: $enabled, timeout: $timeout, test_command: $test_cmd, build_command: $build_cmd}'
}

#######################################
# Detect test command (config or auto-detect)
# Globals:
#   None
# Arguments:
#   $1 - Config JSON
# Returns:
#   Test command string (or empty if none found)
#######################################
detect_test_command() {
    local config=$1

    # Check config first
    local config_cmd
    config_cmd=$(echo "$config" | jq -r '.test_command // ""')
    if [ "$config_cmd" != "null" ] && [ -n "$config_cmd" ]; then
        echo "$config_cmd"
        return 0
    fi

    # Auto-detect based on project files
    # Node.js (npm)
    if [ -f "package.json" ]; then
        if jq -e '.scripts.test' package.json &> /dev/null; then
            echo "npm test"
            return 0
        fi
    fi

    # Rust (cargo)
    if [ -f "Cargo.toml" ]; then
        echo "cargo test"
        return 0
    fi

    # Python (pytest)
    if [ -f "pytest.ini" ] || [ -f "pyproject.toml" ] || [ -d "tests" ]; then
        if command -v pytest &> /dev/null; then
            echo "pytest"
            return 0
        else
            echo "⚠️  Python tests detected but pytest not installed" >&2
            echo "   Install: pip install pytest" >&2
        fi
    fi

    # Go
    if [ -f "go.mod" ] || ls *.go &> /dev/null; then
        echo "go test ./..."
        return 0
    fi

    # Makefile
    if [ -f "Makefile" ]; then
        if make -n test &> /dev/null 2>&1; then
            echo "make test"
            return 0
        fi
    fi

    # No test command found
    echo ""
}

#######################################
# Detect build command (config or auto-detect)
# Globals:
#   None
# Arguments:
#   $1 - Config JSON
# Returns:
#   Build command string (or empty if none found)
#######################################
detect_build_command() {
    local config=$1

    # Check config first
    local config_cmd
    config_cmd=$(echo "$config" | jq -r '.build_command // ""')
    if [ "$config_cmd" != "null" ] && [ -n "$config_cmd" ]; then
        echo "$config_cmd"
        return 0
    fi

    # Auto-detect based on project files
    # Node.js (npm)
    if [ -f "package.json" ]; then
        if jq -e '.scripts.build' package.json &> /dev/null; then
            echo "npm run build"
            return 0
        fi
    fi

    # Rust (cargo)
    if [ -f "Cargo.toml" ]; then
        echo "cargo build"
        return 0
    fi

    # Go
    if [ -f "go.mod" ] || ls *.go &> /dev/null; then
        # Check if main.go exists (buildable)
        if [ -f "main.go" ] || [ -f "cmd/*/main.go" ]; then
            echo "go build"
            return 0
        fi
    fi

    # Makefile
    if [ -f "Makefile" ]; then
        if make -n build &> /dev/null 2>&1; then
            echo "make build"
            return 0
        elif make -n all &> /dev/null 2>&1; then
            echo "make all"
            return 0
        fi
    fi

    # No build command found (not all projects need builds)
    echo ""
}

#######################################
# Run test command with timeout
# Globals:
#   None
# Arguments:
#   $1 - Test command
#   $2 - Timeout in seconds
# Returns:
#   Exit code: 0 = success, non-zero = failure
#   Outputs test results to stdout
#######################################
run_tests() {
    local cmd=$1
    local timeout_sec=$2

    if [ -z "$cmd" ]; then
        echo "No test command found (skipping)"
        return 0
    fi

    # Run with timeout
    local output
    local exit_code
    output=$(timeout "$timeout_sec" bash -c "$cmd" 2>&1) || exit_code=$?

    # Check if timeout
    if [ "${exit_code:-0}" -eq 124 ]; then
        echo -e "${RED}❌ Tests timed out (${timeout_sec}s limit)${NC}"
        echo "Increase timeout in .build-integrity.yaml or optimize tests"
        return 124
    fi

    # Output test results
    echo "$output"
    return "${exit_code:-0}"
}

#######################################
# Run build command with timeout
# Globals:
#   None
# Arguments:
#   $1 - Build command
#   $2 - Timeout in seconds
# Returns:
#   Exit code: 0 = success, non-zero = failure
#   Outputs build results to stdout
#######################################
run_build() {
    local cmd=$1
    local timeout_sec=$2

    if [ -z "$cmd" ]; then
        echo "No build command found (skipping)"
        return 0
    fi

    # Run with timeout
    local output
    local exit_code
    output=$(timeout "$timeout_sec" bash -c "$cmd" 2>&1) || exit_code=$?

    # Check if timeout
    if [ "${exit_code:-0}" -eq 124 ]; then
        echo -e "${RED}❌ Build timed out (${timeout_sec}s limit)${NC}"
        echo "Increase timeout in .build-integrity.yaml or optimize build"
        return 124
    fi

    # Output build results
    echo "$output"
    return "${exit_code:-0}"
}

#######################################
# Format validation results with colors
# Globals:
#   RED, GREEN, YELLOW, NC
# Arguments:
#   $1 - Tests passed (true/false)
#   $2 - Tests output
#   $3 - Build passed (true/false)
#   $4 - Build output
#   $5 - Verbose (true/false, optional)
# Returns:
#   Formatted output string
#######################################
format_results() {
    local tests_passed=$1
    local tests_output=$2
    local build_passed=$3
    local build_output=$4
    local verbose=${5:-false}

    local overall_status="PASSED"
    if [ "$tests_passed" = "false" ] || [ "$build_passed" = "false" ]; then
        overall_status="FAILED"
    fi

    # Header
    if [ "$overall_status" = "PASSED" ]; then
        echo -e "${GREEN}✅ Build/Test Integrity Check PASSED${NC}"
    else
        echo -e "${RED}❌ Build/Test Integrity Check FAILED${NC}"
    fi
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""

    # Tests section
    echo -n "Tests: "
    if [ "$tests_passed" = "true" ]; then
        echo -e "${GREEN}PASSED ✅${NC}"
    else
        echo -e "${RED}FAILED ❌${NC}"
    fi

    if [ "$verbose" = "true" ] || [ "$tests_passed" = "false" ]; then
        echo "$tests_output" | sed 's/^/  /'
    else
        # Summary only
        echo "  • Tests completed successfully"
    fi
    echo ""

    # Build section
    echo -n "Build: "
    if [ "$build_passed" = "true" ]; then
        echo -e "${GREEN}PASSED ✅${NC}"
    elif [ "$build_passed" = "skipped" ]; then
        echo -e "${YELLOW}SKIPPED${NC} (tests failed)"
    else
        echo -e "${RED}FAILED ❌${NC}"
    fi

    if [ "$verbose" = "true" ] || [ "$build_passed" = "false" ]; then
        echo "$build_output" | sed 's/^/  /'
    fi
    echo ""

    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
}

#######################################
# Show remediation guidance
# Globals:
#   YELLOW, NC
# Arguments:
#   None
# Returns:
#   Remediation guidance string
#######################################
show_remediation() {
    echo -e "${YELLOW}"
    echo "⚠️  CRITICAL: You MUST NOT exit session with failing tests."
    echo ""
    echo "Next steps:"
    echo "1. Fix failing tests (see failures above)"
    echo "2. Run /check-build to re-validate"
    echo "3. Try exiting again"
    echo ""
    echo "Remediation guidance:"
    echo "• Brittle test? Fix assertions, mocks, or setup"
    echo "• Deprecated test? Delete if redundant (prove with coverage)"
    echo "• Real bug? Fix the implementation, not the test"
    echo ""
    echo "See always-loaded engram for detailed remediation workflow."
    echo -e "${NC}"
}
