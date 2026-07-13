package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/debug"
	"github.com/vbonnet/dear-agent/agm/internal/git"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/modelrouter"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
	"github.com/vbonnet/dear-agent/internal/telemetry"
)

// createAndRegisterManifest writes the manifest directory, builds the v2
// manifest, and registers it in the configured lifecycle store.
func createAndRegisterManifest(sessionID, sessionName, workDir string, sandboxInfo *manifest.SandboxConfig) error {
	debug.Phase("Create Manifest")
	sessionsDir := getSessionsDir()
	manifestDir := filepath.Join(sessionsDir, sessionName)
	manifestPath := filepath.Join(manifestDir, "manifest.yaml")

	if err := os.MkdirAll(manifestDir, 0700); err != nil {
		ui.PrintWarning(fmt.Sprintf("Failed to create manifest directory: %v", err))
		ui.PrintWarning("Proceeding without manifest - you can run 'agm sync' later")
		return nil
	}

	debug.Log("Using SessionID: %s", sessionID)
	m := buildSessionManifest(sessionID, sessionName, workDir, sandboxInfo, spawnedCodexMetadata)
	if testMode {
		m.IsTest = true
		debug.Log("Marking session as test (is_test=true)")
	}

	debug.Phase("Register in Dolt Database")
	if err := registerSessionInDolt(m); err != nil {
		return err
	}
	if err := git.CommitManifest(manifestPath, "create", sessionName); err != nil {
		debug.Log("manifest commit skipped: %v", err)
	}

	ctx := context.Background()

	// Telemetry: emit routing decision span when a tier was chosen.
	if m.ModelTier != "" {
		d := &modelrouter.Decision{
			Tier:         modelrouter.Tier(m.ModelTier),
			Model:        m.Model,
			Reason:       "recorded at manifest creation",
			ExplicitTier: modelTierFlag != "",
		}
		modelrouter.RecordRoutingDecision(ctx, m.Harness, d)
	}

	// Telemetry: agm.session.start span + active-task metric.
	telemetry.SessionStarted(ctx, m.SessionID, m.Model, m.Harness, m.State, roleName)
	return nil
}

// buildSessionManifest constructs the in-memory manifest for the new session.
func buildSessionManifest(sessionID, sessionName, workDir string, sandboxInfo *manifest.SandboxConfig, codexMeta *manifest.Codex) *manifest.Manifest {
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     sessionID,
		Name:          sessionName,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Lifecycle:     "",
		Workspace:     cfg.Workspace,
		Context: manifest.Context{
			Project: workDir,
			Tags:    buildSessionTags(roleName, sessionTags),
		},
		Tmux:             manifest.Tmux{SessionName: sessionName},
		Harness:          harnessName,
		Model:            modelName,
		ModelTier:        modelTierFlag,
		Claude:           manifest.Claude{},
		PermissionPolicy: clonePermissionPolicy(resolvedSessionPermissionPolicy),
		Sandbox:          sandboxInfo,
		Disposable:       disposable,
	}
	if codexMeta != nil {
		meta := *codexMeta
		m.Codex = &meta
	}
	if modeFlagValue != "" {
		now := time.Now()
		m.PermissionMode = modeFlagValue
		m.PermissionModeUpdatedAt = &now
		m.PermissionModeSource = "startup"
	}
	if disposable {
		m.DisposableTTL = disposableTTL
	}
	if harnessName == "opencode-cli" {
		m.OpenCode = &manifest.OpenCode{
			ServerPort: 4096,
			ServerHost: "localhost",
			AttachTime: time.Now(),
		}
		if envURL := os.Getenv("OPENCODE_SERVER_URL"); envURL != "" {
			m.OpenCode.ServerHost = envURL
		}
	}
	return m
}

func clonePermissionPolicy(policy *manifest.PermissionPolicy) *manifest.PermissionPolicy {
	if policy == nil {
		return nil
	}
	clone := *policy
	clone.Sources = append([]string{}, policy.Sources...)
	clone.Explicit = append([]string{}, policy.Explicit...)
	clone.Allow = append([]string{}, policy.Allow...)
	clone.Targets = append([]manifest.PermissionPolicyTarget{}, policy.Targets...)
	return &clone
}

// registerSessionInDolt persists m to the configured lifecycle store. Test
// environments resolve an isolated SQLite adapter through getStorage.
func registerSessionInDolt(m *manifest.Manifest) error {
	adapter, err := getStorage()
	if err != nil {
		debug.Log("Failed to connect to Dolt: %v", err)
		ui.PrintError(err, "Failed to connect to Dolt storage",
			"  • Ensure Dolt server is running\n"+
				"  • Check WORKSPACE environment variable is set")
		return err
	}
	defer func() { _ = adapter.Close() }()

	if err := adapter.CreateSession(m); err != nil {
		debug.Log("Failed to save session to Dolt: %v", err)
		ui.PrintError(err, "Failed to save session to Dolt",
			"  • Check database connection\n"+
				"  • Verify Dolt server is accessible")
		return err
	}
	debug.Log("Session saved to Dolt database: %s", m.SessionID)
	if testMode {
		ui.PrintSuccess("Test session registered in isolated database")
	} else {
		ui.PrintSuccess("Session registered in database")
	}
	return nil
}
