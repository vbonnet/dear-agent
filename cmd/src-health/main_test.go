package main

import (
	"slices"
	"testing"
	"time"
)

func TestNewGitCommandIsBoundedAndNoninteractive(t *testing.T) {
	ctx, cmd, cancel := newGitCommand(t.TempDir(), "status", "--porcelain")
	defer cancel()

	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("git command context has no deadline")
	}
	remaining := time.Until(deadline)
	if remaining <= 0 || remaining > gitCommandTimeout {
		t.Fatalf("git command deadline remaining = %s, want within (0, %s]", remaining, gitCommandTimeout)
	}
	if !slices.Contains(cmd.Env, "GIT_TERMINAL_PROMPT=0") {
		t.Fatalf("git command environment does not disable terminal prompts: %q", cmd.Env)
	}
	if cmd.Path == "" || len(cmd.Args) < 4 || cmd.Args[1] != "-C" {
		t.Fatalf("unexpected git command: path=%q args=%q", cmd.Path, cmd.Args)
	}
}
