#!/usr/bin/env bash
#
# End-to-End Workflow Test for diagram-as-code skills
# Tests: create-diagrams → review-diagrams → render-diagrams → diagram-sync
#
# Task 9.1: End-to-end workflow testing
# Bead: scheduling-infrastructure-consolidation-myzb

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SKILLS_DIR="$(dirname "$SCRIPT_DIR")/skills"
TEST_OUTPUT_DIR="${SCRIPT_DIR}/output/e2e-$(date +%Y%m%d-%H%M%S)"
TEST_FIXTURES_DIR="${SCRIPT_DIR}/fixtures/sample-codebase"

# Test counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[✓]${NC} $*"
    ((TESTS_PASSED++))
}

log_error() {
    echo -e "${RED}[✗]${NC} $*"
    ((TESTS_FAILED++))
}

log_warning() {
    echo -e "${YELLOW}[!]${NC} $*"
}

log_section() {
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}$*${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

# Test helper
run_test() {
    local test_name="$1"
    shift
    ((TESTS_RUN++))

    log_info "Running: $test_name"

    if "$@"; then
        log_success "$test_name"
        return 0
    else
        log_error "$test_name (exit code: $?)"
        return 1
    fi
}

# Setup test environment
setup_test_env() {
    log_section "Setup Test Environment"

    # Create output directory
    mkdir -p "$TEST_OUTPUT_DIR"
    log_info "Test output directory: $TEST_OUTPUT_DIR"

    # Create test fixtures if not exist
    if [[ ! -d "$TEST_FIXTURES_DIR" ]]; then
        log_info "Creating test fixtures..."
        mkdir -p "$TEST_FIXTURES_DIR"
        create_test_fixtures
    else
        log_info "Using existing test fixtures: $TEST_FIXTURES_DIR"
    fi

    log_success "Test environment ready"
}

# Create minimal test codebase
create_test_fixtures() {
    local fixtures="$TEST_FIXTURES_DIR"

    # Create Go service
    mkdir -p "$fixtures/services/auth"
    cat > "$fixtures/services/auth/main.go" <<'EOF'
package main

import (
    "database/sql"
    "net/http"
)

type AuthService struct {
    db *sql.DB
}

func (s *AuthService) HandleLogin(w http.ResponseWriter, r *http.Request) {
    // Authentication logic
}

func main() {
    db, _ := sql.Open("postgres", "...")
    service := &AuthService{db: db}
    http.HandleFunc("/login", service.HandleLogin)
    http.ListenAndServe(":8080", nil)
}
EOF

    # Create Python API
    mkdir -p "$fixtures/services/api"
    cat > "$fixtures/services/api/app.py" <<'EOF'
from flask import Flask, request
import psycopg2

app = Flask(__name__)

@app.route('/users')
def get_users():
    conn = psycopg2.connect("dbname=users")
    # Query users
    return {"users": []}

if __name__ == '__main__':
    app.run(port=5000)
EOF

    # Create TypeScript frontend
    mkdir -p "$fixtures/frontend/src"
    cat > "$fixtures/frontend/src/App.tsx" <<'EOF'
import React from 'react';
import axios from 'axios';

export const App: React.FC = () => {
    const [users, setUsers] = React.useState([]);

    React.useEffect(() => {
        axios.get('http://localhost:5000/users')
            .then(res => setUsers(res.data.users));
    }, []);

    return <div>Users: {users.length}</div>;
};
EOF

    log_info "Created test fixtures in $fixtures"
}

# Test 1: create-diagrams workflow
test_create_diagrams() {
    log_section "Test 1: create-diagrams"

    local output_dir="$TEST_OUTPUT_DIR/diagrams"
    mkdir -p "$output_dir"

    # Test D2 generation
    run_test "create-diagrams (D2 format)" \
        python3 "$SKILLS_DIR/create-diagrams/create_diagrams.py" \
            "$TEST_FIXTURES_DIR" \
            "$output_dir" \
            --format d2 \
            --level all

    # Verify D2 output files exist
    run_test "verify D2 context diagram exists" \
        test -f "$output_dir/c4-context.d2"

    # Test Mermaid generation
    run_test "create-diagrams (Mermaid format)" \
        python3 "$SKILLS_DIR/create-diagrams/create_diagrams.py" \
            "$TEST_FIXTURES_DIR" \
            "$output_dir" \
            --format mermaid \
            --level context

    # Verify Mermaid output
    run_test "verify Mermaid context diagram exists" \
        test -f "$output_dir/c4-context.mmd"

    # Test Structurizr generation
    run_test "create-diagrams (Structurizr format)" \
        python3 "$SKILLS_DIR/create-diagrams/create_diagrams.py" \
            "$TEST_FIXTURES_DIR" \
            "$output_dir" \
            --format structurizr \
            --level all

    # Verify Structurizr output
    run_test "verify Structurizr workspace exists" \
        test -f "$output_dir/workspace.dsl"

    log_info "Generated diagrams saved to: $output_dir"
}

# Test 2: review-diagrams workflow
test_review_diagrams() {
    log_section "Test 2: review-diagrams"

    local diagrams_dir="$TEST_OUTPUT_DIR/diagrams"
    local review_output="$TEST_OUTPUT_DIR/review-results.json"

    # Review D2 diagram
    if [[ -f "$diagrams_dir/c4-context.d2" ]]; then
        run_test "review-diagrams (D2)" \
            python3 "$SKILLS_DIR/review-diagrams/review_diagrams.py" \
                "$diagrams_dir/c4-context.d2" \
                --output "$review_output"

        # Verify review output contains score
        if [[ -f "$review_output" ]]; then
            run_test "verify review score exists" \
                grep -q '"score"' "$review_output"
        fi
    else
        log_warning "Skipping review test - no D2 diagram generated"
    fi
}

# Test 3: render-diagrams workflow
test_render_diagrams() {
    log_section "Test 3: render-diagrams"

    local diagrams_dir="$TEST_OUTPUT_DIR/diagrams"
    local rendered_dir="$TEST_OUTPUT_DIR/rendered"
    mkdir -p "$rendered_dir"

    # Render D2 to PNG
    if [[ -f "$diagrams_dir/c4-context.d2" ]]; then
        # Check if d2 is installed
        if command -v d2 &> /dev/null; then
            run_test "render-diagrams (D2 → PNG)" \
                python3 "$SKILLS_DIR/render-diagrams/render_diagrams.py" \
                    "$diagrams_dir/c4-context.d2" \
                    "$rendered_dir/c4-context.png" \
                    --format png

            run_test "verify rendered PNG exists" \
                test -f "$rendered_dir/c4-context.png"
        else
            log_warning "Skipping D2 render test - d2 not installed"
        fi
    fi

    # Render Mermaid to SVG
    if [[ -f "$diagrams_dir/c4-context.mmd" ]]; then
        # Check if mmdc is installed
        if command -v mmdc &> /dev/null; then
            run_test "render-diagrams (Mermaid → SVG)" \
                python3 "$SKILLS_DIR/render-diagrams/render_diagrams.py" \
                    "$diagrams_dir/c4-context.mmd" \
                    "$rendered_dir/c4-context.svg" \
                    --format svg

            run_test "verify rendered SVG exists" \
                test -f "$rendered_dir/c4-context.svg"
        else
            log_warning "Skipping Mermaid render test - mmdc not installed"
        fi
    fi
}

# Test 4: diagram-sync workflow
test_diagram_sync() {
    log_section "Test 4: diagram-sync"

    local diagrams_dir="$TEST_OUTPUT_DIR/diagrams"
    local sync_output="$TEST_OUTPUT_DIR/sync-report.json"

    if [[ -f "$diagrams_dir/c4-context.d2" ]]; then
        run_test "diagram-sync (detect drift)" \
            python3 "$SKILLS_DIR/diagram-sync/diagram_sync.py" \
                "$diagrams_dir" \
                "$TEST_FIXTURES_DIR" \
                --output "$sync_output"

        # Verify sync report generated
        if [[ -f "$sync_output" ]]; then
            run_test "verify sync score exists" \
                grep -q '"sync_score"' "$sync_output"
        fi
    else
        log_warning "Skipping sync test - no diagrams to check"
    fi
}

# Test 5: Integration test (full workflow)
test_full_workflow() {
    log_section "Test 5: Full Workflow Integration"

    local workflow_dir="$TEST_OUTPUT_DIR/workflow-test"
    mkdir -p "$workflow_dir"

    log_info "Step 1: Create diagrams"
    if python3 "$SKILLS_DIR/create-diagrams/create_diagrams.py" \
        "$TEST_FIXTURES_DIR" \
        "$workflow_dir/diagrams" \
        --format d2 \
        --level all > "$workflow_dir/create.log" 2>&1; then
        log_success "Diagrams created"
    else
        log_error "Diagram creation failed"
        cat "$workflow_dir/create.log"
        return 1
    fi

    log_info "Step 2: Review diagrams"
    if python3 "$SKILLS_DIR/review-diagrams/review_diagrams.py" \
        "$workflow_dir/diagrams/c4-context.d2" \
        --output "$workflow_dir/review.json" > "$workflow_dir/review.log" 2>&1; then
        log_success "Diagrams reviewed"
    else
        log_error "Diagram review failed"
        cat "$workflow_dir/review.log"
        return 1
    fi

    log_info "Step 3: Check sync status"
    if python3 "$SKILLS_DIR/diagram-sync/diagram_sync.py" \
        "$workflow_dir/diagrams" \
        "$TEST_FIXTURES_DIR" \
        --output "$workflow_dir/sync.json" > "$workflow_dir/sync.log" 2>&1; then
        log_success "Sync checked"
    else
        log_error "Sync check failed"
        cat "$workflow_dir/sync.log"
        return 1
    fi

    log_success "Full workflow completed successfully"
}

# Generate test report
generate_report() {
    log_section "Test Results Summary"

    local report_file="$TEST_OUTPUT_DIR/test-report.txt"

    cat > "$report_file" <<EOF
End-to-End Workflow Test Report
================================

Date: $(date)
Test Output Directory: $TEST_OUTPUT_DIR

Summary:
--------
Total Tests Run: $TESTS_RUN
Passed: $TESTS_PASSED
Failed: $TESTS_FAILED
Success Rate: $(awk "BEGIN {printf \"%.1f\", ($TESTS_PASSED/$TESTS_RUN)*100}")%

Status: $([ $TESTS_FAILED -eq 0 ] && echo "✓ ALL TESTS PASSED" || echo "✗ SOME TESTS FAILED")

Test Coverage:
--------------
1. create-diagrams: Tested D2, Mermaid, Structurizr generation
2. review-diagrams: Tested quality validation
3. render-diagrams: Tested PNG and SVG rendering
4. diagram-sync: Tested drift detection
5. Full workflow: Tested complete create → review → sync pipeline

Notes:
------
- Some tests may be skipped if tools (d2, mmdc) are not installed
- Review and sync tests depend on successful diagram generation
- All generated outputs saved to: $TEST_OUTPUT_DIR

EOF

    cat "$report_file"

    log_info "Full report saved to: $report_file"

    # Print final status
    echo ""
    if [ $TESTS_FAILED -eq 0 ]; then
        echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
        echo -e "${GREEN}✓ ALL TESTS PASSED ($TESTS_PASSED/$TESTS_RUN)${NC}"
        echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
        return 0
    else
        echo -e "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
        echo -e "${RED}✗ TESTS FAILED: $TESTS_FAILED/$TESTS_RUN${NC}"
        echo -e "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
        return 1
    fi
}

# Main test execution
main() {
    log_section "E2E Workflow Test - diagram-as-code Skills"
    log_info "Task 9.1: End-to-end workflow testing"
    log_info "Bead: scheduling-infrastructure-consolidation-myzb"

    setup_test_env

    test_create_diagrams
    test_review_diagrams
    test_render_diagrams
    test_diagram_sync
    test_full_workflow

    generate_report
}

# Run tests
main "$@"
