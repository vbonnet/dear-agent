#!/bin/bash
# Integration Test Suite for Research Feed Manager
#
# Tests the complete workflow:
# 1. arXiv API integration
# 2. One-off URL processing
# 3. Batch source import
# 4. Full backfill workflow
#
# Usage:
#   ./test/integration_test.sh  # From repository root
#   Or: bash test/integration_test.sh

set -e  # Exit on error

echo "======================================="
echo "Research Feed Manager Integration Tests"
echo "======================================="
echo ""

# Configuration
TEST_DIR="/tmp/rfm-integration-test-$$"
BINARY="${BINARY:-research-feed-manager}"  # Allow override with env var

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0

# Helper functions
print_test() {
    echo -e "${YELLOW}TEST $((TESTS_RUN+1)):${NC} $1"
    TESTS_RUN=$((TESTS_RUN+1))
}

pass() {
    echo -e "  ${GREEN}✓ PASS${NC}: $1"
    TESTS_PASSED=$((TESTS_PASSED+1))
}

fail() {
    echo -e "  ${RED}✗ FAIL${NC}: $1"
    TESTS_FAILED=$((TESTS_FAILED+1))
}

cleanup() {
    if [ -d "$TEST_DIR" ]; then
        rm -rf "$TEST_DIR"
    fi
}

# Setup
trap cleanup EXIT

mkdir -p "$TEST_DIR"
echo "Test directory: $TEST_DIR"
echo ""

# Check if binary exists  # noqa: path-portability
if ! command -v "$BINARY" &> /dev/null; then
    echo -e "${RED}ERROR:${NC} $BINARY not found in PATH"
    echo "Install with: go install  # Run from repository root"  # noqa: path-portability
    exit 1
fi

echo "Using binary: $(which $BINARY)"
echo "Version: $($BINARY --version 2>&1 || echo 'unknown')"
echo ""

# ==========================================
# Test 1: arXiv Category Onboarding
# ==========================================
print_test "arXiv category onboarding"

# Create temporary test sources file
TEST_SOURCES="$TEST_DIR/sources.jsonl"
TEST_PROCESSED="$TEST_DIR/processed_items.jsonl"
touch "$TEST_SOURCES"
touch "$TEST_PROCESSED"

# Onboard an arXiv category
if $BINARY onboard \
    --type arxiv \
    --url cs.AI \
    --name "Test arXiv AI Category" \
    --config "$TEST_DIR/config.yaml" 2>&1 | tee "$TEST_DIR/test1.log"; then

    # Check if source was added
    if grep -q "arxiv-cs-AI" "$TEST_DIR/test1.log"; then
        pass "arXiv source onboarded successfully"
    else
        fail "arXiv source ID not found in output"
    fi
else
    fail "arXiv onboard command failed"
fi

echo ""

# ==========================================
# Test 2: arXiv Backfill (Small Scale)
# ==========================================
print_test "arXiv backfill (3 items)"

# Note: This test requires internet connection and will hit arXiv API
# Skip if offline or in CI environment
if [ "${SKIP_NETWORK_TESTS:-0}" = "1" ]; then
    echo "  ⊘ SKIP: Network tests disabled (SKIP_NETWORK_TESTS=1)"
else
    # Create minimal config for testing
    cat > "$TEST_DIR/config.yaml" <<EOF
sources:
  database: $TEST_SOURCES
  processed_items: $TEST_PROCESSED

research:
  cache_dir: $TEST_DIR/research
  use_cache: true

backfill:
  default_max_items: 3
  batch_size: 1
EOF

    if $BINARY backfill \
        --source arxiv-cs-AI \
        --max-items 3 \
        --config "$TEST_DIR/config.yaml" 2>&1 | tee "$TEST_DIR/test2.log"; then

        # Check if items were processed
        if grep -q "Processed\|processed" "$TEST_DIR/test2.log"; then
            pass "arXiv backfill completed"

            # Verify output directory was created
            if [ -d "$TEST_DIR/research/content/ArxivPaper" ]; then
                pass "arXiv output directory created"

                # Count processed papers
                PAPER_COUNT=$(find "$TEST_DIR/research/content/ArxivPaper" -mindepth 1 -maxdepth 1 -type d | wc -l)
                if [ "$PAPER_COUNT" -gt 0 ]; then
                    pass "Processed $PAPER_COUNT arXiv papers"
                else
                    fail "No arXiv papers found in output directory"
                fi
            else
                fail "arXiv output directory not created"
            fi
        else
            fail "No processing confirmation in output"
        fi
    else
        fail "arXiv backfill command failed"
    fi
fi

echo ""

# ==========================================
# Test 3: One-off URL Processing (YouTube)
# ==========================================
print_test "One-off URL processing (YouTube video)"

if [ "${SKIP_NETWORK_TESTS:-0}" = "1" ]; then
    echo "  ⊘ SKIP: Network tests disabled"
else
    # Use a well-known stable video (Andrej Karpathy's GPT video)
    TEST_URL="https://www.youtube.com/watch?v=kCc8FmEb1nY"

    if $BINARY process-url "$TEST_URL" \
        --skip-research \
        --output-dir "$TEST_DIR/research" \
        --config "$TEST_DIR/config.yaml" 2>&1 | tee "$TEST_DIR/test3.log"; then

        if grep -q "youtube" "$TEST_DIR/test3.log"; then
            pass "YouTube URL detected correctly"
        else
            fail "YouTube URL not detected"
        fi

        if grep -q "kCc8FmEb1nY" "$TEST_DIR/test3.log"; then
            pass "Video ID extracted correctly"
        else
            fail "Video ID not extracted"
        fi

        # Check processed_items.jsonl
        if grep -q "one-off" "$TEST_PROCESSED"; then
            pass "One-off processing recorded in registry"
        else
            fail "One-off processing not recorded"
        fi
    else
        fail "process-url command failed"
    fi
fi

echo ""

# ==========================================
# Test 4: Batch Source Import (Dry Run)
# ==========================================
print_test "Batch source import validation"

# Create test import file
cat > "$TEST_DIR/test-sources.yaml" <<EOF
sources:
  - type: youtube
    url: https://youtube.com/@test_channel
    name: "Test YouTube Channel"
    keywords: ["AI", "test"]

  - type: arxiv
    url: cs.SE
    name: "Test arXiv SE"
    keywords: []

  - type: rss
    url: https://example.com/feed
    name: "Test RSS Feed"
    keywords: ["AI"]
EOF

if $BINARY import-sources \
    --file "$TEST_DIR/test-sources.yaml" \
    --dry-run \
    --config "$TEST_DIR/config.yaml" 2>&1 | tee "$TEST_DIR/test4.log"; then

    if grep -q "3 sources" "$TEST_DIR/test4.log"; then
        pass "Found 3 sources in YAML file"
    else
        fail "Did not find expected source count"
    fi

    if grep -q "validated successfully" "$TEST_DIR/test4.log"; then
        pass "Sources validated successfully"
    else
        fail "Validation failed"
    fi

    if grep -q "dry run" -i "$TEST_DIR/test4.log"; then
        pass "Dry run mode confirmed"
    else
        fail "Dry run mode not confirmed"
    fi
else
    fail "import-sources dry-run failed"
fi

echo ""

# ==========================================
# Test 5: Batch Source Import (Actual)
# ==========================================
print_test "Batch source import (actual)"

if $BINARY import-sources \
    --file "$TEST_DIR/test-sources.yaml" \
    --config "$TEST_DIR/config.yaml" 2>&1 | tee "$TEST_DIR/test5.log"; then

    # Count lines in sources.jsonl
    SOURCE_COUNT=$(wc -l < "$TEST_SOURCES")

    if [ "$SOURCE_COUNT" -ge 4 ]; then
        pass "Sources imported (found $SOURCE_COUNT sources total)"
    else
        fail "Sources not imported (found only $SOURCE_COUNT sources)"
    fi

    # Verify each type was imported
    if grep -q '"type":"youtube"' "$TEST_SOURCES"; then
        pass "YouTube source imported"
    else
        fail "YouTube source not found"
    fi

    if grep -q '"type":"arxiv"' "$TEST_SOURCES"; then
        pass "arXiv source imported"
    else
        fail "arXiv source not found"
    fi

    if grep -q '"type":"rss"' "$TEST_SOURCES"; then
        pass "RSS source imported"
    else
        fail "RSS source not found"
    fi
else
    fail "import-sources command failed"
fi

echo ""

# ==========================================
# Test 6: Duplicate Detection (Skip Existing)
# ==========================================
print_test "Duplicate source detection"

if $BINARY import-sources \
    --file "$TEST_DIR/test-sources.yaml" \
    --skip-existing \
    --config "$TEST_DIR/config.yaml" 2>&1 | tee "$TEST_DIR/test6.log"; then

    if grep -q "existing\|duplicate\|skip" -i "$TEST_DIR/test6.log"; then
        pass "Duplicate detection works"
    else
        fail "No duplicate detection message"
    fi

    # Source count should not increase
    NEW_SOURCE_COUNT=$(wc -l < "$TEST_SOURCES")
    if [ "$NEW_SOURCE_COUNT" -eq "$SOURCE_COUNT" ]; then
        pass "No duplicate sources added"
    else
        fail "Duplicate sources were added (expected $SOURCE_COUNT, got $NEW_SOURCE_COUNT)"
    fi
else
    fail "import-sources with skip-existing failed"
fi

echo ""

# ==========================================
# Test 7: Invalid Source Validation
# ==========================================
print_test "Invalid source validation"

# Create invalid sources file
cat > "$TEST_DIR/invalid-sources.yaml" <<EOF
sources:
  - type: youtube
    # Missing URL
    name: "Invalid Source"

  - type: invalid_type
    url: https://example.com
    name: "Invalid Type"

  - type: arxiv
    url: invalid.category
    name: "Invalid arXiv Category"
EOF

if $BINARY import-sources \
    --file "$TEST_DIR/invalid-sources.yaml" \
    --dry-run \
    --config "$TEST_DIR/config.yaml" 2>&1 | tee "$TEST_DIR/test7.log"; then

    fail "Validation should have failed for invalid sources"
else
    # Command should fail (exit code != 0)
    if grep -q "validation\|error" -i "$TEST_DIR/test7.log"; then
        pass "Validation correctly rejected invalid sources"
    else
        fail "No validation error message"
    fi
fi

echo ""

# ==========================================
# Test 8: List Sources
# ==========================================
print_test "List sources command"

if $BINARY list \
    --config "$TEST_DIR/config.yaml" 2>&1 | tee "$TEST_DIR/test8.log"; then

    if grep -q "arxiv-cs-AI\|Test YouTube Channel" "$TEST_DIR/test8.log"; then
        pass "List command shows imported sources"
    else
        fail "List command does not show sources"
    fi
else
    fail "list command failed"
fi

echo ""

# ==========================================
# Summary
# ==========================================
echo "======================================="
echo "Test Summary"
echo "======================================="
echo "Total tests: $TESTS_RUN"
echo -e "${GREEN}Passed: $TESTS_PASSED${NC}"
if [ "$TESTS_FAILED" -gt 0 ]; then
    echo -e "${RED}Failed: $TESTS_FAILED${NC}"
else
    echo "Failed: $TESTS_FAILED"
fi
echo ""

if [ "$TESTS_FAILED" -eq 0 ]; then
    echo -e "${GREEN}✓ All tests passed!${NC}"
    exit 0
else
    echo -e "${RED}✗ Some tests failed${NC}"
    exit 1
fi
