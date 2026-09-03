#!/usr/bin/env bats
#
# Structural contract for the per-family agentic review gate.
#
# The Go tests prove the decision. These prove the wiring around it, which is
# where the two properties that make the decision meaningful actually live:
# the started label has to be published BEFORE the model runs, and nothing in
# the merge-blocking path is allowed to invoke a model.
#
# Both are ordering and composition facts about YAML. A Go test cannot see
# them, and getting either wrong reopens exactly the window the gate exists to
# close while every unit test stays green.

setup() {
  REPO_ROOT="$(cd "${BATS_TEST_DIRNAME}/../.." && pwd)"
  CLAUDE_WF="${REPO_ROOT}/.github/workflows/claude-code-review.yml"
  GEMINI_WF="${REPO_ROOT}/.github/workflows/gemini-review.yml"
  GATE_WF="${REPO_ROOT}/.github/workflows/agentic-review-gate.yml"
  POLICY="${REPO_ROOT}/.github/agentic-review.yml"
}

# Line number of the first line matching a pattern, or empty.
line_of() {
  grep -n -- "$2" "$1" | head -1 | cut -d: -f1
}

@test "the Claude family marks itself started before the model is invoked" {
  started="$(line_of "$CLAUDE_WF" 'phase: started')"
  model="$(line_of "$CLAUDE_WF" 'uses: anthropics/claude-code-action')"
  [ -n "$started" ]
  [ -n "$model" ]
  [ "$started" -lt "$model" ]
}

@test "the Gemini family marks itself started before the model is invoked" {
  started="$(line_of "$GEMINI_WF" 'phase: started')"
  model="$(line_of "$GEMINI_WF" 'name: Run Gemini PR review')"
  [ -n "$started" ]
  [ -n "$model" ]
  [ "$started" -lt "$model" ]
}

@test "every reviewer family publishes a terminal state when it fails" {
  # Without an error state a crashed reviewer is indistinguishable from one
  # still thinking, and the gate holds every merge until the deadline expires.
  grep -q 'if: always()' "$CLAUDE_WF"
  grep -q 'phase=error' "$GEMINI_WF"
  grep -q 'PhaseError' "${REPO_ROOT}/internal/prreviewer/reviewer.go"
}

@test "the merge-blocking gate invokes no model" {
  # The whole point of a label-only gate: a quota incident must degrade the
  # review, never the ability to decide whether a review happened.
  ! grep -qiE 'anthropic|claude-code-action|generativelanguage|openai|gemini-cli' "$GATE_WF"
}

@test "the gate publishes the commit status the ruleset requires" {
  context="$(grep -oE 'agentic-review/gate' "${REPO_ROOT}/.github/rulesets/main.json" | head -1)"
  [ "$context" = "agentic-review/gate" ]
  grep -q -- '--post-status' "$GATE_WF"
}

@test "a push invalidates every stale review label before the gate evaluates" {
  # The labels are only head-bound because this job clears them. The gate must
  # depend on it, or an approval of the previous diff survives the push.
  grep -q "github.event.action == 'synchronize'" "$GATE_WF"
  grep -q 'startswith("agentic-review:")' "$GATE_WF"
  grep -q 'needs: \[invalidate\]' "$GATE_WF"
}

@test "the shipped policy keeps a single reviewer outage from wedging the queue" {
  grep -qE '^quorum: 2$' "$POLICY"
  for family in claude codex gemini; do
    grep -qE "^  - ${family}$" "$POLICY"
  done
}

@test "the policy declares both deadlines the degradation rule needs" {
  grep -qE '^verdict-timeout: [0-9]+' "$POLICY"
  grep -qE '^dispatch-timeout: [0-9]+' "$POLICY"
}
