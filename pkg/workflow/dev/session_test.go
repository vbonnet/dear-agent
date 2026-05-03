package dev

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const helloWorkflow = `schema_version: "1"
name: hello
version: 0.1.0
nodes:
  - id: greet
    kind: bash
    bash:
      cmd: 'echo hello'
`

func writeWorkflowFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "wf.yaml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return p
}

func TestNewSession_LoadsWorkflowAndDefaultsFixtures(t *testing.T) {
	p := writeWorkflowFile(t, helloWorkflow)
	s, err := NewSession(p, "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if s.Workflow().Name != "hello" {
		t.Errorf("Workflow().Name = %q", s.Workflow().Name)
	}
	if s.FixturesPath != FixtureFile(p) {
		t.Errorf("FixturesPath = %q, want %q", s.FixturesPath, FixtureFile(p))
	}
}

func TestSessionRun_BashNodeSucceedsWithoutFixtures(t *testing.T) {
	p := writeWorkflowFile(t, helloWorkflow)
	s, err := NewSession(p, "")
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	report, err := s.Run(context.Background(), false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !report.Succeeded {
		t.Errorf("expected Succeeded=true, report=%+v", report)
	}
	if len(s.History()) != 1 {
		t.Errorf("History len = %d, want 1", len(s.History()))
	}
}

func TestSessionRun_LiveFailsWhenNoLiveAI(t *testing.T) {
	p := writeWorkflowFile(t, helloWorkflow)
	s, _ := NewSession(p, "")
	if _, err := s.Run(context.Background(), true); err == nil {
		t.Fatal("expected error when --live without LiveAI")
	}
}

func TestSessionRetryNode_MissingNodeReturnsError(t *testing.T) {
	p := writeWorkflowFile(t, helloWorkflow)
	s, _ := NewSession(p, "")
	if _, err := s.RetryNode(context.Background(), "nope", false); err == nil {
		t.Fatal("expected error for missing node")
	}
}

func TestSessionRetryNode_ReExecutesSingleNode(t *testing.T) {
	p := writeWorkflowFile(t, helloWorkflow)
	s, _ := NewSession(p, "")
	if _, err := s.Run(context.Background(), false); err != nil {
		t.Fatalf("seed run: %v", err)
	}
	res, err := s.RetryNode(context.Background(), "greet", false)
	if err != nil {
		t.Fatalf("RetryNode: %v", err)
	}
	if res.Error != nil {
		t.Errorf("retry result error: %v", res.Error)
	}
	if !strings.Contains(res.Output, "hello") {
		t.Errorf("expected output to contain 'hello', got %q", res.Output)
	}
}

func TestSessionDiff_NeedsTwoRuns(t *testing.T) {
	p := writeWorkflowFile(t, helloWorkflow)
	s, _ := NewSession(p, "")
	if _, err := s.Run(context.Background(), false); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := s.DiffNode("greet"); err == nil {
		t.Fatal("expected error for diff with one run")
	}
}

func TestSessionDiff_ReportsNoChange(t *testing.T) {
	p := writeWorkflowFile(t, helloWorkflow)
	s, _ := NewSession(p, "")
	for i := 0; i < 2; i++ {
		if _, err := s.Run(context.Background(), false); err != nil {
			t.Fatalf("Run %d: %v", i, err)
		}
	}
	d, err := s.DiffNode("greet")
	if err != nil {
		t.Fatalf("DiffNode: %v", err)
	}
	if d != "" {
		t.Errorf("expected no diff, got %q", d)
	}
}

func TestFormatLineDiff_ShowsAdditionsAndDeletions(t *testing.T) {
	out := formatLineDiff("a\nb\nc", "a\nx\nc")
	if !strings.Contains(out, " a") || !strings.Contains(out, "+x") || !strings.Contains(out, "-b") {
		t.Errorf("unexpected diff output:\n%s", out)
	}
}

func TestREPL_ExitImmediately(t *testing.T) {
	p := writeWorkflowFile(t, helloWorkflow)
	s, _ := NewSession(p, "")
	in := strings.NewReader("exit\n")
	var out bytes.Buffer
	if err := REPL(context.Background(), s, in, &out); err != nil {
		t.Fatalf("REPL: %v", err)
	}
	if !strings.Contains(out.String(), "workflow-dev:") {
		t.Errorf("expected banner in output:\n%s", out.String())
	}
}

func TestREPL_RunVerb(t *testing.T) {
	p := writeWorkflowFile(t, helloWorkflow)
	s, _ := NewSession(p, "")
	in := strings.NewReader("r\nexit\n")
	var out bytes.Buffer
	if err := REPL(context.Background(), s, in, &out); err != nil {
		t.Fatalf("REPL: %v", err)
	}
	if !strings.Contains(out.String(), "run (mock)") {
		t.Errorf("missing run output:\n%s", out.String())
	}
}

func TestREPL_HelpVerb(t *testing.T) {
	p := writeWorkflowFile(t, helloWorkflow)
	s, _ := NewSession(p, "")
	in := strings.NewReader("help\nexit\n")
	var out bytes.Buffer
	if err := REPL(context.Background(), s, in, &out); err != nil {
		t.Fatalf("REPL: %v", err)
	}
	if !strings.Contains(out.String(), "verbs:") {
		t.Errorf("missing help output:\n%s", out.String())
	}
}

func TestREPL_NodesAndFixturesAndHistory(t *testing.T) {
	p := writeWorkflowFile(t, helloWorkflow)
	s, _ := NewSession(p, "")
	in := strings.NewReader("nodes\nfixtures\nhistory\nr\nhistory\nexit\n")
	var out bytes.Buffer
	if err := REPL(context.Background(), s, in, &out); err != nil {
		t.Fatalf("REPL: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "greet") {
		t.Errorf("nodes verb missing greet:\n%s", got)
	}
	if !strings.Contains(got, "(no fixtures)") {
		t.Errorf("fixtures verb missing 'no fixtures':\n%s", got)
	}
	if !strings.Contains(got, "(no runs yet)") {
		t.Errorf("first history call missing 'no runs yet':\n%s", got)
	}
}

func TestREPL_UnknownVerb(t *testing.T) {
	p := writeWorkflowFile(t, helloWorkflow)
	s, _ := NewSession(p, "")
	in := strings.NewReader("frob\nexit\n")
	var out bytes.Buffer
	if err := REPL(context.Background(), s, in, &out); err != nil {
		t.Fatalf("REPL: %v", err)
	}
	if !strings.Contains(out.String(), `unknown verb "frob"`) {
		t.Errorf("expected unknown verb message:\n%s", out.String())
	}
}

func TestREPL_DiffWithoutEnoughRuns(t *testing.T) {
	p := writeWorkflowFile(t, helloWorkflow)
	s, _ := NewSession(p, "")
	in := strings.NewReader("r\ndiff greet\nexit\n")
	var out bytes.Buffer
	if err := REPL(context.Background(), s, in, &out); err != nil {
		t.Fatalf("REPL: %v", err)
	}
	if !strings.Contains(out.String(), "need at least two runs") {
		t.Errorf("expected diff error:\n%s", out.String())
	}
}

func TestREPL_ApproveVerbExplains(t *testing.T) {
	p := writeWorkflowFile(t, helloWorkflow)
	s, _ := NewSession(p, "")
	in := strings.NewReader("approve abc-123\nexit\n")
	var out bytes.Buffer
	if err := REPL(context.Background(), s, in, &out); err != nil {
		t.Fatalf("REPL: %v", err)
	}
	if !strings.Contains(out.String(), "abc-123") {
		t.Errorf("expected approve placeholder mentioning the id:\n%s", out.String())
	}
}

func TestSessionReload_PicksUpFileChanges(t *testing.T) {
	p := writeWorkflowFile(t, helloWorkflow)
	s, _ := NewSession(p, "")

	updated := strings.Replace(helloWorkflow, "greet", "salute", 2)
	if err := os.WriteFile(p, []byte(updated), 0o600); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	n, _, err := s.Reload()
	if err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if n != 1 {
		t.Errorf("nodes after reload = %d, want 1", n)
	}
	if s.Workflow().Nodes[0].ID != "salute" {
		t.Errorf("expected reloaded node id 'salute', got %q", s.Workflow().Nodes[0].ID)
	}
}
