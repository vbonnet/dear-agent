#!/bin/bash
# Phase 3 Completion Verification Script

# Change to script directory
cd "$(dirname "$0")"

echo "============================================================"
echo "Phase 3 Quality Gate Verification"
echo "============================================================"
echo "Working directory: $(pwd)"
echo

# 1. Check documentation files
echo "1. Documentation Files:"
echo "   - SPEC.md:"
if [ -f "SPEC.md" ]; then
    lines=$(wc -l < SPEC.md)
    echo "     ✅ EXISTS ($lines lines)"
else
    echo "     ❌ MISSING"
    exit 1
fi

echo "   - ARCHITECTURE.md:"
if [ -f "ARCHITECTURE.md" ]; then
    lines=$(wc -l < ARCHITECTURE.md)
    echo "     ✅ EXISTS ($lines lines)"
else
    echo "     ❌ MISSING"
    exit 1
fi

echo

# 2. Check virtual environment
echo "2. Python Dependencies:"
if [ -d ".venv" ]; then
    echo "   ✅ Virtual environment exists"
    if .venv/bin/python3 -c "import sentence_transformers; import numpy; import yaml" 2>/dev/null; then
        echo "   ✅ All dependencies installed"
    else
        echo "   ❌ Dependencies missing"
        exit 1
    fi
else
    echo "   ❌ Virtual environment missing"
    exit 1
fi

echo

# 3. Run tests
echo "3. Integration Tests:"
if .venv/bin/python3 test_mcp_server.py 2>&1 | grep -q "✅ All tests passed!"; then
    echo "   ✅ ALL TESTS PASSED"
else
    echo "   ❌ TESTS FAILED"
    exit 1
fi

echo
echo "============================================================"
echo "✅ ALL QUALITY GATES PASSED"
echo "============================================================"
echo
echo "Ready for Phase 4 transition"
exit 0
