package main

import (
	"testing"
)

func TestCheckReturnsZero(t *testing.T) {
	// check() should always return 0 regardless of chezmoi state
	code := check()
	if code != 0 {
		t.Errorf("check() returned %d, want 0", code)
	}
}

func TestRunReturnsZero(t *testing.T) {
	// run() should always return 0 (never block session start)
	code := run()
	if code != 0 {
		t.Errorf("run() returned %d, want 0", code)
	}
}
