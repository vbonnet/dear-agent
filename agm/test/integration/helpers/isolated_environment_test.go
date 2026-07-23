//go:build integration

package helpers

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
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
	socketRoot := filepath.Dir(env.TmuxSocket)
	if socketRoot != filepath.Dir(env.Context.BaseDir) || filepath.Dir(socketRoot) != "/tmp" {
		t.Fatalf("tmux socket %q does not share the short per-user root for %q", env.TmuxSocket, env.Context.BaseDir)
	}
	if len(env.TmuxSocket) >= 100 {
		t.Fatalf("tmux socket %q exceeds the conservative Unix path budget", env.TmuxSocket)
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

	if err := first.StartTmuxServer(firstName); err != nil {
		if IsUnavailablePrerequisite(err) && first.TmuxUnavailable() {
			t.Skipf("tmux cannot create an isolated server in this environment: %v", err)
		}
		t.Fatalf("create first isolated tmux session: %v", err)
	}
	if err := second.StartTmuxServer(secondName); err != nil {
		t.Fatalf("create second isolated tmux session: %v", err)
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

func TestIsUnavailablePrerequisite(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "missing executable", err: fmt.Errorf("wrapped: %w", exec.ErrNotFound), want: true},
		{name: "missing explicit path", err: &os.PathError{Op: "fork/exec", Path: "/bin/ps", Err: os.ErrNotExist}, want: true},
		{name: "filesystem permission", err: fmt.Errorf("wrapped: %w", os.ErrPermission), want: true},
		{name: "sandbox denial", err: errors.New("start server: operation not permitted"), want: true},
		{name: "invalid arguments", err: errors.New("start server: invalid tmux option"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsUnavailablePrerequisite(tt.err); got != tt.want {
				t.Fatalf("IsUnavailablePrerequisite(%q) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestUnavailableTmuxPrerequisiteCleansWithoutFailure(t *testing.T) {
	env := NewIsolatedEnvironment(t)
	t.Setenv("PATH", t.TempDir())

	err := env.StartTmuxServer(env.SessionName("unavailable"))
	if err == nil {
		t.Fatal("StartTmuxServer succeeded without tmux on PATH")
	}
	if !IsUnavailablePrerequisite(err) || !env.TmuxUnavailable() {
		t.Fatalf("missing tmux was not classified unavailable: %v", err)
	}
	if err := env.Cleanup(); err != nil {
		t.Fatalf("cleanup unavailable tmux prerequisite: %v", err)
	}
	if _, err := os.Stat(env.Context.BaseDir); !os.IsNotExist(err) {
		t.Fatalf("isolated root survived unavailable-prerequisite cleanup: %v", err)
	}
}
