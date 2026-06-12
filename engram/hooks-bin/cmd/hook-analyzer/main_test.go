package main

import (
	"os"
	"testing"
)

func TestExpandPath_HomeTilde(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("UserHomeDir unavailable:", err)
	}
	got := expandPath("~/foo/bar")
	want := home + "/foo/bar"
	if got != want {
		t.Errorf("expandPath(\"~/foo/bar\") = %q, want %q", got, want)
	}
}

func TestExpandPath_AbsolutePassthrough(t *testing.T) {
	t.Parallel()
	p := "/usr/local/bin/foo"
	if got := expandPath(p); got != p {
		t.Errorf("expandPath(%q) = %q, want passthrough", p, got)
	}
}

func TestExpandPath_NoTilde(t *testing.T) {
	t.Parallel()
	p := "relative/path"
	if got := expandPath(p); got != p {
		t.Errorf("expandPath(%q) = %q, want passthrough", p, got)
	}
}
