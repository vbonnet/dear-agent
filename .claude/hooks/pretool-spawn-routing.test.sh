#!/usr/bin/env bash
# Tests for the pretool-spawn-routing hook. Run: bash pretool-spawn-routing.test.sh
# Asserts the hook nudges on raw session spawns, stays silent on the AGM path
# and ordinary commands, and NEVER emits a permissionDecision (so it can never
# block or auto-approve a call).
set -uo pipefail
HOOK="$(cd "$(dirname "$0")" && pwd)/pretool-spawn-routing"
pass=0 fail=0

# run <name> <expect: nudge|silent> <json>
run() {
  local name="$1" expect="$2" json="$3" out got
  out="$(printf '%s' "$json" | "$HOOK")"
  if [ -n "$out" ]; then got="nudge"; else got="silent"; fi
  if [ "$got" = "$expect" ]; then
    pass=$((pass + 1))
  else
    fail=$((fail + 1)); printf 'FAIL: %s — expected %s, got %s\n  out=%s\n' "$name" "$expect" "$got" "$out"
  fi
  # Invariant: the hook must NEVER make a permission decision.
  if printf '%s' "$out" | grep -q 'permissionDecision'; then
    fail=$((fail + 1)); printf 'FAIL: %s — hook emitted a permissionDecision (must never block/approve)\n' "$name"
  fi
}

run "raw claude spawn"        nudge  '{"tool_name":"Bash","tool_input":{"command":"claude -p \"do a thing\""}}'
run "claude-code spawn"       nudge  '{"tool_name":"Bash","tool_input":{"command":"claude-code --resume xyz"}}'
run "cowork spawn"            nudge  '{"tool_name":"Bash","tool_input":{"command":"cowork run task"}}'
run "spawn after &&"          nudge  '{"tool_name":"Bash","tool_input":{"command":"cd /tmp && claude -p hi"}}'
run "spawn with env prefix"   nudge  '{"tool_name":"Bash","tool_input":{"command":"FOO=bar claude -p hi"}}'
run "scheduled task mcp"      nudge  '{"tool_name":"mcp__scheduled-tasks__create_scheduled_task","tool_input":{}}'
run "agm is the right path"   silent '{"tool_name":"Bash","tool_input":{"command":"agm new worker && agm send worker hi"}}'
run "claude mcp is read-only" silent '{"tool_name":"Bash","tool_input":{"command":"claude mcp list"}}'
run "claude --version"        silent '{"tool_name":"Bash","tool_input":{"command":"claude --version"}}'
run "ordinary command"        silent '{"tool_name":"Bash","tool_input":{"command":"go test ./..."}}'
run "grep mentioning claude"  silent '{"tool_name":"Bash","tool_input":{"command":"git log --author=claude"}}'
run "Agent tool not matched"  silent '{"tool_name":"Agent","tool_input":{"description":"x"}}'
run "empty/garbage input"     silent 'not json at all'

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]
