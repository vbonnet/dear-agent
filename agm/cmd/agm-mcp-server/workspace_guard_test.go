package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestResolveWorkspace is the env matrix for workspace resolution: WORKSPACE env
// wins, else the config (mcp-server.yaml) workspace, else a loud error. The
// error case is the regression guard for the silent 'personal' fallback that
// booted a handless MCP surface (ce-vj8a).
func TestResolveWorkspace(t *testing.T) {
	tests := []struct {
		name         string
		envWorkspace string
		configWS     string
		want         string
		wantErr      bool
	}{
		{"env set", "oss", "", "oss", false},
		{"env wins over config", "personal", "oss", "personal", false},
		{"config used when env empty", "", "oss", "oss", false},
		{"both empty -> loud error", "", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(k string) string {
				if k == "WORKSPACE" {
					return tt.envWorkspace
				}
				return ""
			}
			got, err := resolveWorkspace(tt.configWS, getenv)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error when no workspace is set, got nil")
				}
				if !strings.Contains(err.Error(), "workspace") {
					t.Errorf("error should name the missing workspace + how to set it, got: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestBoot_NoWorkspace_FailsLoud builds the server and boots it with WORKSPACE
// unset and a config carrying no workspace. It must exit non-zero with an
// actionable error — never boot the silent partial tool surface (ce-vj8a).
func TestBoot_NoWorkspace_FailsLoud(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping boot test in short mode")
	}
	bin := filepath.Join(t.TempDir(), "agm-mcp-server")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}

	// A temp HOME with an ENABLED config that has NO workspace.
	home := t.TempDir()
	cfgDir := filepath.Join(home, ".config", "agm")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "mcp-server.yaml"), []byte("mcp_server:\n  enabled: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin)
	cmd.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH")} // deliberately no WORKSPACE
	out, err := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		t.Fatal("server hung instead of failing loud on a missing workspace (ce-vj8a)")
	}
	if err == nil {
		t.Fatalf("server booted with no workspace — it must fail loud, not boot a partial surface (ce-vj8a)\n%s", out)
	}
	s := string(out)
	if !strings.Contains(s, "FATAL") || !strings.Contains(s, "workspace") {
		t.Errorf("expected an actionable FATAL workspace error, got:\n%s", s)
	}
}
