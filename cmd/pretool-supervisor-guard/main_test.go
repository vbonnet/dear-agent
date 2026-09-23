package main

import (
	"bytes"
	"strings"
	"testing"
)

// testHome is a home directory that is deliberately not under any temp-dir
// carveout, so paths beneath it are judged by the policy rather than waved
// through as scratch space. It need not exist on disk.
const testHome = "/Users/tester"

// supervisorEnv is the environment of a supervisor session spawned the way
// vroom-dispatch spawns one: `agm session new vroom-meta-orchestrator`, which
// sets the session name and not AGM_SUPERVISOR_ID.
func supervisorEnv(key string) string {
	switch key {
	case "HOME":
		return testHome
	case "AGM_SESSION_NAME":
		return "vroom-meta-orchestrator"
	}
	return ""
}

// workerEnv is an ordinary worker session: same home, no supervisor marker.
func workerEnv(key string) string {
	switch key {
	case "HOME":
		return testHome
	case "AGM_SESSION_NAME":
		return "worker-ce-1234"
	}
	return ""
}

func runHook(t *testing.T, envelope string, getenv func(string) string) (int, string) {
	t.Helper()
	// fsguard.New() resolves the home from the process environment, so the
	// test process's HOME must match the one the fake getenv reports.
	t.Setenv("HOME", testHome)
	var errBuf bytes.Buffer
	code := run(strings.NewReader(envelope), &errBuf, getenv)
	return code, errBuf.String()
}

// TestSupervisorEditIsBlocked is the end-to-end regression for the incident:
// the Meta-Orchestrator tried to Edit SPEC.md and wedged on a permission modal.
// Through the hook, that call is refused before a modal can appear.
func TestSupervisorEditIsBlocked(t *testing.T) {
	envelope := `{
		"tool_name": "Edit",
		"cwd": "/Users/tester/worktrees/dear-agent/b",
		"tool_input": {"file_path": "/Users/tester/worktrees/dear-agent/b/SPEC.md"}
	}`

	code, stderr := runHook(t, envelope, supervisorEnv)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 (hard block); stderr: %s", code, stderr)
	}
	for _, want := range []string{"Delegate", "vroom-dispatch-direct", "permission prompt"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("guidance is missing %q; got:\n%s", want, stderr)
		}
	}
}

// TestWorkerIsUnaffected is the other half of the contract: the identical call
// from a worker session passes through, so the guard cannot slow delivery.
func TestWorkerIsUnaffected(t *testing.T) {
	envelopes := []struct {
		name string
		json string
	}{
		{"edit", `{"tool_name":"Edit","cwd":"/Users/tester/worktrees/dear-agent/b",
			"tool_input":{"file_path":"/Users/tester/worktrees/dear-agent/b/SPEC.md"}}`},
		{"write", `{"tool_name":"Write","cwd":"/Users/tester/worktrees/dear-agent/b",
			"tool_input":{"file_path":"/Users/tester/worktrees/dear-agent/b/NEW.md"}}`},
		{"commit", `{"tool_name":"Bash","cwd":"/Users/tester/worktrees/dear-agent/b",
			"tool_input":{"command":"git commit -m wip"}}`},
		{"redirect", `{"tool_name":"Bash","cwd":"/Users/tester/worktrees/dear-agent/b",
			"tool_input":{"command":"echo x > SPEC.md"}}`},
	}

	for _, e := range envelopes {
		t.Run(e.name, func(t *testing.T) {
			code, stderr := runHook(t, e.json, workerEnv)
			if code != 0 {
				t.Errorf("worker call blocked with exit %d; the guard must be role-scoped.\n%s", code, stderr)
			}
		})
	}
}

func TestSupervisorToolCoverage(t *testing.T) {
	tests := []struct {
		name     string
		envelope string
		wantCode int
	}{
		{
			name: "Write into a worktree is blocked",
			envelope: `{"tool_name":"Write","cwd":"/Users/tester/worktrees/dear-agent/b",
				"tool_input":{"file_path":"/Users/tester/worktrees/dear-agent/b/NEW.md"}}`,
			wantCode: 2,
		},
		{
			name: "NotebookEdit is blocked through notebook_path",
			envelope: `{"tool_name":"NotebookEdit","cwd":"/Users/tester/worktrees/dear-agent/b",
				"tool_input":{"notebook_path":"/Users/tester/worktrees/dear-agent/b/a.ipynb"}}`,
			wantCode: 2,
		},
		{
			name: "a git commit is blocked",
			envelope: `{"tool_name":"Bash","cwd":"/Users/tester/worktrees/dear-agent/b",
				"tool_input":{"command":"git commit -am wip"}}`,
			wantCode: 2,
		},
		{
			name: "a shell write is blocked",
			envelope: `{"tool_name":"Bash","cwd":"/Users/tester/worktrees/dear-agent/b",
				"tool_input":{"command":"sed -i '' s/a/b/ SPEC.md"}}`,
			wantCode: 2,
		},
		{
			name: "the heartbeat still runs",
			envelope: `{"tool_name":"Bash","cwd":"/Users/tester",
				"tool_input":{"command":"agm supervisor heartbeat --id vroom-meta-orchestrator"}}`,
			wantCode: 0,
		},
		{
			name: "the ready queue still runs",
			envelope: `{"tool_name":"Bash","cwd":"/Users/tester",
				"tool_input":{"command":"bd --db ~/beads/context-engine/.beads --dolt-auto-commit on ready --json"}}`,
			wantCode: 0,
		},
		{
			name: "dispatch still runs",
			envelope: `{"tool_name":"Bash","cwd":"/Users/tester",
				"tool_input":{"command":"~/go/bin/vroom-dispatch-direct -db ~/beads/context-engine/.beads -repo vbonnet/dear-agent"}}`,
			wantCode: 0,
		},
		{
			name: "reading a repository file is untouched",
			envelope: `{"tool_name":"Read","cwd":"/Users/tester/worktrees/dear-agent/b",
				"tool_input":{"file_path":"/Users/tester/worktrees/dear-agent/b/SPEC.md"}}`,
			wantCode: 0,
		},
		{
			name: "writing the heartbeat file is allowed",
			envelope: `{"tool_name":"Write","cwd":"/Users/tester",
				"tool_input":{"file_path":"/Users/tester/.agm/vroom/heartbeat/meta-o.json"}}`,
			wantCode: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stderr := runHook(t, tt.envelope, supervisorEnv)
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d; stderr: %s", code, tt.wantCode, stderr)
			}
		})
	}
}

// TestFailsOpen pins the safety property: nothing the hook cannot read may ever
// block a tool call, since a guard bug would otherwise wedge the supervisor it
// exists to protect.
func TestFailsOpen(t *testing.T) {
	tests := []struct {
		name     string
		envelope string
		getenv   func(string) string
	}{
		{"malformed JSON", `{not json`, supervisorEnv},
		{"empty input", ``, supervisorEnv},
		{"unknown tool", `{"tool_name":"WebFetch","tool_input":{"url":"https://example.com"}}`, supervisorEnv},
		{"empty path", `{"tool_name":"Edit","cwd":"/Users/tester","tool_input":{}}`, supervisorEnv},
		{"empty command", `{"tool_name":"Bash","cwd":"/Users/tester","tool_input":{"command":""}}`, supervisorEnv},
		{
			"unparseable command",
			`{"tool_name":"Bash","cwd":"/Users/tester","tool_input":{"command":"echo \"unterminated"}}`,
			supervisorEnv,
		},
		{"no environment markers", `{"tool_name":"Edit","cwd":"/Users/tester",
			"tool_input":{"file_path":"/Users/tester/worktrees/x/a.go"}}`, func(string) string { return "" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stderr := runHook(t, tt.envelope, tt.getenv)
			if code != 0 {
				t.Errorf("exit code = %d, want 0 (fail open); stderr: %s", code, stderr)
			}
		})
	}
}

// TestBlockIsNeverAnAsk is the property that separates this hook from the write
// guards: it must never emit a permission decision that raises a modal, because
// a modal in a detached supervisor is the failure being prevented. A block is
// exit 2 with guidance on stderr and nothing on stdout.
func TestBlockIsNeverAnAsk(t *testing.T) {
	t.Setenv("FSGUARD_ENFORCEMENT", "ask")

	envelope := `{"tool_name":"Edit","cwd":"/Users/tester/worktrees/dear-agent/b",
		"tool_input":{"file_path":"/Users/tester/worktrees/dear-agent/b/SPEC.md"}}`

	code, stderr := runHook(t, envelope, supervisorEnv)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2 even under FSGUARD_ENFORCEMENT=ask", code)
	}
	if strings.Contains(stderr, "permissionDecision") {
		t.Errorf("hook emitted a permission decision; it must hard-block:\n%s", stderr)
	}
}

// TestSupervisorRunSpawnPathIsDetected covers the other spawn path: `agm
// supervisor run` marks the child with AGM_SUPERVISOR_ID and no session name,
// and must be guarded identically.
func TestSupervisorRunSpawnPathIsDetected(t *testing.T) {
	getenv := func(key string) string {
		switch key {
		case "HOME":
			return testHome
		case "AGM_SUPERVISOR_ID":
			return "vroom-overseer"
		}
		return ""
	}

	envelope := `{"tool_name":"Edit","cwd":"/Users/tester/worktrees/dear-agent/b",
		"tool_input":{"file_path":"/Users/tester/worktrees/dear-agent/b/SPEC.md"}}`

	code, stderr := runHook(t, envelope, getenv)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2; AGM_SUPERVISOR_ID must be honoured", code)
	}
	if !strings.Contains(stderr, "vroom-overseer") {
		t.Errorf("guidance should name the supervisor; got:\n%s", stderr)
	}
}
