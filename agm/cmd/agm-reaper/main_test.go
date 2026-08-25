package main

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// TestBuildSentinel verifies the agm-reaper binary compiles.
func TestBuildSentinel(t *testing.T) {}

func TestParseReaperOutcome(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  manifest.SessionOutcome
	}{
		{name: "legacy empty", want: manifest.OutcomeUnknown},
		{name: "completed", value: "completed", want: manifest.OutcomeCompleted},
		{name: "crashed", value: "crashed", want: manifest.OutcomeCrashed},
		{name: "killed", value: "killed", want: manifest.OutcomeKilled},
		{name: "gc stale", value: "gc-stale", want: manifest.OutcomeGCStale},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseReaperOutcome(tc.value)
			if err != nil {
				t.Fatalf("parseReaperOutcome(%q) error = %v", tc.value, err)
			}
			if got != tc.want {
				t.Errorf("parseReaperOutcome(%q) = %q, want %q", tc.value, got, tc.want)
			}
		})
	}

	if _, err := parseReaperOutcome("unknown"); err == nil || !strings.Contains(err.Error(), "invalid --outcome") {
		t.Fatalf("parseReaperOutcome(unknown) error = %v, want invalid --outcome", err)
	}
}

func TestRunRejectsUnknownOutcomeBeforeStartupAcknowledgement(t *testing.T) {
	startupReader, startupWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = startupReader.Close() }()
	defer func() { _ = startupWriter.Close() }()

	originalCommandLine := flag.CommandLine
	originalArgs := os.Args
	t.Cleanup(func() {
		flag.CommandLine = originalCommandLine
		os.Args = originalArgs
	})
	commandLine := flag.NewFlagSet("agm-reaper", flag.ContinueOnError)
	commandLine.SetOutput(io.Discard)
	flag.CommandLine = commandLine
	logFile := filepath.Join(t.TempDir(), "reaper.log")
	os.Args = []string{
		"agm-reaper",
		"--session-id", "stable-id",
		"--session", "resolved-tmux",
		"--outcome", "unknown",
		"--log-file", logFile,
		"--startup-fd", strconv.Itoa(int(startupReader.Fd())),
	}

	err = run()
	if err == nil || !strings.Contains(err.Error(), "invalid --outcome") {
		t.Fatalf("run() error = %v, want outcome validation before acknowledgement write", err)
	}
	if _, statErr := os.Stat(logFile); !os.IsNotExist(statErr) {
		t.Fatalf("log file status after rejected outcome = %v, want file to remain absent", statErr)
	}
}

func TestValidateResolvedTargets(t *testing.T) {
	tests := []struct {
		name        string
		sessionID   string
		tmuxSession string
		wantErr     string
	}{
		{name: "resolved", sessionID: "stable-id", tmuxSession: "resolved-tmux"},
		{name: "missing tmux", sessionID: "stable-id", wantErr: "--session flag is required"},
		{name: "missing stable id", tmuxSession: "resolved-tmux", wantErr: "--session-id flag is required"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateResolvedTargets(tc.sessionID, tc.tmuxSession)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validateResolvedTargets() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("validateResolvedTargets() error = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestValidateRevision(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		actual   string
		wantErr  string
	}{
		{name: "same revision", expected: "0123456789ab", actual: "0123456789ab"},
		{name: "full and short revision", expected: "0123456789abcdef", actual: "0123456789ab"},
		{name: "same dirty revision", expected: "0123456789abcdef-dirty", actual: "0123456789ab-dirty"},
		{name: "clean and dirty mismatch", expected: "0123456789ab", actual: "0123456789ab-dirty", wantErr: "does not match"},
		{name: "mismatch", expected: "0123456789ab", actual: "fedcba987654", wantErr: "does not match"},
		{name: "missing expected", expected: "unknown", actual: "0123456789ab", wantErr: "expected AGM revision is unavailable"},
		{name: "dirty missing expected", expected: "unknown-dirty", actual: "0123456789ab-dirty", wantErr: "expected AGM revision is unavailable"},
		{name: "missing actual", expected: "0123456789ab", actual: "unknown", wantErr: "no embedded VCS revision"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRevision(tc.expected, tc.actual)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validateRevision() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("validateRevision() error = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestAcknowledgeStartupWritesReadyAndClosesDescriptor(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()

	if err := acknowledgeStartup(int(writer.Fd())); err != nil {
		t.Fatalf("acknowledgeStartup() error = %v", err)
	}
	_ = writer.Close()
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "ready\n" {
		t.Fatalf("acknowledgeStartup() wrote %q, want ready record", got)
	}
}

func TestAcknowledgeStartupAllowsDisabledChannel(t *testing.T) {
	if err := acknowledgeStartup(-1); err != nil {
		t.Fatalf("acknowledgeStartup(-1) error = %v", err)
	}
}
