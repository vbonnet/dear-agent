package main

import (
	"os"
	"strings"
	"testing"
)

func TestExpandPath_Tilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("UserHomeDir unavailable")
	}
	got := expandPath("~/foo/bar")
	want := home + "/foo/bar"
	if got != want {
		t.Errorf("expandPath(~/foo/bar) = %q, want %q", got, want)
	}
}

func TestExpandPath_NoTilde(t *testing.T) {
	got := expandPath("/absolute/path")
	if got != "/absolute/path" {
		t.Errorf("expandPath(/absolute/path) = %q, want unchanged", got)
	}
}

func TestExpandPath_Relative(t *testing.T) {
	got := expandPath("relative/path")
	if strings.HasPrefix(got, "~") {
		t.Errorf("expandPath(relative/path) = %q, should not start with ~", got)
	}
}
