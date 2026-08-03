package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/audit"
)

func TestRun_NoArgs_ReturnsError(t *testing.T) {
	code := run(nil, os.Stdout, os.Stderr)
	if code == 0 {
		t.Error("run with no args should return non-zero exit code")
	}
}

func TestRun_Help_ReturnsZero(t *testing.T) {
	for _, flag := range []string{"-h", "--help", "help"} {
		code := run([]string{flag}, os.Stdout, os.Stderr)
		if code != 0 {
			t.Errorf("run(%q) = %d, want 0", flag, code)
		}
	}
}

func TestRun_UnknownSubcommand_ReturnsError(t *testing.T) {
	code := run([]string{"notasubcommand"}, os.Stdout, os.Stderr)
	if code == 0 {
		t.Error("run with unknown subcommand should return non-zero exit code")
	}
}

func TestRunAudit_DryRunFlagIsNotExposed(t *testing.T) {
	code := run([]string{"run", "--dry-run"}, os.Stdout, os.Stderr)
	if code != 2 {
		t.Errorf("run with removed --dry-run flag = %d, want usage error 2", code)
	}
}

func TestRunShow_RendersEveryRemediationField(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "audit.db")
	store, err := audit.OpenSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteStore: %v", err)
	}
	findings := []struct {
		fingerprint string
		suggested   audit.Remediation
		want        []string
	}{
		{
			fingerprint: "auto",
			suggested:   audit.Remediation{Strategy: audit.StrategyAuto, Command: "go test ./..."},
			want:        []string{"strategy=auto", `command="go test ./..."`},
		},
		{
			fingerprint: "pr",
			suggested: audit.Remediation{
				Strategy: audit.StrategyPR,
				Patch:    "diff --git a/a b/a",
				Title:    "Fix audit finding",
				Body:     "Implementation details",
			},
			want: []string{"strategy=pr", `suggested_patch: "diff --git a/a b/a"`, `suggested_title: "Fix audit finding"`, `suggested_body:  "Implementation details"`},
		},
		{
			fingerprint: "issue",
			suggested: audit.Remediation{
				Strategy: audit.StrategyIssue,
				Title:    "Investigate audit finding",
				Body:     "Triage details",
			},
			want: []string{"strategy=issue", `suggested_title: "Investigate audit finding"`, `suggested_body:  "Triage details"`},
		},
	}

	storedIDs := make([]string, len(findings))
	for i, tc := range findings {
		stored, upsertErr := store.UpsertFinding(t.Context(), audit.Finding{
			Repo:        "demo",
			CheckID:     "demo",
			Fingerprint: tc.fingerprint,
			Severity:    audit.SeverityP1,
			Title:       "requires attention",
			Suggested:   tc.suggested,
		})
		if upsertErr != nil {
			t.Fatalf("UpsertFinding(%s): %v", tc.fingerprint, upsertErr)
		}
		storedIDs[i] = stored.FindingID
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close fixture store: %v", err)
	}

	for i, tc := range findings {
		t.Run(tc.fingerprint, func(t *testing.T) {
			stdout, err := os.CreateTemp(t.TempDir(), "stdout")
			if err != nil {
				t.Fatalf("CreateTemp stdout: %v", err)
			}
			stderr, err := os.CreateTemp(t.TempDir(), "stderr")
			if err != nil {
				t.Fatalf("CreateTemp stderr: %v", err)
			}
			code := runShow([]string{"--db", dbPath, storedIDs[i]}, stdout, stderr)
			if err := stdout.Close(); err != nil {
				t.Fatalf("close stdout: %v", err)
			}
			if err := stderr.Close(); err != nil {
				t.Fatalf("close stderr: %v", err)
			}
			if code != 0 {
				stderrBytes, _ := os.ReadFile(stderr.Name())
				t.Fatalf("runShow code = %d, stderr = %s", code, stderrBytes)
			}
			stdoutBytes, err := os.ReadFile(stdout.Name())
			if err != nil {
				t.Fatalf("ReadFile stdout: %v", err)
			}
			got := string(stdoutBytes)
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q:\n%s", want, got)
				}
			}
		})
	}
}
