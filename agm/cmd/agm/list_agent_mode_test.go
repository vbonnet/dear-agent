package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

// captureStdout is defined in acceptance_test.go and reused here.

func sampleListResult() *ops.ListSessionsResult {
	return &ops.ListSessionsResult{
		Operation: "list_sessions",
		Sessions: []ops.SessionSummary{
			{
				ID:        "11111111-2222-3333-4444-555555555555",
				Name:      "worker-alpha",
				Status:    "active",
				Harness:   "claude-code",
				Workspace: "oss",
				Project:   "/Users/vbonnet/.agm/sandboxes/abcd-1234/merged",
				Tags:      []string{"role:worker"},
			},
			{
				ID:        "66666666-7777-8888-9999-000000000000",
				Name:      "worker-beta",
				Status:    "stopped",
				Harness:   "gemini-cli",
				Workspace: "oss",
				Project:   "/Users/vbonnet/code/dear-agent",
			},
		},
		Total: 2,
	}
}

func TestSessionList_JSONFieldsApplyToSessionRows(t *testing.T) {
	result := sampleListResult()
	var out string
	withGlobals(t, ModeAgent, []string{"name", "status", "harness", "workspace", "tags"}, func() {
		out = captureStdout(t, func() {
			if err := printListJSON(result); err != nil {
				t.Fatalf("printListJSON: %v", err)
			}
		})
	})

	if strings.TrimSpace(out) == "{}" {
		t.Fatal("session row fields must not collapse list output to {}")
	}
	var decoded struct {
		Sessions []map[string]any `json:"sessions"`
		Total    int              `json:"total"`
	}
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if decoded.Total != 2 || len(decoded.Sessions) != 2 {
		t.Fatalf("decoded list = total %d sessions %d, want 2/2: %s", decoded.Total, len(decoded.Sessions), out)
	}
	first := decoded.Sessions[0]
	for _, want := range []string{"name", "status", "harness", "workspace", "tags"} {
		if _, ok := first[want]; !ok {
			t.Fatalf("filtered row missing %q: %#v", want, first)
		}
	}
	if _, ok := first["operation"]; ok {
		t.Fatalf("row field filtering leaked top-level operation into row: %#v", first)
	}
}

// withGlobals saves and restores the package-level output globals that the
// agent-mode rendering path reads, so tests don't leak state into one another.
func withGlobals(t *testing.T, mode OutputMode, fields []string, fn func()) {
	t.Helper()
	origMode, origFields := outputMode, fieldsFlag
	outputMode, fieldsFlag = mode, fields
	defer func() { outputMode, fieldsFlag = origMode, origFields }()
	fn()
}

// TestSessionList_AgentMode_CompactJSON verifies agent-mode output is compact
// (single line, no indentation).
func TestSessionList_AgentMode_CompactJSON(t *testing.T) {
	result := sampleListResult()
	var out string
	withGlobals(t, ModeAgent, nil, func() {
		out = captureStdout(t, func() {
			if err := printJSON(result); err != nil {
				t.Fatalf("printJSON: %v", err)
			}
		})
	})

	out = strings.TrimRight(out, "\n")
	if strings.Contains(out, "\n") {
		t.Errorf("agent-mode JSON should be single-line/compact, got multi-line:\n%s", out)
	}
	if strings.Contains(out, "  ") {
		t.Errorf("agent-mode JSON should have no indentation, got:\n%s", out)
	}
}

// TestSessionList_AgentMode_NoUUIDs verifies the raw UUID is dropped by default
// and retained only when --fields id is requested.
func TestSessionList_AgentMode_NoUUIDs(t *testing.T) {
	// Default: no --fields → IDs stripped.
	result := sampleListResult()
	applyAgentListDefaults(result, nil)
	for _, s := range result.Sessions {
		if s.ID != "" {
			t.Errorf("expected ID stripped in agent-mode default, got %q for %s", s.ID, s.Name)
		}
	}
	out := captureCompact(t, result)
	if strings.Contains(out, `"id"`) {
		t.Errorf("compact agent-mode JSON should omit id key, got:\n%s", out)
	}

	// With --fields id → IDs retained.
	result2 := sampleListResult()
	applyAgentListDefaults(result2, []string{"id"})
	for _, s := range result2.Sessions {
		if s.ID == "" {
			t.Errorf("expected ID retained with --fields id, got empty for %s", s.Name)
		}
	}
}

// captureCompact marshals via the agent-mode printJSON path and returns output.
func captureCompact(t *testing.T, result *ops.ListSessionsResult) string {
	t.Helper()
	var out string
	withGlobals(t, ModeAgent, nil, func() {
		out = captureStdout(t, func() {
			if err := printJSON(result); err != nil {
				t.Fatalf("printJSON: %v", err)
			}
		})
	})
	return out
}

// TestSessionList_AgentMode_PathBasename verifies sandbox paths collapse to the
// UUID and other paths reduce to their basename.
func TestSessionList_AgentMode_PathBasename(t *testing.T) {
	cases := map[string]string{
		"/Users/vbonnet/.agm/sandboxes/abcd-1234/merged": "abcd-1234",
		"/Users/x/.agm/sandboxes/uuid-only":              "uuid-only",
		"/Users/vbonnet/code/dear-agent":                 "dear-agent",
		"":                                               "",
	}
	for in, want := range cases {
		if got := sanitizeSandboxPath(in); got != want {
			t.Errorf("sanitizeSandboxPath(%q) = %q, want %q", in, got, want)
		}
	}

	// End-to-end through the transform.
	result := sampleListResult()
	applyAgentListDefaults(result, nil)
	if got := result.Sessions[0].Project; got != "abcd-1234" {
		t.Errorf("sandbox project = %q, want %q", got, "abcd-1234")
	}
	if got := result.Sessions[1].Project; got != "dear-agent" {
		t.Errorf("non-sandbox project = %q, want basename %q", got, "dear-agent")
	}
}

// TestSessionList_HumanMode_FullOutput verifies human mode is unchanged:
// indented JSON, UUIDs and full paths present.
func TestSessionList_HumanMode_FullOutput(t *testing.T) {
	result := sampleListResult()
	var out string
	withGlobals(t, ModeHuman, nil, func() {
		out = captureStdout(t, func() {
			if err := printJSON(result); err != nil {
				t.Fatalf("printJSON: %v", err)
			}
		})
	})

	if !strings.Contains(out, "\n  ") {
		t.Errorf("human-mode JSON should be indented, got:\n%s", out)
	}
	if !strings.Contains(out, "11111111-2222-3333-4444-555555555555") {
		t.Errorf("human-mode JSON should keep UUIDs, got:\n%s", out)
	}
	if !strings.Contains(out, "/Users/vbonnet/.agm/sandboxes/abcd-1234/merged") {
		t.Errorf("human-mode JSON should keep full project path, got:\n%s", out)
	}
}
