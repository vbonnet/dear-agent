package context

import (
	"testing"
)

func TestDefaultContextPath(t *testing.T) {
	t.Setenv("HOME", "/fakehome")
	path := DefaultContextPath()
	expected := "/fakehome/.claude/session-context/current.json"
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}

func TestDefaultContextPath_NoHome(t *testing.T) {
	t.Setenv("HOME", "")
	path := DefaultContextPath()
	expected := "/tmp/.claude/session-context/current.json"
	if path != expected {
		t.Errorf("expected %q, got %q", expected, path)
	}
}
