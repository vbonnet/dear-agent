package main

import (
	"os"
	"testing"
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
