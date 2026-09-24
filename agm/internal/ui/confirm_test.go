package ui

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"context"
)

func TestCleanupConfirmationDescription_ListsEachSelectedTarget(t *testing.T) {
	archive := []string{`"shared-name" [ID: "stopped-id"]`}
	remove := []string{
		`"shared-name" [ID: "archive-a"]`,
		`"shared-name" [ID: "archive-b"]`,
	}
	want := "Archive 1 session:\n" +
		"  - \"shared-name\" [ID: \"stopped-id\"]\n" +
		"Delete 2 sessions:\n" +
		"  - \"shared-name\" [ID: \"archive-a\"]\n" +
		"  - \"shared-name\" [ID: \"archive-b\"]\n" +
		"\nThis action cannot be undone for deleted sessions."
	if got := cleanupConfirmationDescription(archive, remove); got != want {
		t.Errorf("confirmation description = %q, want %q", got, want)
	}
}

func TestConfirmCleanup_LongListRemainsVisibleAndNoRejects(t *testing.T) {
	var out bytes.Buffer
	targets := make([]string, 40)
	for i := range targets {
		targets[i] = fmt.Sprintf("selected-%02d [ID: %02d]", i, i)
	}
	confirmed, err := confirmCleanupWithIO(nil, targets, DefaultConfig(), &out, strings.NewReader("n\n"))
	if err != nil || confirmed {
		t.Fatalf("confirmation = %v, err = %v, want rejected", confirmed, err)
	}
	for _, fragment := range []string{targets[0], targets[len(targets)-1], "cannot be undone"} {
		if !strings.Contains(out.String(), fragment) {
			t.Errorf("confirmation output missing %q", fragment)
		}
	}
}

func TestConfirmCleanup_ListsTargetsBeforeAccepting(t *testing.T) {
	var out bytes.Buffer
	archive := []string{`"shared-name" [ID: "stopped-id"]`}
	remove := []string{`"shared-name" [ID: "archive-b"]`}
	confirmed, err := confirmCleanupWithIO(archive, remove, DefaultConfig(), &out, strings.NewReader("y\n"))
	if err != nil || !confirmed {
		t.Fatalf("confirmation = %v, err = %v, want accepted", confirmed, err)
	}
	for _, fragment := range []string{archive[0], remove[0], "cannot be undone", "Confirm cleanup"} {
		if !strings.Contains(out.String(), fragment) {
			t.Errorf("confirmation output missing %q: %q", fragment, out.String())
		}
	}
}

func TestConfirmCleanup_OutputFailureCannotConfirm(t *testing.T) {
	confirmed, err := confirmCleanupWithIO([]string{"target"}, nil, DefaultConfig(), failingCleanupWriter{}, strings.NewReader("y\n"))
	if err == nil || confirmed {
		t.Fatalf("confirmation = %v, err = %v, want display failure", confirmed, err)
	}
}

func TestConfirmCleanup_RemainsVisibleWhenStdoutRedirected(t *testing.T) {
	if os.Getenv("AGM_TEST_CONFIRM_STDERR_HELPER") == "1" {
		confirmed, err := ConfirmCleanup(nil, []string{`"shared-name" [ID: "archive-b"]`}, DefaultConfig())
		if err != nil || confirmed {
			t.Fatalf("confirmation = %v, err = %v, want rejected", confirmed, err)
		}
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestConfirmCleanup_RemainsVisibleWhenStdoutRedirected$")
	cmd.Env = []string{
		"AGM_TEST_CONFIRM_STDERR_HELPER=1",
		"HOME=" + t.TempDir(),
		"TERM=dumb",
	}
	cmd.Stdin = strings.NewReader("n\n")
	cmd.Stdout = io.Discard
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("redirected confirmation failed: %v; stderr = %q", err, stderr.String())
	}
	for _, fragment := range []string{`[ID: "archive-b"]`, "cannot be undone", "Confirm cleanup"} {
		if !strings.Contains(stderr.String(), fragment) {
			t.Errorf("redirected confirmation missing %q: %q", fragment, stderr.String())
		}
	}
}

type failingCleanupWriter struct{}

func (failingCleanupWriter) Write([]byte) (int, error) { return 0, errors.New("output failed") }

// Ctrl-C at the confirmation prompt must stop the command, not hang it.
//
// huh's RunAccessible takes no context and blocks on stdin, so SIGINT
// cancelled the command context while the process sat waiting for input that
// was never coming. The cleanup loops downstream never reached their own
// cancellation checks.
func TestConfirmCleanupContextGivesUpOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// A reader that never yields, which is what a terminal awaiting input is.
	blocked, _ := io.Pipe()
	defer blocked.Close()

	done := make(chan struct{})
	var confirmed bool
	var err error
	go func() {
		confirmed, err = confirmCleanupWithIOContext(ctx,
			[]string{"session-a"}, nil, &Config{}, io.Discard, blocked)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("confirmation did not observe cancellation; Ctrl-C would hang the command")
	}
	if confirmed {
		t.Error("a canceled confirmation must not read as confirmed")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
