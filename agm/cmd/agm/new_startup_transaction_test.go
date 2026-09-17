package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/config"
	"github.com/vbonnet/dear-agent/agm/internal/launchparity"
	"github.com/vbonnet/dear-agent/agm/internal/tmux"
	"github.com/vbonnet/dear-agent/internal/sandbox"
)

func TestFinalizeCLICreateSessionStopsCancellationAfterLiveness(t *testing.T) {
	t.Setenv("AGM_TEST_RUN_ID", "")
	t.Setenv("AGM_TEST_ENV", "")
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	completed := false

	err := finalizeCLICreateSession(ctx, "agy-create", cliCreateFinalizationRuntime{
		checkLiveness: func(got context.Context, _, _ string) (tmux.PaneLiveness, error) {
			if got != ctx {
				t.Fatal("liveness scan did not receive the caller context")
			}
			cancel()
			return tmux.PaneLiveness{SessionExists: true, HarnessAlive: true}, nil
		},
		updateTitle: func(string) { completed = true },
		attach:      func(string) { completed = true },
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("finalizeCLICreateSession() error = %v, want context.Canceled", err)
	}
	if completed {
		t.Fatal("creation completed or attached after liveness scan canceled its caller")
	}
}

func TestValidateFinalStartupLiveness(t *testing.T) {
	tests := []struct {
		name    string
		verdict tmux.PaneLiveness
		err     error
		wantErr string
	}{
		{name: "live", verdict: tmux.PaneLiveness{SessionExists: true, HarnessAlive: true}},
		{name: "probe failure", err: errors.New("ps failed"), wantErr: "liveness check failed"},
		{name: "session gone", verdict: tmux.PaneLiveness{}, wantErr: "session disappeared"},
		{name: "harness dead", verdict: tmux.PaneLiveness{SessionExists: true, Evidence: "descendants: zsh"}, wantErr: "no active harness process"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := launchparity.ValidateFinalLiveness(tt.verdict, tt.err)
			if tt.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

type instanceBoundCleanupProvider struct {
	cancel         context.CancelFunc
	workspace      string
	destroyedIDs   []string
	destroyContext error
}

func (p *instanceBoundCleanupProvider) Create(_ context.Context, req sandbox.SandboxRequest) (*sandbox.Sandbox, error) {
	p.workspace = req.WorkspaceDir
	merged := filepath.Join(req.WorkspaceDir, "merged")
	upper := filepath.Join(req.WorkspaceDir, "upper")
	work := filepath.Join(req.WorkspaceDir, "work")
	for _, dir := range []string{merged, upper, work} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
	}
	if p.cancel != nil {
		p.cancel()
	}
	return &sandbox.Sandbox{
		ID:         req.SessionID,
		MergedPath: merged,
		WorkingDir: merged,
		UpperPath:  upper,
		WorkPath:   work,
		CreatedAt:  time.Now(),
	}, nil
}

func (p *instanceBoundCleanupProvider) Destroy(ctx context.Context, id string) error {
	p.destroyContext = ctx.Err()
	p.destroyedIDs = append(p.destroyedIDs, id)
	return os.RemoveAll(p.workspace)
}

func (*instanceBoundCleanupProvider) Validate(context.Context, string) error { return nil }

// Match the APFS provider's display label, which intentionally differs from
// its factory selector ("apfs"). Rollback must never use this as a lookup key.
func (*instanceBoundCleanupProvider) Name() string { return "apfs-reflink" }

func TestCreateLifecycleRollsBackWithOriginalProviderInstanceAfterProvisioning(t *testing.T) {
	originalCfg := cfg
	originalEnableSandbox, originalNoSandbox := enableSandbox, noSandbox
	originalProvider := sandboxProvider
	originalHarness, originalModel := harnessName, modelName
	originalPrompt, originalPromptFile := prompt, promptFile
	originalRole, originalPermissionProfile := roleName, permissionProfile
	originalPermissionsAllow := permissionsAllow
	originalInheritPermissions := inheritPermissions
	originalResolvedPolicy := resolvedSessionPermissionPolicy
	t.Cleanup(func() {
		cfg = originalCfg
		enableSandbox, noSandbox = originalEnableSandbox, originalNoSandbox
		sandboxProvider = originalProvider
		harnessName, modelName = originalHarness, originalModel
		prompt, promptFile = originalPrompt, originalPromptFile
		roleName, permissionProfile = originalRole, originalPermissionProfile
		permissionsAllow = originalPermissionsAllow
		inheritPermissions = originalInheritPermissions
		resolvedSessionPermissionPolicy = originalResolvedPolicy
	})

	homeDir := mkdirPhysical(t, t.TempDir())
	loaded, _ := loadSandboxRuntimeAuthority(t, homeDir)
	repoRoot := mkdirPhysical(t, t.TempDir())
	loaded.Sandbox = config.SandboxConfig{
		Enabled:    true,
		Repos:      []string{repoRoot},
		Onboarding: config.OnboardingConfig{Enabled: false},
	}
	cfg = loaded

	wrongHome := mkdirPhysical(t, t.TempDir())
	_, wrongAuthority := loadSandboxRuntimeAuthority(t, wrongHome)
	wrongRoot, err := wrongAuthority.Sandboxes()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", homeDir)

	ctx, cancel := context.WithCancel(t.Context())
	provider := &instanceBoundCleanupProvider{cancel: cancel}
	factoryCalls := 0
	const providerSelector = "instance-bound-cleanup-test"
	sandbox.RegisterProvider(providerSelector, func() sandbox.Provider {
		factoryCalls++
		if factoryCalls == 1 {
			return provider
		}
		return &instanceBoundCleanupProvider{}
	})

	enableSandbox = false
	noSandbox = false
	sandboxProvider = providerSelector
	harnessName = "claude-code"
	modelName = "sonnet"
	prompt = ""
	promptFile = ""
	roleName = ""
	permissionProfile = ""
	permissionsAllow = nil
	inheritPermissions = false
	resolvedSessionPermissionPolicy = nil
	t.Setenv("AGM_DB_PATH", filepath.Join(t.TempDir(), "sessions.db"))
	t.Setenv("AGM_SESSIONS_DIR", t.TempDir())
	t.Setenv(trustedAddDirsEnv, "")
	t.Setenv(trustedAddDirsSessionEnv, "")
	t.Setenv(trustedGuardPathEnv, "")

	const sessionID = "instance-bound-cleanup-session"
	err = runCreateSessionLifecycle(ctx, "instance-bound-cleanup", sessionID, repoRoot, false, &circuitBreakerAdmission{sandboxRoot: wrongRoot})
	if err == nil || !strings.Contains(err.Error(), "sandbox.create-authority") {
		t.Fatalf("runCreateSessionLifecycle() error = %v, want post-provision authority failure", err)
	}
	if factoryCalls != 1 {
		t.Fatalf("provider factory calls = %d, want one retained provider instance", factoryCalls)
	}
	if len(provider.destroyedIDs) != 1 || provider.destroyedIDs[0] != sessionID {
		t.Fatalf("original provider Destroy IDs = %v, want stable AGM session ID %q", provider.destroyedIDs, sessionID)
	}
	if provider.destroyContext != nil {
		t.Fatalf("Destroy context error = %v, want cancellation detached for rollback", provider.destroyContext)
	}
	if _, statErr := os.Stat(provider.workspace); !os.IsNotExist(statErr) {
		t.Fatalf("provisioned workspace still exists after rollback: %v", statErr)
	}
}
