// Package gittest provides hermetic Git helpers for the isolated module's
// tests. It keeps host Git configuration and hooks out of temporary fixtures.
package gittest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func Run(t testing.TB, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{
		"-c", "core.hooksPath=" + os.DevNull,
		"-c", "init.defaultBranch=main",
		"-c", "commit.gpgsign=false",
		"-c", "user.name=dear-agent test",
		"-c", "user.email=test@dear-agent.invalid",
	}, args...)...)
	command.Dir = dir
	command.Env = environment()
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("gittest: git %s failed in %s: %v\n%s", strings.Join(args, " "), dir, err, output)
	}
	return string(output)
}

func NewRepo(t testing.TB) string {
	t.Helper()
	repository := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repository, 0o700); err != nil {
		t.Fatalf("gittest: create repository: %v", err)
	}
	Run(t, repository, "init", "-q")
	HardenRepo(t, repository)
	if err := os.WriteFile(filepath.Join(repository, "README.md"), []byte("# test\n"), 0o600); err != nil {
		t.Fatalf("gittest: write README: %v", err)
	}
	Run(t, repository, "add", "README.md")
	Run(t, repository, "commit", "-qm", "initial")
	return repository
}

func HardenRepo(t testing.TB, repository string) {
	t.Helper()
	Run(t, repository, "config", "core.hooksPath", os.DevNull)
	Run(t, repository, "config", "user.name", "dear-agent test")
	Run(t, repository, "config", "user.email", "test@dear-agent.invalid")
}

func environment() []string {
	environment := make([]string, 0, len(os.Environ())+5)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "GIT_") || strings.HasPrefix(entry, "HOME=") || strings.HasPrefix(entry, "XDG_CONFIG_HOME=") {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment,
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
	)
}
