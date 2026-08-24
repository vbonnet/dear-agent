package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
	"github.com/vbonnet/dear-agent/internal/specguard"
)

func TestRunProvidesCooperativeTerminalReminderForValidStagedContract(t *testing.T) {
	root := stagedContractRepository(t)
	for _, test := range []struct {
		provider       string
		input          string
		wantDecision   string
		wantReason     bool
		wantSystemCopy bool
	}{
		{provider: "claude", input: `{}`, wantDecision: "block", wantReason: true},
		{provider: "codex", input: `{}`, wantDecision: "block", wantReason: true, wantSystemCopy: true},
		{provider: "pi", input: `{}`, wantDecision: "block", wantReason: true},
		{provider: "opencode", input: "", wantDecision: "block", wantReason: true, wantSystemCopy: true},
		{provider: "antigravity", input: `{}`, wantDecision: "continue", wantReason: true},
	} {
		t.Run(test.provider, func(t *testing.T) {
			var output bytes.Buffer
			if got := run([]string{"--root", root, "--provider", test.provider, "--event", "Stop"}, strings.NewReader(test.input), &output, &bytes.Buffer{}); got != 0 {
				t.Fatalf("run() = %d output=%s", got, output.String())
			}
			response := decodeResponse(t, output.Bytes())
			if response.Decision != test.wantDecision {
				t.Fatalf("response = %#v, want decision %q", response, test.wantDecision)
			}
			if test.wantReason && response.Reason != stagedSPECReminderMessage {
				t.Fatalf("response reason = %q, want shared reminder", response.Reason)
			}
			if test.wantSystemCopy && response.SystemMessage != response.Reason {
				t.Fatalf("response = %#v, want native top-level reminder copy", response)
			}
			assertReminderText(t, response.Reason)
		})
	}
}

func TestRunPreservesProviderNativeAllowOutcome(t *testing.T) {
	root := baseRepository(t)
	for _, provider := range []string{"claude", "codex", "pi", "opencode", "antigravity"} {
		t.Run(provider, func(t *testing.T) {
			input := `{}`
			if provider == "opencode" {
				input = ""
			}
			var output bytes.Buffer
			if got := run([]string{"--root", root, "--provider", provider, "--event", "Stop"}, strings.NewReader(input), &output, &bytes.Buffer{}); got != 0 {
				t.Fatalf("run() = %d output=%s", got, output.String())
			}
			response := decodeResponse(t, output.Bytes())
			if provider == "antigravity" {
				if response.Decision != "allow" || response.Reason != "" {
					t.Fatalf("response = %#v, want native allow", response)
				}
				return
			}
			if response != (hookResponse{}) {
				t.Fatalf("response = %#v, want empty native allow envelope", response)
			}
		})
	}
}

func TestRunPreservesProviderNativeBlockOutcome(t *testing.T) {
	root := baseRepository(t)
	writeFile(t, root, "pkg/example/SPEC.md", "untracked mutable contract\n")
	for _, test := range []struct {
		provider       string
		wantDecision   string
		wantSystemCopy bool
	}{
		{provider: "claude", wantDecision: "block"},
		{provider: "codex", wantDecision: "block", wantSystemCopy: true},
		{provider: "pi", wantDecision: "block"},
		{provider: "opencode", wantDecision: "block", wantSystemCopy: true},
		{provider: "antigravity", wantDecision: "continue"},
	} {
		t.Run(test.provider, func(t *testing.T) {
			input := `{}`
			if test.provider == "opencode" {
				input = ""
			}
			var output bytes.Buffer
			if got := run([]string{"--root", root, "--provider", test.provider, "--event", "Stop"}, strings.NewReader(input), &output, &bytes.Buffer{}); got != 0 {
				t.Fatalf("run() = %d output=%s", got, output.String())
			}
			response := decodeResponse(t, output.Bytes())
			if response.Decision != test.wantDecision || !strings.Contains(response.Reason, "dirty-governed-worktree") {
				t.Fatalf("response = %#v, want native deterministic block", response)
			}
			if test.wantSystemCopy && response.SystemMessage != response.Reason {
				t.Fatalf("response = %#v, want native top-level block copy", response)
			}
			if strings.Contains(response.Reason, "docs/spec-authoring.md") || strings.Contains(response.Reason, "spec-governance/skills/write-spec/SKILL.md") {
				t.Fatalf("blocking diagnostic falsely used successful reminder guidance: %#v", response)
			}
		})
	}
}

func TestRunYieldsInvalidInvocationWithoutEvaluatingTheGuard(t *testing.T) {
	var output bytes.Buffer
	calls := 0
	evaluate := func(context.Context, specguard.Request) specguard.Result {
		calls++
		return specguard.Result{Decision: specguard.DecisionBlock}
	}
	if got := runWithEvaluator([]string{"--root", ".", "--event", "PreToolUse"}, bytes.NewReader(nil), &output, &bytes.Buffer{}, evaluate); got != 0 {
		t.Fatalf("runWithEvaluator() = %d", got)
	}
	response := decodeResponse(t, output.Bytes())
	if response.Decision != "" || response.SystemMessage == "" || calls != 0 {
		t.Fatalf("response = %#v, evaluation calls = %d", response, calls)
	}
}

func TestRunYieldsOversizedOrMalformedInputWithoutEvaluatingTheGuard(t *testing.T) {
	evaluate := func(context.Context, specguard.Request) specguard.Result {
		t.Fatal("guard evaluated invalid transport input")
		return specguard.Result{}
	}
	t.Run("oversized", func(t *testing.T) {
		var output bytes.Buffer
		input := bytes.Repeat([]byte("x"), maxHookInputBytes+1)
		if got := runWithEvaluator([]string{"--root", ".", "--provider", "codex", "--event", "Stop"}, bytes.NewReader(input), &output, &bytes.Buffer{}, evaluate); got != 0 {
			t.Fatalf("runWithEvaluator() = %d", got)
		}
		response := decodeResponse(t, output.Bytes())
		if response.Decision != "" || response.SystemMessage == "" {
			t.Fatalf("response = %#v, want advisory yield", response)
		}
	})
	for _, test := range []struct {
		name      string
		provider  string
		event     string
		input     string
		wantAllow bool
		wantHook  bool
	}{
		{name: "claude malformed object", provider: "claude", event: "Stop", input: `{`},
		{name: "codex array", provider: "codex", event: "Stop", input: `[]`},
		{name: "opencode null", provider: "opencode", event: "Stop", input: `null`},
		{name: "pi empty", provider: "pi", event: "SubagentStop", input: "", wantHook: true},
		{name: "antigravity null", provider: "antigravity", event: "Stop", input: `null`, wantAllow: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			if got := runWithEvaluator([]string{"--root", ".", "--provider", test.provider, "--event", test.event}, strings.NewReader(test.input), &output, &bytes.Buffer{}, evaluate); got != 0 {
				t.Fatalf("runWithEvaluator() = %d", got)
			}
			response := decodeResponse(t, output.Bytes())
			switch {
			case test.wantAllow && (response.Decision != "allow" || response.Reason != "" || response.SystemMessage != ""):
				t.Fatalf("response = %#v, want identity-less native allow without advisory continuation", response)
			case test.wantHook && (response.Decision != "" || response.HookSpecificOutput == nil || response.HookSpecificOutput.HookEventName != test.event || response.HookSpecificOutput.AdditionalContext == ""):
				t.Fatalf("response = %#v, want hook-specific advisory yield", response)
			case !test.wantAllow && !test.wantHook && (response.Decision != "" || response.SystemMessage == ""):
				t.Fatalf("response = %#v, want top-level advisory yield", response)
			}
		})
	}
}

func TestRunCompactsOversizedNativeBlock(t *testing.T) {
	evaluate := func(context.Context, specguard.Request) specguard.Result {
		return specguard.Result{
			Decision: specguard.DecisionBlock,
			Findings: []specguard.Finding{{Code: "OVERSIZED", Message: strings.Repeat("x", maxHookOutputBytes)}},
		}
	}
	var output bytes.Buffer
	if got := runWithEvaluator([]string{"--root", t.TempDir(), "--provider", "opencode", "--event", "Stop"}, strings.NewReader(""), &output, &bytes.Buffer{}, evaluate); got != 0 {
		t.Fatalf("runWithEvaluator() = %d output=%s", got, output.String())
	}
	response := decodeResponse(t, output.Bytes())
	if output.Len() > maxHookOutputBytes || response.Decision != "block" || response.Reason == "" || response.SystemMessage != response.Reason || !strings.Contains(response.Reason, "exceeded its safety limit") {
		t.Fatalf("output bytes = %d, response = %#v", output.Len(), response)
	}
}

func TestEmitJSONPreservesNativeOutcomeWithinOutputBound(t *testing.T) {
	feedbackID := strings.Repeat("a", 64)
	for _, test := range []struct {
		name     string
		response hookResponse
		assert   func(*testing.T, hookResponse)
	}{
		{
			name:     "block remains block",
			response: hookResponse{Decision: "block", Reason: strings.Repeat("x", maxHookOutputBytes), DearAgentSpecFeedbackID: feedbackID},
			assert: func(t *testing.T, response hookResponse) {
				if response.Decision != "block" || response.DearAgentSpecFeedbackID != feedbackID || !strings.Contains(response.Reason, "exceeded its safety limit") {
					t.Fatalf("response = %#v", response)
				}
			},
		},
		{
			name:     "Antigravity continuation remains continuation",
			response: hookResponse{Decision: "continue", Reason: strings.Repeat("x", maxHookOutputBytes)},
			assert: func(t *testing.T, response hookResponse) {
				if response.Decision != "continue" || !strings.Contains(response.Reason, "exceeded its safety limit") {
					t.Fatalf("response = %#v", response)
				}
			},
		},
		{
			name:     "top-level yield remains yield",
			response: hookResponse{SystemMessage: strings.Repeat("x", maxHookOutputBytes), DearAgentSpecFeedbackID: feedbackID},
			assert: func(t *testing.T, response hookResponse) {
				if response.Decision != "" || response.Reason != "" || response.DearAgentSpecFeedbackID != feedbackID || !strings.Contains(response.SystemMessage, "yielding") {
					t.Fatalf("response = %#v", response)
				}
			},
		},
		{
			name: "hook-specific yield keeps event and identity",
			response: hookResponse{
				HookSpecificOutput:      &hookSpecificOutput{HookEventName: "Stop", AdditionalContext: strings.Repeat("x", maxHookOutputBytes)},
				DearAgentSpecFeedbackID: feedbackID,
			},
			assert: func(t *testing.T, response hookResponse) {
				if response.Decision != "" || response.DearAgentSpecFeedbackID != feedbackID || response.HookSpecificOutput == nil || response.HookSpecificOutput.HookEventName != "Stop" || !strings.Contains(response.HookSpecificOutput.AdditionalContext, "yielding") {
					t.Fatalf("response = %#v", response)
				}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			if got := emitJSON(&output, test.response); got != 0 {
				t.Fatalf("emitJSON() = %d", got)
			}
			if output.Len() > maxHookOutputBytes {
				t.Fatalf("output length = %d, limit = %d", output.Len(), maxHookOutputBytes)
			}
			test.assert(t, decodeResponse(t, output.Bytes()))
		})
	}
	if got := emitJSON(shortWriter{remaining: 1}, hookResponse{Decision: "block", Reason: "must be complete"}); got != 1 {
		t.Fatalf("emitJSON(short writer) = %d, want 1", got)
	}
}

type shortWriter struct{ remaining int }

func (writer shortWriter) Write(payload []byte) (int, error) {
	if writer.remaining >= len(payload) {
		return len(payload), nil
	}
	return writer.remaining, nil
}

func assertReminderText(t *testing.T, text string) {
	t.Helper()
	for _, phrase := range []string{"Cooperative", "docs/spec-authoring.md", "spec-governance/skills/write-spec/SKILL.md", "reference that skill instead of copying its body", "does not claim native skill discovery", "mutable checkout hook", "not tamper-resistant", "separately reviewed changed-SPEC CI and provider rollout", "does not attest that enforcement is deployed, has run, or is provider-required"} {
		if !strings.Contains(text, phrase) {
			t.Errorf("reminder %q omits %q", text, phrase)
		}
	}
}

func decodeResponse(t *testing.T, encoded []byte) hookResponse {
	t.Helper()
	var response hookResponse
	if err := json.Unmarshal(encoded, &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func baseRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	sandbox := gittest.New(t)
	sandbox.Run(t, root, "init")
	writeFile(t, root, "README.md", "base\n")
	sandbox.Run(t, root, "add", "--", "README.md")
	sandbox.Run(t, root, "commit", "-m", "base")
	return root
}

func stagedContractRepository(t *testing.T) string {
	t.Helper()
	root := baseRepository(t)
	sandbox := gittest.New(t)
	writeFile(t, root, "pkg/example/SPEC.md", "# Example\n\n**EXAMPLE-01** When the guard evaluates a contract, the system shall preserve reciprocal BDD traceability.\n\n## BDD Traceability\n\n- Feature: `agm/test/bdd/features/example.feature`\n")
	writeFile(t, root, "agm/test/bdd/features/example.feature", "# SPEC: pkg/example/SPEC.md\nFeature: Example\n  Scenario: Contract remains valid\n    Given a staged contract\n    Then the guard reports its result\n")
	sandbox.Run(t, root, "add", "--", "pkg/example/SPEC.md", "agm/test/bdd/features/example.feature")
	return root
}

func writeFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
