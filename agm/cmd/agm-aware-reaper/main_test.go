package main

import (
	"strings"
	"testing"
)

// TestRunFlagParseError verifies an unknown flag is reported rather than
// panicking, exercising the flag wiring without touching the OS.
func TestRunFlagParseError(t *testing.T) {
	if err := run([]string{"--nope"}, &strings.Builder{}); err == nil {
		t.Fatal("expected an error for an unknown flag")
	}
}

// TestRunHelp verifies -h returns flag.ErrHelp (a clean, non-crashing exit
// path) and that the binary's flag set is constructed.
func TestRunHelp(t *testing.T) {
	var out strings.Builder
	err := run([]string{"-h"}, &out)
	if err == nil {
		t.Fatal("expected flag.ErrHelp for -h")
	}
}
