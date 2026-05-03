package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeRoles(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "roles.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCmdValidateOK(t *testing.T) {
	path := writeRoles(t, `
roles:
  research:
    primary:
      model: m
`)
	if got := cmdValidate([]string{path}); got != 0 {
		t.Errorf("exit = %d, want 0", got)
	}
}

func TestCmdValidateBadFile(t *testing.T) {
	path := writeRoles(t, `roles:
  bad:
    description: no tiers`)
	if got := cmdValidate([]string{path}); got != 1 {
		t.Errorf("exit = %d, want 1", got)
	}
}

func TestCmdValidateMissingArg(t *testing.T) {
	if got := cmdValidate(nil); got != 2 {
		t.Errorf("exit = %d, want 2", got)
	}
}

func TestCmdListUsesFile(t *testing.T) {
	path := writeRoles(t, `
roles:
  research:
    primary:
      model: m
`)
	if got := cmdList([]string{"--file", path}); got != 0 {
		t.Errorf("exit = %d", got)
	}
}

func TestCmdDescribeKnownRole(t *testing.T) {
	path := writeRoles(t, `
roles:
  research:
    description: "long-context"
    primary:
      model: m
`)
	if got := cmdDescribe([]string{"--file", path, "research"}); got != 0 {
		t.Errorf("exit = %d", got)
	}
}

func TestCmdDescribeUnknownRole(t *testing.T) {
	path := writeRoles(t, `
roles:
  research:
    primary:
      model: m
`)
	if got := cmdDescribe([]string{"--file", path, "ghost"}); got != 1 {
		t.Errorf("exit = %d", got)
	}
}

func TestCmdDescribeMissingArg(t *testing.T) {
	if got := cmdDescribe(nil); got != 2 {
		t.Errorf("exit = %d", got)
	}
}
