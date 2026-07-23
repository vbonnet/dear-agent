//go:build integration

package helpers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsolatedEnvironmentUsesSourceBinaryAndOwnedPaths(t *testing.T) {
	t.Setenv("DOLT_DATABASE", "host-database-must-not-leak")
	t.Setenv("GITHUB_TOKEN", "host-credential-must-not-leak")
	env := NewIsolatedEnvironment(t)

	for _, path := range []string{
		env.HomeDir, env.StateDir, env.SessionsDir, env.WorkDir, env.BinDir,
	} {
		if !strings.HasPrefix(path, env.Context.BaseDir+string(filepath.Separator)) {
			t.Errorf("isolated path %q is outside owned root %q", path, env.Context.BaseDir)
		}
	}
	if filepath.Dir(env.TmuxSocket) != "/tmp" {
		t.Fatalf("tmux socket %q is not on the short /tmp path", env.TmuxSocket)
	}
	if info, err := os.Stat(env.AGMBinary); err != nil || info.Mode()&0111 == 0 {
		t.Fatalf("source AGM binary is not executable: info=%v err=%v", info, err)
	}

	command := env.Command("--help")
	if command.Path != env.AGMBinary {
		t.Fatalf("command path = %q, want source binary %q", command.Path, env.AGMBinary)
	}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("run source AGM binary: %v\n%s", err, output)
	}
	joined := strings.Join(env.Environ(), "\n")
	for _, rejected := range []string{"host-database-must-not-leak", "host-credential-must-not-leak"} {
		if strings.Contains(joined, rejected) {
			t.Errorf("isolated subprocess environment leaked %q", rejected)
		}
	}

	owned := env.SessionName("owned")
	if err := env.RegisterSession(owned); err != nil {
		t.Fatalf("register owned session: %v", err)
	}
	if err := env.RegisterSession("unowned-session"); err == nil {
		t.Fatal("registered a session outside the environment prefix")
	}
	if err := env.WriteExecutable("../escape", "#!/bin/sh\n"); err == nil {
		t.Fatal("wrote an executable outside the owned bin directory")
	}
	if err := env.BuildGoExecutable("../escape", "package main\nfunc main() {}\n"); err == nil {
		t.Fatal("built an executable outside the owned bin directory")
	}
}

func TestIsolatedEnvironmentTmuxServersDoNotOverlap(t *testing.T) {
	first := NewIsolatedEnvironment(t)
	second := NewIsolatedEnvironment(t)
	firstName := first.SessionName("sentinel")
	secondName := second.SessionName("sentinel")
	if err := first.RegisterSession(firstName); err != nil {
		t.Fatal(err)
	}
	if err := second.RegisterSession(secondName); err != nil {
		t.Fatal(err)
	}

	if output, err := first.TmuxCommand("new-session", "-d", "-s", firstName, "sleep 30").CombinedOutput(); err != nil {
		t.Skipf("tmux cannot create an isolated server in this environment: %v: %s", err, output)
	}
	if output, err := second.TmuxCommand("new-session", "-d", "-s", secondName, "sleep 30").CombinedOutput(); err != nil {
		t.Fatalf("create second isolated tmux session: %v: %s", err, output)
	}
	if !first.HasSession(firstName) || !second.HasSession(secondName) {
		t.Fatal("an isolated tmux session exited before the cleanup assertion")
	}
	if first.HasSession(secondName) || second.HasSession(firstName) {
		t.Fatal("an isolated tmux server observed the other environment's session")
	}

	if err := first.Cleanup(); err != nil {
		t.Fatalf("cleanup first environment: %v", err)
	}
	if !second.HasSession(secondName) {
		t.Fatal("cleaning the first environment removed the second environment's session")
	}
}
