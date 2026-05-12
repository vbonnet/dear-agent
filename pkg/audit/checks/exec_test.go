package checks

import (
	"context"
	"strings"
	"testing"
)

func TestCappedBuffer_TruncatesAtCap(t *testing.T) {
	b := &cappedBuffer{cap: 10}
	n, err := b.Write([]byte("hello"))
	if err != nil || n != 5 {
		t.Fatalf("first write: n=%d err=%v, want 5/nil", n, err)
	}
	// Second write fills the buffer; remaining capacity is 5.
	n, err = b.Write([]byte("world!!"))
	if err != nil {
		t.Fatalf("second write err: %v", err)
	}
	// Write reports the *input* length, not the bytes-actually-stored, so
	// callers (io.Copy etc.) don't perceive truncation as an error.
	if n != 5 {
		t.Errorf("n = %d, want 5 (bytes that fit)", n)
	}
	if got := b.String(); got != "helloworld" {
		t.Errorf("buffer = %q, want %q", got, "helloworld")
	}
}

func TestCappedBuffer_WriteAfterFullIsNoop(t *testing.T) {
	b := &cappedBuffer{cap: 4}
	if _, err := b.Write([]byte("abcd")); err != nil {
		t.Fatalf("initial write: %v", err)
	}
	// Buffer is full; further writes report their length but store nothing.
	n, err := b.Write([]byte("more"))
	if err != nil {
		t.Fatalf("write to full: %v", err)
	}
	if n != 4 {
		t.Errorf("n = %d, want 4 (input length reported even when dropped)", n)
	}
	if got := b.String(); got != "abcd" {
		t.Errorf("buffer = %q, want %q", got, "abcd")
	}
}

func TestFirstNonEmptyLine(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"only whitespace", "   \n\n   \n", ""},
		{"single line", "hello\n", "hello"},
		{"leading blanks", "\n\n  \nfound me\nrest\n", "found me"},
		{"trimmed", "  spaced  \n", "spaced"},
		{"no newline", "no newline here", "no newline here"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstNonEmptyLine(tt.in); got != tt.want {
				t.Errorf("firstNonEmptyLine(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestRunCommand_CapturesStdoutAndExit(t *testing.T) {
	res := runCommand(context.Background(), "", "sh", "-c", "echo hello && echo err 1>&2 && exit 0")
	if res.Err != nil {
		t.Fatalf("Err = %v, want nil", res.Err)
	}
	if !strings.Contains(res.Stdout, "hello") {
		t.Errorf("Stdout = %q, want to contain %q", res.Stdout, "hello")
	}
	if !strings.Contains(res.Stderr, "err") {
		t.Errorf("Stderr = %q, want to contain %q", res.Stderr, "err")
	}
	if res.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", res.ExitCode)
	}
}

func TestRunCommand_NonZeroExitIsNotErrErr(t *testing.T) {
	// Per the contract in runCommand's docstring, a non-zero exit is NOT
	// surfaced as Err — checks rely on exit codes as a signal.
	res := runCommand(context.Background(), "", "sh", "-c", "exit 3")
	if res.Err != nil {
		t.Errorf("Err = %v, want nil (non-zero exit is not an Err)", res.Err)
	}
	if res.ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3", res.ExitCode)
	}
}

func TestRunCommand_MissingBinaryIsErr(t *testing.T) {
	res := runCommand(context.Background(), "", "this-binary-does-not-exist-xyzzy")
	if res.Err == nil {
		t.Error("Err = nil, want non-nil for missing binary")
	}
}

func TestRunCommand_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := runCommand(ctx, "", "sh", "-c", "sleep 1")
	if res.Err == nil && res.ExitCode == 0 {
		t.Error("expected cancellation to surface as Err or non-zero exit")
	}
}
