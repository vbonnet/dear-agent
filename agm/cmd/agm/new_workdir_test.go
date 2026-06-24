package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/cli"
)

func TestGetWorkDir_ExplicitDirectoryBeatsPWD(t *testing.T) {
	oldDirectory := directory
	oldProjectDir := cli.GetProjectDirectory()
	t.Cleanup(func() {
		directory = oldDirectory
		cli.SetProjectDirectory(oldProjectDir)
	})

	tmp := t.TempDir()
	pwd := filepath.Join(tmp, "pwd")
	explicit := filepath.Join(tmp, "explicit")
	if err := os.MkdirAll(pwd, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(explicit, 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PWD", pwd)

	directory = explicit
	cli.SetProjectDirectory(explicit)

	got, err := getWorkDir()
	if err != nil {
		t.Fatalf("getWorkDir returned error: %v", err)
	}
	if got != explicit {
		t.Fatalf("getWorkDir() = %q, want explicit -C directory %q", got, explicit)
	}
}

func TestCurrentWorkingDirectory_ExplicitDirectoryBeatsCWD(t *testing.T) {
	oldDirectory := directory
	oldProjectDir := cli.GetProjectDirectory()
	t.Cleanup(func() {
		directory = oldDirectory
		cli.SetProjectDirectory(oldProjectDir)
	})

	tmp := t.TempDir()
	cwd := filepath.Join(tmp, "cwd")
	explicit := filepath.Join(tmp, "explicit")
	if err := os.MkdirAll(cwd, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(explicit, 0755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(cwd)

	directory = explicit
	cli.SetProjectDirectory(explicit)

	if got := currentWorkingDirectory(); got != explicit {
		t.Fatalf("currentWorkingDirectory() = %q, want explicit -C directory %q", got, explicit)
	}
}
