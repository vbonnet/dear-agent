package hippocampus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCriticCheck_SQLInjection(t *testing.T) {
	code := `query := fmt.Sprintf("SELECT * FROM users WHERE id = %s", userInput)`
	decision := CriticCheck(code)

	if decision.Approved {
		t.Error("expected critic to reject SQL injection")
	}

	found := false
	for _, issue := range decision.Issues {
		if issue.Category == "sql_injection" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected sql_injection issue category")
	}
}

func TestCriticCheck_PathTraversal(t *testing.T) {
	code := `path := filepath.Join(baseDir, "../../etc/passwd")`
	decision := CriticCheck(code)

	if decision.Approved {
		t.Error("expected critic to reject path traversal")
	}

	found := false
	for _, issue := range decision.Issues {
		if issue.Category == "path_traversal" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected path_traversal issue category")
	}
}

func TestCriticCheck_ResourceLeak(t *testing.T) {
	code := `f, err := os.Open(path)
if err != nil {
    return err
}
// no defer close
data := readAll(f)`

	decision := CriticCheck(code)

	if decision.Approved {
		t.Error("expected critic to flag resource leak")
	}

	found := false
	for _, issue := range decision.Issues {
		if issue.Category == "resource_leak" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected resource_leak issue category")
	}
}

func TestCriticCheck_ResourceWithDefer(t *testing.T) {
	code := `f, err := os.Open(path)
if err != nil {
    return err
}
defer f.Close()
data := readAll(f)`

	decision := CriticCheck(code)

	// Should not flag resource leak when defer Close is present
	for _, issue := range decision.Issues {
		if issue.Category == "resource_leak" {
			t.Error("should not flag resource leak when defer Close() is present")
		}
	}
}

func TestCriticCheck_CleanCode(t *testing.T) {
	code := `result := db.Query(ctx, "SELECT * FROM users WHERE id = $1", userID)
return result, nil`

	decision := CriticCheck(code)

	if !decision.Approved {
		t.Errorf("expected clean code to be approved, got issues: %v", decision.Issues)
	}
}

func TestToolRestrictions_Worker(t *testing.T) {
	w := WorkerRole()

	tools := []string{"Bash", "Write", "Read", "WebFetch", "Glob"}
	for _, tool := range tools {
		if !w.IsAllowed(tool) {
			t.Errorf("worker should allow %s", tool)
		}
	}
}

func TestToolRestrictions_Critic(t *testing.T) {
	c := CriticRole()

	// Critic can read
	readTools := []string{"Read", "Glob", "Grep"}
	for _, tool := range readTools {
		if !c.IsAllowed(tool) {
			t.Errorf("critic should allow %s", tool)
		}
	}

	// Critic cannot execute or write
	blockedTools := []string{"Bash", "Write", "Edit", "WebFetch", "WebSearch"}
	for _, tool := range blockedTools {
		if c.IsAllowed(tool) {
			t.Errorf("critic should NOT allow %s", tool)
		}
	}
}

func TestDyad_LogDecision(t *testing.T) {
	dir := t.TempDir()
	dyad := NewDyad(dir)

	decision := CriticDecision{
		WorkerTask: "test task",
		Approved:   false,
		Issues: []CriticIssue{
			{Category: "sql_injection", Severity: "error", Detail: "test detail"},
		},
		Reasoning: "test reasoning",
	}

	if err := dyad.LogDecision(decision); err != nil {
		t.Fatalf("LogDecision: %v", err)
	}

	// Verify log file exists and contains valid JSONL
	logPath := filepath.Join(dir, "decisions.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(lines))
	}

	var logged CriticDecision
	if err := json.Unmarshal([]byte(lines[0]), &logged); err != nil {
		t.Fatalf("unmarshal log line: %v", err)
	}

	if logged.WorkerTask != "test task" {
		t.Errorf("WorkerTask = %q, want %q", logged.WorkerTask, "test task")
	}
	if logged.Approved {
		t.Error("expected Approved=false")
	}
}

func TestDyad_LogDecision_Append(t *testing.T) {
	dir := t.TempDir()
	dyad := NewDyad(dir)

	// Log two decisions
	d1 := CriticDecision{WorkerTask: "task1", Approved: true, Reasoning: "ok"}
	d2 := CriticDecision{WorkerTask: "task2", Approved: false, Reasoning: "bad"}

	if err := dyad.LogDecision(d1); err != nil {
		t.Fatal(err)
	}
	if err := dyad.LogDecision(d2); err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(dir, "decisions.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 log lines, got %d", len(lines))
	}
}

func TestNewDyad_DefaultLogDir(t *testing.T) {
	dyad := NewDyad("")
	if dyad.LogDir == "" {
		t.Error("expected non-empty default LogDir")
	}
	if !strings.Contains(dyad.LogDir, ".engram") {
		t.Errorf("expected LogDir to contain .engram, got %s", dyad.LogDir)
	}
}
