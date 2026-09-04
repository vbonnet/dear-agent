package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/gclog"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

func TestSandboxGCReapRefusesBeforeInventoryOrCandidateMutation(t *testing.T) {
	restoreSandboxGCDepsForTest(t)
	sandboxGCReap = true
	// The transport refusal must precede even command-specific duration validation.
	sandboxGCMinAge = "not-a-duration"
	t.Setenv("AGM_DB_PATH", filepath.Join(t.TempDir(), "bypass.db"))
	t.Setenv("ENGRAM_TEST_MODE", "1")

	home := t.TempDir()
	t.Setenv("HOME", home)
	sentinel := filepath.Join(home, ".agm", "sandboxes", "candidate", "sentinel")
	if err := os.MkdirAll(filepath.Dir(sentinel), 0o700); err != nil {
		t.Fatalf("create candidate: %v", err)
	}
	if err := os.WriteFile(sentinel, []byte("preserve"), 0o600); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	var configCalls, openCalls, sweepCalls, logCalls int
	sandboxGCStoreConfigs = func() ([]*dolt.Config, error) {
		configCalls++
		return []*dolt.Config{{Workspace: "test", Database: "test"}}, nil
	}
	openSandboxGCStore = func(*dolt.Config) (sandboxGCSessionStore, error) {
		openCalls++
		return &fakeSandboxGCStore{}, nil
	}
	runSandboxGCSweep = func(*ops.OpContext, *ops.SandboxGCRequest) (*ops.SandboxGCResult, error) {
		sweepCalls++
		return &ops.SandboxGCResult{}, nil
	}
	logSandboxGCEntry = func(gclog.Entry) { logCalls++ }

	err := runSandboxGC(&cobra.Command{}, nil)
	if !errors.Is(err, errSandboxGCReapTransportUnavailable) {
		t.Fatalf("runSandboxGC() error = %v, want authenticated-transport refusal", err)
	}
	if configCalls != 0 || openCalls != 0 || sweepCalls != 0 || logCalls != 0 {
		t.Fatalf(
			"guard order calls = config:%d open:%d sweep:%d log:%d, want all zero",
			configCalls, openCalls, sweepCalls, logCalls,
		)
	}
	got, readErr := os.ReadFile(sentinel)
	if readErr != nil {
		t.Fatalf("sentinel was not preserved: %v", readErr)
	}
	if string(got) != "preserve" {
		t.Fatalf("sentinel = %q, want preserved content", got)
	}
}

func TestSandboxGCReapRefusesBeforeRootPreRun(t *testing.T) {
	tests := []struct {
		name   string
		config func(t *testing.T, root string) (path string, untouched string)
	}{
		{
			name: "malformed explicit config",
			config: func(t *testing.T, root string) (string, string) {
				t.Helper()
				path := filepath.Join(root, "malformed.yaml")
				if err := os.WriteFile(path, []byte("storage: ["), 0o600); err != nil {
					t.Fatal(err)
				}
				return path, ""
			},
		},
		{
			name: "centralized storage config",
			config: func(t *testing.T, root string) (string, string) {
				t.Helper()
				workspace := filepath.Join(root, "workspace")
				if err := os.MkdirAll(workspace, 0o700); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(root, "centralized.yaml")
				contents := "storage:\n  mode: centralized\n  workspace: \"" + workspace + "\"\n  relative_path: \".agm-work\"\n"
				if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
					t.Fatal(err)
				}
				return path, filepath.Join(workspace, ".agm-work")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restoreCommandTreeFlagsForTest(t, rootCmd)
			home := filepath.Join(t.TempDir(), "home")
			if err := os.MkdirAll(home, 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("HOME", home)
			configPath, centralizedTarget := tt.config(t, t.TempDir())

			oldCfg := cfg
			oldSilenceErrors := rootCmd.SilenceErrors
			oldSilenceUsage := rootCmd.SilenceUsage
			cfg = nil
			rootCmd.SilenceErrors = true
			rootCmd.SilenceUsage = true
			rootCmd.SetArgs([]string{"--config", configPath, "sandbox", "gc", "--reap", "--json"})
			t.Cleanup(func() {
				cfg = oldCfg
				rootCmd.SilenceErrors = oldSilenceErrors
				rootCmd.SilenceUsage = oldSilenceUsage
				rootCmd.SetArgs(nil)
			})

			err := rootCmd.Execute()
			if !errors.Is(err, errSandboxGCReapTransportUnavailable) {
				t.Fatalf("root command error = %v, want authenticated-transport refusal", err)
			}
			if cfg != nil {
				t.Fatal("root persistent pre-run loaded configuration before refusing reap")
			}
			if _, err := os.Lstat(filepath.Join(home, ".agm")); !os.IsNotExist(err) {
				t.Fatalf("root pre-run touched HOME/.agm before refusal: %v", err)
			}
			if centralizedTarget != "" {
				if _, err := os.Lstat(centralizedTarget); !os.IsNotExist(err) {
					t.Fatalf("root pre-run touched centralized storage before refusal: %v", err)
				}
			}
		})
	}
}

func TestSandboxGCDryRunRemainsAvailable(t *testing.T) {
	restoreSandboxGCDepsForTest(t)
	sandboxGCReap = false
	sandboxGCJSON = true
	sandboxGCStoreConfigs = func() ([]*dolt.Config, error) {
		return []*dolt.Config{{Workspace: "test", Database: "test"}}, nil
	}
	openSandboxGCStore = func(*dolt.Config) (sandboxGCSessionStore, error) {
		return &fakeSandboxGCStore{sessions: []*manifest.Manifest{{SessionID: "live"}}}, nil
	}

	wantContext := t.Context()
	var sweepCalls int
	runSandboxGCSweep = func(opCtx *ops.OpContext, req *ops.SandboxGCRequest) (*ops.SandboxGCResult, error) {
		sweepCalls++
		if opCtx.Context != wantContext {
			t.Fatalf("operation context = %v, want command context %v", opCtx.Context, wantContext)
		}
		if req.Reap {
			t.Fatal("dry run passed Reap=true to the sweep")
		}
		live, err := req.LiveSessionIDs()
		if err != nil {
			t.Fatalf("live-session inventory: %v", err)
		}
		if !live["live"] {
			t.Fatalf("live-session inventory = %v, want live", live)
		}
		return &ops.SandboxGCResult{Operation: "sandbox_gc", DryRun: true}, nil
	}

	var out bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetContext(wantContext)
	cmd.SetOut(&out)
	if err := runSandboxGC(cmd, nil); err != nil {
		t.Fatalf("runSandboxGC() dry run error = %v", err)
	}
	if sweepCalls != 1 {
		t.Fatalf("sweep calls = %d, want 1", sweepCalls)
	}
	if !strings.Contains(out.String(), `"dry_run": true`) {
		t.Fatalf("dry-run output = %q, want dry_run=true", out.String())
	}
}
