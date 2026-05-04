package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// parseWorktreeAdd tests
// ---------------------------------------------------------------------------

func TestParseWorktreeAdd_BasicPath(t *testing.T) {
	event := parseWorktreeAdd("git worktree add /tmp/my-worktree")
	if event == nil {
		t.Fatal("Expected event, got nil")
	}
	if event.Operation != "add" {
		t.Errorf("Expected operation 'add', got %q", event.Operation)
	}
	if event.WorktreePath != "/tmp/my-worktree" {
		t.Errorf("Expected path '/tmp/my-worktree', got %q", event.WorktreePath)
	}
}

func TestParseWorktreeAdd_WithBranchFlag(t *testing.T) {
	event := parseWorktreeAdd("git worktree add /tmp/wt -b feature-branch")
	if event == nil {
		t.Fatal("Expected event, got nil")
	}
	if event.WorktreePath != "/tmp/wt" {
		t.Errorf("Expected path '/tmp/wt', got %q", event.WorktreePath)
	}
	if event.Branch != "feature-branch" {
		t.Errorf("Expected branch 'feature-branch', got %q", event.Branch)
	}
}

func TestParseWorktreeAdd_WithBranchArg(t *testing.T) {
	event := parseWorktreeAdd("git worktree add /tmp/wt existing-branch")
	if event == nil {
		t.Fatal("Expected event, got nil")
	}
	if event.WorktreePath != "/tmp/wt" {
		t.Errorf("Expected path '/tmp/wt', got %q", event.WorktreePath)
	}
	if event.Branch != "existing-branch" {
		t.Errorf("Expected branch 'existing-branch', got %q", event.Branch)
	}
}

func TestParseWorktreeAdd_WithCFlag(t *testing.T) {
	event := parseWorktreeAdd("git -C ~/repo worktree add /tmp/wt -b feat")
	if event == nil {
		t.Fatal("Expected event, got nil")
	}
	if event.RepoPath != "~/repo" {
		t.Errorf("Expected repo '~/repo', got %q", event.RepoPath)
	}
	if event.WorktreePath != "/tmp/wt" {
		t.Errorf("Expected path '/tmp/wt', got %q", event.WorktreePath)
	}
	if event.Branch != "feat" {
		t.Errorf("Expected branch 'feat', got %q", event.Branch)
	}
}

func TestParseWorktreeAdd_NoMatch(t *testing.T) {
	event := parseWorktreeAdd("git status")
	if event != nil {
		t.Errorf("Expected nil for non-worktree command, got %+v", event)
	}
}

func TestParseWorktreeAdd_WithDetachFlag(t *testing.T) {
	event := parseWorktreeAdd("git worktree add --detach /tmp/wt HEAD")
	if event == nil {
		t.Fatal("Expected event, got nil")
	}
	if event.WorktreePath != "/tmp/wt" {
		t.Errorf("Expected path '/tmp/wt', got %q", event.WorktreePath)
	}
}

func TestParseWorktreeAdd_LeadingTrailingWhitespace(t *testing.T) {
	event := parseWorktreeAdd("  git worktree add /tmp/wt  ")
	if event == nil {
		t.Fatal("Expected event, got nil")
	}
	if event.WorktreePath != "/tmp/wt" {
		t.Errorf("Expected path '/tmp/wt', got %q", event.WorktreePath)
	}
}

func TestParseWorktreeAdd_MultipleSpaces(t *testing.T) {
	event := parseWorktreeAdd("git  worktree  add  /tmp/wt")
	if event == nil {
		t.Fatal("Expected event, got nil")
	}
	if event.WorktreePath != "/tmp/wt" {
		t.Errorf("Expected path '/tmp/wt', got %q", event.WorktreePath)
	}
}

func TestParseWorktreeAdd_RelativePath(t *testing.T) {
	event := parseWorktreeAdd("git worktree add ../my-worktree -b feat")
	if event == nil {
		t.Fatal("Expected event, got nil")
	}
	if event.WorktreePath != "../my-worktree" {
		t.Errorf("Expected path '../my-worktree', got %q", event.WorktreePath)
	}
	if event.Branch != "feat" {
		t.Errorf("Expected branch 'feat', got %q", event.Branch)
	}
}

func TestParseWorktreeAdd_EmptyString(t *testing.T) {
	event := parseWorktreeAdd("")
	if event != nil {
		t.Errorf("Expected nil for empty string, got %+v", event)
	}
}

func TestParseWorktreeAdd_NoBranch(t *testing.T) {
	event := parseWorktreeAdd("git worktree add /tmp/wt")
	if event == nil {
		t.Fatal("Expected event, got nil")
	}
	if event.Branch != "" {
		t.Errorf("Expected empty branch, got %q", event.Branch)
	}
	if event.RepoPath != "" {
		t.Errorf("Expected empty repo path, got %q", event.RepoPath)
	}
}

func TestParseWorktreeAdd_BranchWithSlash(t *testing.T) {
	event := parseWorktreeAdd("git worktree add /tmp/wt -b feature/my-feature")
	if event == nil {
		t.Fatal("Expected event, got nil")
	}
	if event.Branch != "feature/my-feature" {
		t.Errorf("Expected branch 'feature/my-feature', got %q", event.Branch)
	}
}

func TestParseWorktreeAdd_WithForceFlag(t *testing.T) {
	event := parseWorktreeAdd("git worktree add --force /tmp/wt -b feat")
	if event == nil {
		t.Fatal("Expected event, got nil")
	}
	if event.WorktreePath != "/tmp/wt" {
		t.Errorf("Expected path '/tmp/wt', got %q", event.WorktreePath)
	}
}

// ---------------------------------------------------------------------------
// parseWorktreeRemove tests
// ---------------------------------------------------------------------------

func TestParseWorktreeRemove_Basic(t *testing.T) {
	event := parseWorktreeRemove("git worktree remove /tmp/my-worktree")
	if event == nil {
		t.Fatal("Expected event, got nil")
	}
	if event.Operation != "remove" {
		t.Errorf("Expected operation 'remove', got %q", event.Operation)
	}
	if event.WorktreePath != "/tmp/my-worktree" {
		t.Errorf("Expected path '/tmp/my-worktree', got %q", event.WorktreePath)
	}
}

func TestParseWorktreeRemove_WithForce(t *testing.T) {
	event := parseWorktreeRemove("git worktree remove --force /tmp/wt")
	if event == nil {
		t.Fatal("Expected event, got nil")
	}
	if event.WorktreePath != "/tmp/wt" {
		t.Errorf("Expected path '/tmp/wt', got %q", event.WorktreePath)
	}
}

func TestParseWorktreeRemove_WithCFlag(t *testing.T) {
	event := parseWorktreeRemove("git -C ~/repo worktree remove /tmp/wt")
	if event == nil {
		t.Fatal("Expected event, got nil")
	}
	if event.RepoPath != "~/repo" {
		t.Errorf("Expected repo '~/repo', got %q", event.RepoPath)
	}
	if event.WorktreePath != "/tmp/wt" {
		t.Errorf("Expected path '/tmp/wt', got %q", event.WorktreePath)
	}
}

func TestParseWorktreeRemove_NoMatch(t *testing.T) {
	event := parseWorktreeRemove("git worktree list")
	if event != nil {
		t.Errorf("Expected nil for non-remove command, got %+v", event)
	}
}

func TestParseWorktreeRemove_EmptyString(t *testing.T) {
	event := parseWorktreeRemove("")
	if event != nil {
		t.Errorf("Expected nil for empty string, got %+v", event)
	}
}

func TestParseWorktreeRemove_LeadingTrailingWhitespace(t *testing.T) {
	event := parseWorktreeRemove("  git worktree remove /tmp/wt  ")
	if event == nil {
		t.Fatal("Expected event, got nil")
	}
	if event.WorktreePath != "/tmp/wt" {
		t.Errorf("Expected path '/tmp/wt', got %q", event.WorktreePath)
	}
}

func TestParseWorktreeRemove_WithCFlagAndForce(t *testing.T) {
	event := parseWorktreeRemove("git -C /repo worktree remove --force /tmp/wt")
	if event == nil {
		t.Fatal("Expected event, got nil")
	}
	if event.RepoPath != "/repo" {
		t.Errorf("Expected repo '/repo', got %q", event.RepoPath)
	}
	if event.WorktreePath != "/tmp/wt" {
		t.Errorf("Expected path '/tmp/wt', got %q", event.WorktreePath)
	}
}

func TestParseWorktreeRemove_BranchAlwaysEmpty(t *testing.T) {
	event := parseWorktreeRemove("git worktree remove /tmp/wt")
	if event == nil {
		t.Fatal("Expected event, got nil")
	}
	if event.Branch != "" {
		t.Errorf("Expected empty branch for remove, got %q", event.Branch)
	}
}

// ---------------------------------------------------------------------------
// detectWorktreeEvent tests
// ---------------------------------------------------------------------------

func TestDetectWorktreeEvent_Add(t *testing.T) {
	event := detectWorktreeEvent("git worktree add /tmp/wt -b feat")
	if event == nil {
		t.Fatal("Expected event, got nil")
	}
	if event.Operation != "add" {
		t.Errorf("Expected 'add', got %q", event.Operation)
	}
}

func TestDetectWorktreeEvent_Remove(t *testing.T) {
	event := detectWorktreeEvent("git worktree remove /tmp/wt")
	if event == nil {
		t.Fatal("Expected event, got nil")
	}
	if event.Operation != "remove" {
		t.Errorf("Expected 'remove', got %q", event.Operation)
	}
}

func TestDetectWorktreeEvent_Unrelated(t *testing.T) {
	tests := []string{
		"git status",
		"git commit -m 'msg'",
		"git branch -d foo",
		"ls -la",
		"",
	}
	for _, cmd := range tests {
		event := detectWorktreeEvent(cmd)
		if event != nil {
			t.Errorf("Expected nil for command %q, got %+v", cmd, event)
		}
	}
}

func TestDetectWorktreeEvent_List(t *testing.T) {
	event := detectWorktreeEvent("git worktree list")
	if event != nil {
		t.Errorf("Expected nil for 'worktree list', got %+v", event)
	}
}

func TestDetectWorktreeEvent_Prune(t *testing.T) {
	event := detectWorktreeEvent("git worktree prune")
	if event != nil {
		t.Errorf("Expected nil for 'worktree prune', got %+v", event)
	}
}

func TestDetectWorktreeEvent_Lock(t *testing.T) {
	event := detectWorktreeEvent("git worktree lock /tmp/wt")
	if event != nil {
		t.Errorf("Expected nil for 'worktree lock', got %+v", event)
	}
}

func TestDetectWorktreeEvent_WordWorktreeInPath(t *testing.T) {
	event := detectWorktreeEvent("cat ~/worktree-docs.txt")
	if event != nil {
		t.Errorf("Expected nil for non-git command mentioning worktree, got %+v", event)
	}
}

func TestDetectWorktreeEvent_AddWithCFlag(t *testing.T) {
	event := detectWorktreeEvent("git -C /repo worktree add /tmp/wt -b feat")
	if event == nil {
		t.Fatal("Expected event, got nil")
	}
	if event.Operation != "add" {
		t.Errorf("Expected 'add', got %q", event.Operation)
	}
	if event.RepoPath != "/repo" {
		t.Errorf("Expected repo '/repo', got %q", event.RepoPath)
	}
}

func TestDetectWorktreeEvent_RemoveWithCFlag(t *testing.T) {
	event := detectWorktreeEvent("git -C /repo worktree remove /tmp/wt")
	if event == nil {
		t.Fatal("Expected event, got nil")
	}
	if event.Operation != "remove" {
		t.Errorf("Expected 'remove', got %q", event.Operation)
	}
	if event.RepoPath != "/repo" {
		t.Errorf("Expected repo '/repo', got %q", event.RepoPath)
	}
}

// ---------------------------------------------------------------------------
// HookInput JSON parsing tests
// ---------------------------------------------------------------------------

func TestHookInput_JSONRoundTrip(t *testing.T) {
	input := HookInput{
		ToolName: "Bash",
		ToolInput: struct {
			Command string `json:"command"`
		}{
			Command: "git worktree add /tmp/wt -b feat",
		},
		ToolResult: struct {
			Stdout   string `json:"stdout"`
			Stderr   string `json:"stderr"`
			ExitCode int    `json:"exitCode"`
		}{
			Stdout:   "Preparing worktree",
			Stderr:   "",
			ExitCode: 0,
		},
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Failed to marshal HookInput: %v", err)
	}

	var parsed HookInput
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal HookInput: %v", err)
	}

	if parsed.ToolName != "Bash" {
		t.Errorf("Expected ToolName 'Bash', got %q", parsed.ToolName)
	}
	if parsed.ToolInput.Command != "git worktree add /tmp/wt -b feat" {
		t.Errorf("Expected command, got %q", parsed.ToolInput.Command)
	}
	if parsed.ToolResult.ExitCode != 0 {
		t.Errorf("Expected ExitCode 0, got %d", parsed.ToolResult.ExitCode)
	}
	if parsed.ToolResult.Stdout != "Preparing worktree" {
		t.Errorf("Expected stdout 'Preparing worktree', got %q", parsed.ToolResult.Stdout)
	}
}

func TestHookInput_MalformedJSON(t *testing.T) {
	var parsed HookInput
	err := json.Unmarshal([]byte("{bad json"), &parsed)
	if err == nil {
		t.Fatal("Expected error for malformed JSON, got nil")
	}
}

func TestHookInput_EmptyJSON(t *testing.T) {
	var parsed HookInput
	err := json.Unmarshal([]byte("{}"), &parsed)
	if err != nil {
		t.Fatalf("Expected no error for empty JSON object, got: %v", err)
	}
	if parsed.ToolName != "" {
		t.Errorf("Expected empty ToolName, got %q", parsed.ToolName)
	}
	if parsed.ToolInput.Command != "" {
		t.Errorf("Expected empty Command, got %q", parsed.ToolInput.Command)
	}
}

func TestHookInput_MissingFields(t *testing.T) {
	var parsed HookInput
	err := json.Unmarshal([]byte(`{"tool_name":"Bash"}`), &parsed)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if parsed.ToolName != "Bash" {
		t.Errorf("Expected ToolName 'Bash', got %q", parsed.ToolName)
	}
	if parsed.ToolInput.Command != "" {
		t.Errorf("Expected empty command, got %q", parsed.ToolInput.Command)
	}
}

func TestHookInput_ExtraFields(t *testing.T) {
	jsonStr := `{"tool_name":"Bash","extra_field":"ignored","tool_input":{"command":"git status","description":"test"}}`
	var parsed HookInput
	err := json.Unmarshal([]byte(jsonStr), &parsed)
	if err != nil {
		t.Fatalf("Expected no error for extra fields, got: %v", err)
	}
	if parsed.ToolName != "Bash" {
		t.Errorf("Expected ToolName 'Bash', got %q", parsed.ToolName)
	}
}

func TestHookInput_NonZeroExitCode(t *testing.T) {
	jsonStr := `{"tool_name":"Bash","tool_input":{"command":"git worktree add /tmp/fail"},"tool_result":{"exitCode":128,"stderr":"fatal: error"}}`
	var parsed HookInput
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}
	if parsed.ToolResult.ExitCode != 128 {
		t.Errorf("Expected ExitCode 128, got %d", parsed.ToolResult.ExitCode)
	}
	if parsed.ToolResult.Stderr != "fatal: error" {
		t.Errorf("Expected stderr 'fatal: error', got %q", parsed.ToolResult.Stderr)
	}
}

// ---------------------------------------------------------------------------
// run() integration tests via stdin mocking
// ---------------------------------------------------------------------------

func mockStdin(t *testing.T, data []byte) func() {
	t.Helper()
	origStdin := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	_, err = w.Write(data)
	if err != nil {
		t.Fatalf("Failed to write to pipe: %v", err)
	}
	w.Close()
	os.Stdin = r
	return func() {
		os.Stdin = origStdin
		r.Close()
	}
}

func TestRun_NonBashTool(t *testing.T) {
	input := HookInput{
		ToolName: "Read",
	}
	data, _ := json.Marshal(input)
	cleanup := mockStdin(t, data)
	defer cleanup()

	run()
}

func TestRun_FailedCommand(t *testing.T) {
	input := HookInput{
		ToolName: "Bash",
		ToolInput: struct {
			Command string `json:"command"`
		}{Command: "git worktree add /tmp/wt -b feat"},
		ToolResult: struct {
			Stdout   string `json:"stdout"`
			Stderr   string `json:"stderr"`
			ExitCode int    `json:"exitCode"`
		}{ExitCode: 1, Stderr: "fatal: error"},
	}
	data, _ := json.Marshal(input)
	cleanup := mockStdin(t, data)
	defer cleanup()

	run()
}

func TestRun_NonWorktreeCommand(t *testing.T) {
	input := HookInput{
		ToolName: "Bash",
		ToolInput: struct {
			Command string `json:"command"`
		}{Command: "git status"},
		ToolResult: struct {
			Stdout   string `json:"stdout"`
			Stderr   string `json:"stderr"`
			ExitCode int    `json:"exitCode"`
		}{ExitCode: 0},
	}
	data, _ := json.Marshal(input)
	cleanup := mockStdin(t, data)
	defer cleanup()

	run()
}

func TestRun_WorktreeAddNoSession(t *testing.T) {
	t.Setenv("CLAUDE_SESSION_ID", "")

	input := HookInput{
		ToolName: "Bash",
		ToolInput: struct {
			Command string `json:"command"`
		}{Command: "git worktree add /tmp/wt -b feat"},
		ToolResult: struct {
			Stdout   string `json:"stdout"`
			Stderr   string `json:"stderr"`
			ExitCode int    `json:"exitCode"`
		}{ExitCode: 0},
	}
	data, _ := json.Marshal(input)
	cleanup := mockStdin(t, data)
	defer cleanup()

	run()
}

func TestRun_WorktreeRemoveNoSession(t *testing.T) {
	t.Setenv("CLAUDE_SESSION_ID", "")

	input := HookInput{
		ToolName: "Bash",
		ToolInput: struct {
			Command string `json:"command"`
		}{Command: "git worktree remove /tmp/wt"},
		ToolResult: struct {
			Stdout   string `json:"stdout"`
			Stderr   string `json:"stderr"`
			ExitCode int    `json:"exitCode"`
		}{ExitCode: 0},
	}
	data, _ := json.Marshal(input)
	cleanup := mockStdin(t, data)
	defer cleanup()

	run()
}

func TestRun_MalformedStdin(t *testing.T) {
	cleanup := mockStdin(t, []byte("this is not json"))
	defer cleanup()

	run()
}

func TestRun_EmptyStdin(t *testing.T) {
	cleanup := mockStdin(t, []byte(""))
	defer cleanup()

	run()
}

func TestRun_DebugLogging(t *testing.T) {
	t.Setenv("AGM_HOOK_DEBUG", "1")
	t.Setenv("CLAUDE_SESSION_ID", "")

	input := HookInput{
		ToolName: "Bash",
		ToolInput: struct {
			Command string `json:"command"`
		}{Command: "git worktree add /tmp/wt -b feat"},
		ToolResult: struct {
			Stdout   string `json:"stdout"`
			Stderr   string `json:"stderr"`
			ExitCode int    `json:"exitCode"`
		}{ExitCode: 0},
	}
	data, _ := json.Marshal(input)
	cleanup := mockStdin(t, data)
	defer cleanup()

	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	run()

	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	os.Stderr = origStderr


	output := buf.String()
	if !strings.Contains(output, "Detected worktree add") {
		t.Errorf("Expected debug output about worktree detection, got: %q", output)
	}
}

// ---------------------------------------------------------------------------
// findAGMSession tests
// ---------------------------------------------------------------------------

func TestFindAGMSession_EmptySessionID(t *testing.T) {
	result := findAGMSession("")
	if result != "" {
		t.Errorf("Expected empty string for empty session ID, got %q", result)
	}
}

func TestFindAGMSession_NonexistentManifest(t *testing.T) {
	result := findAGMSession("nonexistent-session-id-99999")
	if result != "" {
		t.Errorf("Expected empty string for non-existent manifest, got %q", result)
	}
}

func TestFindAGMSession_ValidManifest(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Cannot get home dir")
	}

	sessionID := "test-find-agm-session-valid-12345"
	manifestDir := homeDir + "/.claude/sessions/" + sessionID
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		t.Fatalf("Failed to create manifest dir: %v", err)
	}
	defer os.RemoveAll(manifestDir)

	manifestContent := "agm_session_name: my-test-session\nother_field: value\n"
	if err := os.WriteFile(manifestDir+"/manifest.yaml", []byte(manifestContent), 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	result := findAGMSession(sessionID)
	if result != "my-test-session" {
		t.Errorf("Expected 'my-test-session', got %q", result)
	}
}

func TestFindAGMSession_ManifestWithoutAGMField(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Cannot get home dir")
	}

	sessionID := "test-find-agm-no-field-98765"
	manifestDir := homeDir + "/.claude/sessions/" + sessionID
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		t.Fatalf("Failed to create manifest dir: %v", err)
	}
	defer os.RemoveAll(manifestDir)

	if err := os.WriteFile(manifestDir+"/manifest.yaml", []byte("some_other_field: value\n"), 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	result := findAGMSession(sessionID)
	if result != "" {
		t.Errorf("Expected empty string for manifest without agm_session_name, got %q", result)
	}
}

// ---------------------------------------------------------------------------
// log() tests
// ---------------------------------------------------------------------------

func TestLog_DebugEnabled(t *testing.T) {
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	log(true, "INFO", "test message")

	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	os.Stderr = origStderr

	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("Expected log output with 'test message', got: %q", output)
	}
	if !strings.Contains(output, "INFO") {
		t.Errorf("Expected log output with 'INFO', got: %q", output)
	}
	if !strings.Contains(output, "posttool-worktree-tracker") {
		t.Errorf("Expected log output with program name, got: %q", output)
	}
}

func TestLog_DebugDisabled_InfoSuppressed(t *testing.T) {
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	log(false, "INFO", "should not appear")

	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	os.Stderr = origStderr

	if buf.String() != "" {
		t.Errorf("Expected no output when debug=false for INFO, got: %q", buf.String())
	}
}

func TestLog_DebugDisabled_ErrorAlwaysShown(t *testing.T) {
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	log(false, "ERROR", "error message")

	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	os.Stderr = origStderr

	output := buf.String()
	if !strings.Contains(output, "error message") {
		t.Errorf("Expected error in output even with debug=false, got: %q", output)
	}
	if !strings.Contains(output, "ERROR") {
		t.Errorf("Expected 'ERROR' level in output, got: %q", output)
	}
}

// ---------------------------------------------------------------------------
// WorktreeEvent struct tests
// ---------------------------------------------------------------------------

func TestWorktreeEvent_Fields(t *testing.T) {
	event := WorktreeEvent{
		Operation:    "add",
		WorktreePath: "/tmp/wt",
		Branch:       "feat",
		RepoPath:     "~/repo",
	}
	if event.Operation != "add" {
		t.Errorf("Expected operation 'add', got %q", event.Operation)
	}
	if event.WorktreePath != "/tmp/wt" {
		t.Errorf("Expected path '/tmp/wt', got %q", event.WorktreePath)
	}
	if event.Branch != "feat" {
		t.Errorf("Expected branch 'feat', got %q", event.Branch)
	}
	if event.RepoPath != "~/repo" {
		t.Errorf("Expected repo '~/repo', got %q", event.RepoPath)
	}
}
