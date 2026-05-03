package monitoring

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/eventbus"
)

func TestIsSourceFile(t *testing.T) {
	v := &Validator{}
	tests := []struct {
		path string
		want bool
	}{
		{"main.go", true},
		{"app.js", true},
		{"index.ts", true},
		{"script.py", true},
		{"Main.java", true},
		{"lib.c", true},
		{"lib.cpp", true},
		{"header.h", true},
		{"app.rb", true},
		{"main.rs", true},
		{"README.md", false},
		{"config.yaml", false},
		{"image.png", false},
		{".git/objects/pack/abc", false},
		{"/repo/node_modules/lodash/index.js", false},
		{"__pycache__/module.pyc", false},
		{".vscode/settings.json", false},
		{".idea/workspace.xml", false},
		{"/.git/HEAD", false},
		{"/project/.vscode/launch.json", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := v.isSourceFile(tt.path)
			if got != tt.want {
				t.Errorf("isSourceFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestGenerateSummary_AllPassed(t *testing.T) {
	v := &Validator{}
	signals := []ValidationSignal{
		{Name: "git_commits", Passed: true},
		{Name: "file_count", Passed: true},
	}
	summary := v.generateSummary(signals, 1.0, true)
	if summary != "All validation signals passed. Implementation appears complete." {
		t.Errorf("unexpected summary for all-passed: %s", summary)
	}
}

func TestGenerateSummary_PassedWithWarnings(t *testing.T) {
	v := &Validator{}
	signals := []ValidationSignal{
		{Name: "git_commits", Passed: true},
		{Name: "file_count", Passed: false},
	}
	summary := v.generateSummary(signals, 0.7, true)
	if summary == "" {
		t.Fatal("expected non-empty summary")
	}
	if !contains(summary, "file_count") {
		t.Errorf("expected failed signal name in summary, got: %s", summary)
	}
	if !contains(summary, "passed overall") {
		t.Errorf("expected 'passed overall' in summary, got: %s", summary)
	}
}

func TestGenerateSummary_Failed(t *testing.T) {
	v := &Validator{}
	signals := []ValidationSignal{
		{Name: "git_commits", Passed: false, Message: "0 commits (expected >= 2)"},
		{Name: "file_count", Passed: false, Message: "0 files (expected >= 3)"},
	}
	summary := v.generateSummary(signals, 0.1, false)
	if !contains(summary, "failed") {
		t.Errorf("expected 'failed' in summary, got: %s", summary)
	}
	if !contains(summary, "0 commits") {
		t.Errorf("expected failed signal message in summary, got: %s", summary)
	}
}

func TestCountEventsByType_EmptyLog(t *testing.T) {
	v := &Validator{agentID: "agent-1", eventLog: ""}
	count := v.countEventsByType(EventGitCommit)
	if count != 0 {
		t.Errorf("expected 0 for empty event log path, got %d", count)
	}
}

func TestCountEventsByType_NonexistentFile(t *testing.T) {
	v := &Validator{agentID: "agent-1", eventLog: "/tmp/nonexistent-test-file-xyz.jsonl"}
	count := v.countEventsByType(EventGitCommit)
	if count != 0 {
		t.Errorf("expected 0 for nonexistent file, got %d", count)
	}
}

func TestCountEventsByType_WithEvents(t *testing.T) {
	tmpFile, err := os.CreateTemp(t.TempDir(), "event-log-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	events := []eventbus.Event{
		{Type: EventGitCommit, Data: map[string]interface{}{"agent_id": "agent-1"}},
		{Type: EventGitCommit, Data: map[string]interface{}{"agent_id": "agent-1"}},
		{Type: EventGitCommit, Data: map[string]interface{}{"agent_id": "agent-2"}},
		{Type: EventTestStarted, Data: map[string]interface{}{"agent_id": "agent-1"}},
	}
	for _, e := range events {
		line, _ := json.Marshal(e)
		tmpFile.Write(append(line, '\n'))
	}
	tmpFile.Close()

	v := &Validator{agentID: "agent-1", eventLog: tmpFile.Name()}

	commitCount := v.countEventsByType(EventGitCommit)
	if commitCount != 2 {
		t.Errorf("expected 2 git commits for agent-1, got %d", commitCount)
	}

	testCount := v.countEventsByType(EventTestStarted)
	if testCount != 1 {
		t.Errorf("expected 1 test started for agent-1, got %d", testCount)
	}

	fileCount := v.countEventsByType(EventFileCreated)
	if fileCount != 0 {
		t.Errorf("expected 0 file created events, got %d", fileCount)
	}
}

func TestCountFilesInDir(t *testing.T) {
	dir, err := os.MkdirTemp("", "file-count-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(dir, "app.js"), []byte("console.log('hi')"), 0644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# readme"), 0644)

	v := &Validator{workDir: dir}
	count := v.countFilesInDir(dir)
	if count != 2 {
		t.Errorf("expected 2 source files, got %d", count)
	}
}

func TestCountLinesInDir(t *testing.T) {
	dir, err := os.MkdirTemp("", "line-count-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	os.WriteFile(filepath.Join(dir, "main.go"), []byte("line1\nline2\nline3\n"), 0644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("should be ignored\n"), 0644)

	v := &Validator{workDir: dir}
	lines := v.countLinesInDir(dir)
	if lines != 3 {
		t.Errorf("expected 3 lines, got %d", lines)
	}
}

func TestValidateGitCommits_FromEventLog(t *testing.T) {
	tmpFile, err := os.CreateTemp(t.TempDir(), "event-log-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	events := []eventbus.Event{
		{Type: EventGitCommit, Data: map[string]interface{}{"agent_id": "a1"}},
		{Type: EventGitCommit, Data: map[string]interface{}{"agent_id": "a1"}},
		{Type: EventGitCommit, Data: map[string]interface{}{"agent_id": "a1"}},
	}
	for _, e := range events {
		line, _ := json.Marshal(e)
		tmpFile.Write(append(line, '\n'))
	}
	tmpFile.Close()

	v := &Validator{
		agentID:  "a1",
		workDir:  "/nonexistent",
		eventLog: tmpFile.Name(),
		config:   DefaultValidationConfig,
	}

	sig := v.ValidateGitCommits()
	if !sig.Passed {
		t.Errorf("expected git_commits to pass with 3 commits (min %d), got passed=%v",
			DefaultValidationConfig.MinCommitCount, sig.Passed)
	}
	if sig.Value != 3 {
		t.Errorf("expected value=3, got %v", sig.Value)
	}
}

func TestValidateFileCount(t *testing.T) {
	dir, err := os.MkdirTemp("", "file-count-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	for i := 0; i < 5; i++ {
		os.WriteFile(filepath.Join(dir, filepath.Base(
			filepath.Join(dir, "file"+string(rune('a'+i))+".go"),
		)), []byte("package main"), 0644)
	}

	v := &Validator{
		agentID:  "a1",
		workDir:  dir,
		eventLog: "",
		config:   DefaultValidationConfig,
	}

	sig := v.ValidateFileCount()
	if !sig.Passed {
		t.Errorf("expected file_count to pass with 5 files (min %d), passed=%v",
			DefaultValidationConfig.MinFileCount, sig.Passed)
	}
}

func TestValidateStubKeywords(t *testing.T) {
	dir, err := os.MkdirTemp("", "stub-kw-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// No stub keywords
	os.WriteFile(filepath.Join(dir, "clean.go"), []byte("package main\nfunc main() {}\n"), 0644)

	v := &Validator{workDir: dir, config: DefaultValidationConfig}
	sig := v.ValidateStubKeywords()
	if !sig.Passed {
		t.Errorf("expected stub_keywords to pass for clean code, got passed=%v value=%v", sig.Passed, sig.Value)
	}
}

func TestValidate_ScoringLogic(t *testing.T) {
	dir, err := os.MkdirTemp("", "validate-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// Create enough files and lines to pass those signals
	for i := 0; i < 5; i++ {
		content := "package main\nfunc f() {}\n"
		for j := 0; j < 20; j++ {
			content += "// padding\n"
		}
		os.WriteFile(filepath.Join(dir, "f"+string(rune('a'+i))+".go"), []byte(content), 0644)
	}

	// Create event log with commits and tests
	tmpFile, err := os.CreateTemp(t.TempDir(), "event-log-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	events := []eventbus.Event{
		{Type: EventGitCommit, Data: map[string]interface{}{"agent_id": "score-agent"}},
		{Type: EventGitCommit, Data: map[string]interface{}{"agent_id": "score-agent"}},
		{Type: EventTestStarted, Data: map[string]interface{}{"agent_id": "score-agent"}},
	}
	for _, e := range events {
		line, _ := json.Marshal(e)
		tmpFile.Write(append(line, '\n'))
	}
	tmpFile.Close()

	v := NewValidator("score-agent", dir, tmpFile.Name(), DefaultValidationConfig)
	result, err := v.Validate()
	if err != nil {
		t.Fatalf("Validate() error: %v", err)
	}

	if result.Score <= 0 {
		t.Errorf("expected positive score, got %f", result.Score)
	}
	if len(result.Signals) != 5 {
		t.Errorf("expected 5 signals, got %d", len(result.Signals))
	}
	if result.Summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestCanRunTests(t *testing.T) {
	// Go project
	dir, err := os.MkdirTemp("", "can-run-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	v := &Validator{workDir: dir}
	if v.canRunTests() {
		t.Error("expected canRunTests=false for empty dir")
	}

	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)
	if !v.canRunTests() {
		t.Error("expected canRunTests=true for Go project")
	}

	os.Remove(filepath.Join(dir, "go.mod"))
	os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0644)
	if !v.canRunTests() {
		t.Error("expected canRunTests=true for npm project")
	}

	os.Remove(filepath.Join(dir, "package.json"))
	os.WriteFile(filepath.Join(dir, "setup.py"), []byte(""), 0644)
	if !v.canRunTests() {
		t.Error("expected canRunTests=true for Python project")
	}
}
