package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/debug"
	"github.com/vbonnet/dear-agent/agm/internal/git"
	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/ui"
)

// createAndRegisterManifest writes the manifest directory, builds the v2
// manifest, and registers it in Dolt (skipping Dolt only in test sandbox mode).
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
	m := buildSessionManifest(sessionID, sessionName, workDir, sandboxInfo)
	if testMode {
		m.IsTest = true
		debug.Log("Marking session as test (is_test=true)")
	}

	debug.Phase("Register in Dolt Database")
	if err := registerSessionInDolt(m); err != nil {
		return err
	}
	_ = git.CommitManifest(manifestPath, "create", sessionName)
	return nil
}

// buildSessionManifest constructs the in-memory manifest for the new session.
func buildSessionManifest(sessionID, sessionName, workDir string, sandboxInfo *manifest.SandboxConfig) *manifest.Manifest {
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
		Tmux:       manifest.Tmux{SessionName: sessionName},
		Harness:    harnessName,
		Model:      modelName,
		Claude:     manifest.Claude{},
		Sandbox:    sandboxInfo,
		Disposable: disposable,
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

// registerSessionInDolt persists m to Dolt. In the test-sandbox env (where Dolt
// is intentionally unavailable) the failure is swallowed.
func registerSessionInDolt(m *manifest.Manifest) error {
	adapter, err := getStorage()
	if err != nil {
		if os.Getenv("AGM_TEST_RUN_ID") != "" {
			debug.Log("Test sandbox: Dolt unavailable (expected): %v", err)
			ui.PrintSuccess("Test sandbox session created (Dolt skipped)")
			return nil
		}
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
		ui.PrintSuccess("Test session registered in database (hidden from default list)")
	} else {
		ui.PrintSuccess("Session registered in database")
	}
	return nil
}
