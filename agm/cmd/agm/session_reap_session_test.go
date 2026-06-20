package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/orphan"
)

func TestSessionReapSessionCmd_Metadata(t *testing.T) {
	if sessionReapSessionCmd.Use != "reap-session" {
		t.Errorf("Use = %q, want reap-session", sessionReapSessionCmd.Use)
	}
	// Must be registered under `agm session`.
	var found bool
	for _, c := range sessionCmd.Commands() {
		if c.Use == "reap-session" {
			found = true
			break
		}
	}
	if !found {
		t.Error("reap-session is not registered under sessionCmd")
	}
	for _, f := range []string{"dry-run", "targets", "root-names", "root-pid"} {
		if sessionReapSessionCmd.Flags().Lookup(f) == nil {
			t.Errorf("missing flag --%s", f)
		}
	}
}

func TestPrintSessionReapSummary_NoRoot(t *testing.T) {
	var buf bytes.Buffer
	printSessionReapSummary(&buf, orphan.SessionResult{
		Targets:   orphan.DefaultTargets,
		RootNames: orphan.DefaultRootNames,
		StartPID:  4242,
		RootFound: false,
	})
	out := buf.String()
	if !strings.Contains(out, "no session root found") || !strings.Contains(out, "nothing reaped") {
		t.Errorf("no-root summary = %q", out)
	}
}

func TestPrintSessionReapSummary_Killed(t *testing.T) {
	var buf bytes.Buffer
	printSessionReapSummary(&buf, orphan.SessionResult{
		Targets:   orphan.DefaultTargets,
		RootPID:   10,
		RootFound: true,
		Found:     []orphan.Proc{{PID: 11, Command: "gopls"}, {PID: 12, Command: "agm-mcp-server"}},
		Killed:    []orphan.Proc{{PID: 11, Command: "gopls"}, {PID: 12, Command: "agm-mcp-server"}},
	})
	out := buf.String()
	if !strings.Contains(out, "root pid=10") || !strings.Contains(out, "2 reaped") {
		t.Errorf("killed summary = %q", out)
	}
	if !strings.Contains(out, "reaped  pid=11 command=gopls") {
		t.Errorf("missing per-pid reaped line: %q", out)
	}
}

func TestPrintSessionReapSummary_DryRun(t *testing.T) {
	var buf bytes.Buffer
	printSessionReapSummary(&buf, orphan.SessionResult{
		Targets:   orphan.DefaultTargets,
		RootPID:   10,
		RootFound: true,
		DryRun:    true,
		Found:     []orphan.Proc{{PID: 11, Command: "gopls"}},
	})
	out := buf.String()
	if !strings.Contains(out, "would reap") || !strings.Contains(out, "would reap pid=11") {
		t.Errorf("dry-run summary = %q", out)
	}
}

func TestPrintSessionReapSummary_Failed(t *testing.T) {
	var buf bytes.Buffer
	printSessionReapSummary(&buf, orphan.SessionResult{
		Targets:   orphan.DefaultTargets,
		RootPID:   10,
		RootFound: true,
		Found:     []orphan.Proc{{PID: 12, Command: "agm-mcp-server"}},
		Failed:    []orphan.Proc{{PID: 12, Command: "agm-mcp-server"}},
		KillError: map[int]string{12: "permission denied"},
	})
	out := buf.String()
	if !strings.Contains(out, "1 failed") || !strings.Contains(out, "FAILED  pid=12") || !strings.Contains(out, "permission denied") {
		t.Errorf("failed summary = %q", out)
	}
}
