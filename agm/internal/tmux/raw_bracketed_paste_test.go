package tmux

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseBracketedPasteFlag(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		enabled bool
		wantErr bool
	}{
		{name: "enabled", output: "1\n", enabled: true},
		{name: "disabled", output: "0\n"},
		{name: "empty is disabled", output: ""},
		{name: "malformed", output: "yes\n", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			enabled, err := parseBracketedPasteFlag([]byte(test.output))
			if (err != nil) != test.wantErr || enabled != test.enabled {
				t.Fatalf("parseBracketedPasteFlag(%q) = (%v, %v), want (%v, error=%v)",
					test.output, enabled, err, test.enabled, test.wantErr)
			}
		})
	}
}

func TestBracketedPasteModeObservationFailureIsDefiniteNotSent(t *testing.T) {
	err := requireBracketedPasteMode(t.Context(), filepath.Join(t.TempDir(), "missing.sock"), "%7")
	if err == nil {
		t.Fatal("missing bracketed-paste observation returned nil")
	}
	if PromptSubmissionMayHaveOccurred(err) {
		t.Fatalf("pre-paste observation failure was marked submission-uncertain: %v", err)
	}
}

func TestRawMultilineDeliveryRequiresLiveBracketedPasteMode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping isolated tmux raw-paste integration in short mode")
	}
	skipIfNoTmux(t)
	socketPath, cleanup := setupTestSocket(t)
	defer cleanup()

	helperPath := filepath.Join(t.TempDir(), "raw-paste-reader.sh")
	helper := `#!/bin/sh
set -eu
mode=$1
output=$2
stty raw -echo
if [ "$mode" = "on" ]; then
  printf '\033[?2004h'
fi
: > "$output"
exec cat >> "$output"
`
	if err := os.WriteFile(helperPath, []byte(helper), 0o700); err != nil {
		t.Fatalf("write raw-paste helper: %v", err)
	}

	for _, test := range []struct {
		name      string
		mode      string
		wantErr   bool
		wantBytes []byte
	}{
		{
			name:    "mode off refuses before pane mutation",
			mode:    "off",
			wantErr: true,
		},
		{
			name:      "mode on delivers one bracketed raw paste and Enter",
			mode:      "on",
			wantBytes: []byte("\x1b[200~A\nB\x1b[201~\r"),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			sessionName := "raw-mode-" + test.mode
			outputPath := filepath.Join(t.TempDir(), "stdin.bin")
			command := strings.Join([]string{
				shellQuoteForRawPasteTest(helperPath),
				test.mode,
				shellQuoteForRawPasteTest(outputPath),
			}, " ")
			if output, err := exec.Command("tmux", "-S", socketPath, "new-session", "-d", "-s", sessionName, command).CombinedOutput(); err != nil {
				t.Fatalf("create raw-paste session: %v: %s", err, output)
			}
			t.Cleanup(func() {
				_ = exec.Command("tmux", "-S", socketPath, "kill-session", "-t", sessionName).Run()
			})

			waitForRawPasteReader(t, socketPath, sessionName, outputPath, test.mode == "on")
			paneID := rawPastePaneID(t, socketPath, sessionName)

			err := deliverRawMultilineForTest(t.Context(), paneID, "A\nB")
			if (err != nil) != test.wantErr {
				t.Fatalf("raw multiline delivery error = %v, want error=%v", err, test.wantErr)
			}
			if err != nil {
				if PromptSubmissionMayHaveOccurred(err) {
					t.Fatalf("pre-paste bracket-mode rejection was marked submission-uncertain: %v", err)
				}
				if !strings.Contains(err.Error(), "does not have bracketed-paste mode enabled") {
					t.Fatalf("raw multiline rejection = %v, want bracketed-paste requirement", err)
				}
			}

			got := waitForRawPasteBytes(t, outputPath, len(test.wantBytes))
			if !bytes.Equal(got, test.wantBytes) {
				t.Fatalf("raw multiline pane bytes = % x, want % x", got, test.wantBytes)
			}
		})
	}
}

func deliverRawMultilineForTest(ctx context.Context, paneID, command string) error {
	if err := acquireTmuxSemaphore(ctx); err != nil {
		return err
	}
	defer releaseTmuxSemaphore()
	return withTmuxLockContext(ctx, func() error {
		return sendCommandToTargetForHarnessLockedWithOptions(ctx, paneID, command, "codex-cli", false, true)
	})
}

func waitForRawPasteReader(t *testing.T, socketPath, sessionName, outputPath string, wantBracketed bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		_, fileErr := os.Stat(outputPath)
		flag, flagErr := exec.Command("tmux", "-S", socketPath, "display-message", "-p", "-t", sessionName, "#{bracket_paste_flag}").Output()
		if fileErr == nil && flagErr == nil && strings.TrimSpace(string(flag)) == fmt.Sprint(boolIntForRawPasteTest(wantBracketed)) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("raw-paste reader did not reach bracketed=%v", wantBracketed)
}

func rawPastePaneID(t *testing.T, socketPath, sessionName string) string {
	t.Helper()
	output, err := exec.Command("tmux", "-S", socketPath, "display-message", "-p", "-t", sessionName, "#{pane_id}").Output()
	if err != nil {
		t.Fatalf("resolve raw-paste pane: %v", err)
	}
	return strings.TrimSpace(string(output))
}

func waitForRawPasteBytes(t *testing.T, outputPath string, wantLength int) []byte {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(outputPath)
		if err == nil && len(data) >= wantLength {
			return data
		}
		time.Sleep(20 * time.Millisecond)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read raw-paste bytes: %v", err)
	}
	return data
}

func shellQuoteForRawPasteTest(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func boolIntForRawPasteTest(value bool) int {
	if value {
		return 1
	}
	return 0
}
