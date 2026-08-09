package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateUnstickReason(t *testing.T) {
	tests := []struct {
		name    string
		reason  string
		wantErr bool
	}{
		{name: "empty", reason: "", wantErr: true},
		{name: "too short", reason: "fix", wantErr: true},
		{name: "whitespace only", reason: "      ", wantErr: true},
		{name: "exactly min", reason: "fixit", wantErr: false},
		{name: "long enough", reason: "ghost-text block, supervisor recovery", wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateUnstickReason(tt.reason)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for reason %q, got nil", tt.reason)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error for reason %q: %v", tt.reason, err)
			}
		})
	}
}

func TestSessionUnstickRequiresSessionName(t *testing.T) {
	// cobra ExactArgs(1) should reject a missing session name.
	if got := sessionUnstickCmd.Args; got == nil {
		t.Fatal("sessionUnstickCmd should declare an Args validator")
	}
	if err := sessionUnstickCmd.Args(sessionUnstickCmd, []string{}); err == nil {
		t.Fatal("expected error when no session name is provided")
	}
	if err := sessionUnstickCmd.Args(sessionUnstickCmd, []string{"a", "b"}); err == nil {
		t.Fatal("expected error when too many args are provided")
	}
	if err := sessionUnstickCmd.Args(sessionUnstickCmd, []string{"worker-1"}); err != nil {
		t.Fatalf("unexpected error for single arg: %v", err)
	}
}

func TestAppendUnstickAuditWritesRecord(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dear-agent", "unstick-audit.jsonl")

	rec := unstickAuditRecord{
		Timestamp: "2026-06-20T00:00:00Z",
		Session:   "worker-42",
		Action:    "stash-human",
		Reason:    "ghost-text block, supervisor recovery",
		Caller:    "supervisor-1",
	}
	if err := appendUnstickAuditTo(path, rec); err != nil {
		t.Fatalf("appendUnstickAuditTo failed: %v", err)
	}

	// A second record should append, not overwrite.
	rec2 := rec
	rec2.Action = "submit-agm"
	if err := appendUnstickAuditTo(path, rec2); err != nil {
		t.Fatalf("second appendUnstickAuditTo failed: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open audit log: %v", err)
	}
	defer func() { _ = f.Close() }()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if len(lines) != 2 {
		t.Fatalf("expected 2 audit lines, got %d", len(lines))
	}

	var got unstickAuditRecord
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("failed to unmarshal first audit line: %v", err)
	}
	if got != rec {
		t.Fatalf("audit record mismatch:\n got  %+v\n want %+v", got, rec)
	}
}

func TestUnstickAuditPathHonorsXDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/tmp/agm-test-state")
	got := unstickAuditPath()
	want := filepath.Join("/tmp/agm-test-state", "dear-agent", "unstick-audit.jsonl")
	if got != want {
		t.Fatalf("unstickAuditPath() = %q, want %q", got, want)
	}
}

func TestUnstickAuditPathDefault(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")
	got := unstickAuditPath()
	if !strings.HasSuffix(got, filepath.Join(".local", "state", "dear-agent", "unstick-audit.jsonl")) {
		t.Fatalf("unstickAuditPath() = %q, want suffix .local/state/dear-agent/unstick-audit.jsonl", got)
	}
}

func TestUnstickCaller(t *testing.T) {
	t.Setenv("AGM_SESSION_NAME", "supervisor-7")
	if got := unstickCaller(); got != "supervisor-7" {
		t.Fatalf("unstickCaller() = %q, want supervisor-7", got)
	}

	t.Setenv("AGM_SESSION_NAME", "")
	t.Setenv("USER", "operator")
	if got := unstickCaller(); got != "operator" {
		t.Fatalf("unstickCaller() = %q, want operator", got)
	}
}
