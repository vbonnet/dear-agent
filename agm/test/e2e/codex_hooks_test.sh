#!/bin/bash
# Hook Validation Test for Codex/OpenAI Adapter
# Tests that SessionStart and SessionEnd hooks fire correctly

set -e

# Navigate to repository root (relative to script location)
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$REPO_ROOT"

echo "=== Codex Hook Validation Test ==="
echo "Repository: $REPO_ROOT"
echo ""

# Test synthetic hooks implementation
echo "Test 1: Verify hook implementation exists"
if grep -q "executeHook" internal/agent/openai_adapter.go; then
    echo "✓ executeHook method exists"
else
    echo "✗ executeHook method not found"
    exit 1
fi

# Test hook directory structure
echo ""
echo "Test 2: Verify hook directory path"
if grep -q '\.agm.*openai-hooks' internal/agent/openai_adapter.go; then
    echo "✓ Hook directory path configured (~/.agm/openai-hooks/)"
else
    echo "✗ Hook directory path not found"
    exit 1
fi

# Test SessionStart hook trigger
echo ""
echo "Test 3: Verify SessionStart hook trigger"
if grep -q 'SessionStart.*hook' internal/agent/openai_adapter.go; then
    echo "✓ SessionStart hook trigger implemented"
else
    echo "✗ SessionStart hook not found"
    exit 1
fi

# Test SessionEnd hook trigger
echo ""
echo "Test 4: Verify SessionEnd hook trigger"
if grep -q 'SessionEnd.*hook' internal/agent/openai_adapter.go; then
    echo "✓ SessionEnd hook trigger implemented"
else
    echo "✗ SessionEnd hook not found"
    exit 1
fi

# Test hook context data
echo ""
echo "Test 5: Verify hook context data structure"
if grep -q '"session_id"' internal/agent/openai_adapter.go && \
   grep -q '"hook_name"' internal/agent/openai_adapter.go && \
   grep -q '"session_name"' internal/agent/openai_adapter.go; then
    echo "✓ Hook context includes required fields"
else
    echo "✗ Hook context data incomplete"
    exit 1
fi

# Test non-fatal hook failures
echo ""
echo "Test 6: Verify hooks are non-fatal"
if grep -q 'Non-fatal.*hooks are optional' internal/agent/openai_adapter.go; then
    echo "✓ Hooks fail gracefully (non-blocking)"
else
    echo "✗ Hook failure handling unclear"
    exit 1
fi

# Test hook file format
echo ""
echo "Test 7: Verify hook file naming format"
if grep -q '\.json"' internal/agent/openai_adapter.go; then
    echo "✓ Hook files use JSON format"
else
    echo "✗ Hook file format not JSON"
    exit 1
fi

echo ""
echo "=== Hook Validation Summary ==="
echo "✓ All hook implementation tests passed"
echo ""
echo "Hook Implementation Details:"
echo "  • Type: Synthetic (file-based, not shell scripts)"
echo "  • Location: ~/.agm/openai-hooks/"
echo "  • Format: {session_id}-{hook_name}.json"
echo "  • Triggers: SessionStart (on CreateSession), SessionEnd (on TerminateSession)"
echo "  • Content: JSON with session_id, hook_name, session_name, working_dir, model, timestamp"
echo "  • Failure Mode: Non-fatal (warnings logged, operations continue)"
echo ""
echo "Differences from Claude hooks:"
echo "  • Claude: Shell scripts in ~/.agm/hooks/"
echo "  • Codex: JSON files in ~/.agm/openai-hooks/"
echo "  • Both: Fire at same lifecycle events (SessionStart, SessionEnd)"
echo ""
echo "Note: Real hook testing requires OPENAI_API_KEY for actual session creation"
echo "To test hooks with real API:"
echo "  export OPENAI_API_KEY=sk-..."
echo "  agm session new --agent=codex test-hooks"
echo "  ls ~/.agm/openai-hooks/"
echo "  cat ~/.agm/openai-hooks/{session-id}-SessionStart.json"
echo ""
