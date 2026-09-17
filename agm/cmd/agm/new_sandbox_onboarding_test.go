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
	"github.com/vbonnet/dear-agent/internal/sandbox"
)

type onboardingTrackingProvider struct {
	name           string
	sandboxID      string
	mergedPath     string
	workingDir     string
	destroyErr     error
	destroyIDs     []string
	destroyCtxErr  []error
	destroyBounded []bool
}

func (p *onboardingTrackingProvider) Create(_ context.Context, _ sandbox.SandboxRequest) (*sandbox.Sandbox, error) {
	return &sandbox.Sandbox{
		ID:         p.sandboxID,
		MergedPath: p.mergedPath,
		WorkingDir: p.workingDir,
		CreatedAt:  time.Now(),
	}, nil
}

func (p *onboardingTrackingProvider) Destroy(ctx context.Context, id string) error {
	p.destroyIDs = append(p.destroyIDs, id)
	p.destroyCtxErr = append(p.destroyCtxErr, ctx.Err())
	_, bounded := ctx.Deadline()
	p.destroyBounded = append(p.destroyBounded, bounded)
	return p.destroyErr
}

func (*onboardingTrackingProvider) Validate(context.Context, string) error { return nil }
func (p *onboardingTrackingProvider) Name() string                         { return p.name }

func TestProvisionSandboxRollsBackUnsafeOnboardingWithoutExternalMutation(t *testing.T) {
	originalCfg := cfg
	t.Cleanup(func() { cfg = originalCfg })
	home := mkdirPhysical(t, filepath.Join(t.TempDir(), "retained-home"))
	loaded, authority := loadSandboxRuntimeAuthority(t, home)
	repoRoot := mkdirPhysical(t, filepath.Join(t.TempDir(), "repo"))
	outside := mkdirPhysical(t, filepath.Join(t.TempDir(), "outside"))
	sentinel := filepath.Join(outside, "sentinel")
	if err := os.WriteFile(sentinel, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(home, ".claude")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	driftHome := mkdirPhysical(t, filepath.Join(t.TempDir(), "ambient-home"))
	driftSentinel := filepath.Join(driftHome, "sentinel")
	if err := os.WriteFile(driftSentinel, []byte("ambient-preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", driftHome)

	provider := &onboardingTrackingProvider{
		name:       "unsafe-onboarding-rollback-test",
		sandboxID:  "provider-issued-id",
		mergedPath: "/provider/merged",
		workingDir: "/provider/merged/project",
	}
	sandbox.RegisterProvider(provider.name, func() sandbox.Provider { return provider })
	loaded.Sandbox = config.SandboxConfig{
		Enabled:    true,
		Repos:      []string{repoRoot},
		Onboarding: config.OnboardingConfig{Enabled: true},
	}
	cfg = loaded
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	var gotErr error
	var gotNonNil bool
	stdout := captureStdout(t, func() {
		got, err := provisionSandbox(canceledCtx, authority, provider.name, "requested-session", repoRoot)
		gotNonNil = got != nil
		gotErr = err
	})
	err := gotErr
	if err == nil || !strings.Contains(err.Error(), "install sandbox onboarding") {
		t.Fatalf("provisionSandbox() error = %v, want onboarding failure", err)
	}
	if gotNonNil {
		t.Fatal("provisionSandbox() returned a usable sandbox after onboarding failure")
	}
	if strings.Contains(stdout, "Sandbox provisioned") {
		t.Fatalf("failed provisioning printed success output: %q", stdout)
	}
	if len(provider.destroyIDs) != 1 || provider.destroyIDs[0] != provider.sandboxID {
		t.Fatalf("Destroy calls = %v, want exactly [%q]", provider.destroyIDs, provider.sandboxID)
	}
	if len(provider.destroyCtxErr) != 1 || provider.destroyCtxErr[0] != nil {
		t.Fatalf("Destroy context errors = %v, want one cancellation-independent context", provider.destroyCtxErr)
	}
	if len(provider.destroyBounded) != 1 || !provider.destroyBounded[0] {
		t.Fatalf("Destroy bounded contexts = %v, want one deadline-bounded context", provider.destroyBounded)
	}
	for path, want := range map[string]string{sentinel: "preserve", driftSentinel: "ambient-preserve"} {
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if string(content) != want {
			t.Fatalf("sentinel %q = %q, want %q", path, content, want)
		}
	}
	entries, readErr := os.ReadDir(outside)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 1 || entries[0].Name() != "sentinel" {
		t.Fatalf("outside directory mutated: %v", entries)
	}
}

func TestProvisionSandboxJoinsOnboardingAndProviderCleanupErrors(t *testing.T) {
	originalCfg := cfg
	t.Cleanup(func() { cfg = originalCfg })
	home := mkdirPhysical(t, filepath.Join(t.TempDir(), "home"))
	loaded, authority := loadSandboxRuntimeAuthority(t, home)
	repoRoot := mkdirPhysical(t, filepath.Join(t.TempDir(), "repo"))
	templatePath := filepath.Join(t.TempDir(), "invalid.tmpl")
	if err := os.WriteFile(templatePath, []byte("{{end}}"), 0o600); err != nil {
		t.Fatal(err)
	}
	cleanupErr := errors.New("provider cleanup failed")
	provider := &onboardingTrackingProvider{
		name:       "onboarding-cleanup-error-test",
		sandboxID:  "cleanup-error-sandbox",
		mergedPath: "/provider/merged",
		workingDir: "/provider/merged/project",
		destroyErr: cleanupErr,
	}
	sandbox.RegisterProvider(provider.name, func() sandbox.Provider { return provider })
	loaded.Sandbox = config.SandboxConfig{
		Enabled: true,
		Repos:   []string{repoRoot},
		Onboarding: config.OnboardingConfig{
			Enabled:      true,
			TemplatePath: templatePath,
		},
	}
	cfg = loaded

	got, err := provisionSandbox(context.Background(), authority, provider.name, "requested-session", repoRoot)
	if err == nil || !strings.Contains(err.Error(), "parse onboarding template") || !errors.Is(err, cleanupErr) {
		t.Fatalf("provisionSandbox() error = %v, want render and cleanup failures", err)
	}
	if got != nil {
		t.Fatalf("provisionSandbox() = %#v, want no usable sandbox", got)
	}
	if len(provider.destroyIDs) != 1 || provider.destroyIDs[0] != provider.sandboxID {
		t.Fatalf("Destroy calls = %v, want exactly [%q]", provider.destroyIDs, provider.sandboxID)
	}
}

func TestProvisionSandboxKeepsDisabledOnboardingBehavior(t *testing.T) {
	originalCfg := cfg
	t.Cleanup(func() { cfg = originalCfg })
	home := mkdirPhysical(t, filepath.Join(t.TempDir(), "home"))
	loaded, authority := loadSandboxRuntimeAuthority(t, home)
	repoRoot := mkdirPhysical(t, filepath.Join(t.TempDir(), "repo"))
	outside := mkdirPhysical(t, filepath.Join(t.TempDir(), "outside"))
	if err := os.Symlink(outside, filepath.Join(home, ".claude")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	provider := &onboardingTrackingProvider{
		name:       "disabled-onboarding-test",
		sandboxID:  "disabled-onboarding-sandbox",
		mergedPath: "/provider/merged",
		workingDir: "/provider/merged/project",
	}
	sandbox.RegisterProvider(provider.name, func() sandbox.Provider { return provider })
	loaded.Sandbox = config.SandboxConfig{
		Enabled:    true,
		Repos:      []string{repoRoot},
		Onboarding: config.OnboardingConfig{Enabled: false},
	}
	cfg = loaded

	got, err := provisionSandbox(context.Background(), authority, provider.name, "requested-session", repoRoot)
	if err != nil {
		t.Fatalf("provisionSandbox() error = %v", err)
	}
	if got == nil || got.ID != provider.sandboxID {
		t.Fatalf("provisionSandbox() = %#v, want provider sandbox", got)
	}
	if len(provider.destroyIDs) != 0 {
		t.Fatalf("Destroy calls = %v, want none", provider.destroyIDs)
	}
	entries, readErr := os.ReadDir(outside)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("disabled onboarding mutated output target: %v", entries)
	}
}
