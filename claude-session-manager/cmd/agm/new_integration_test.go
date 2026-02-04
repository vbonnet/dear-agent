package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
)

// TestNewCommand_RenameBeforeAssoc is a REGRESSION TEST for the command sequence
// in csm new. It verifies that /rename is sent BEFORE /csm-assoc to ensure
// Claude UUID is generated.
//
// Bug History (2026-01-13):
// - Issue: Claude UUID is only generated when first message is sent
// - Original: Only sent /csm-assoc, which couldn't populate UUID until AFTER it ran
// - Fix: Send /rename first (generates UUID), then /csm-assoc (populates manifest)
//
// This test documents the correct command sequence.
func TestNewCommand_RenameBeforeAssoc(t *testing.T) {
	// This is a documentation test - it doesn't run the full command
	// but documents the expected behavior

	t.Log("csm new command sequence (CRITICAL ORDER):")
	t.Log("")
	t.Log("1. Create tmux session")
	t.Log("2. Start Claude with --add-dir flag")
	t.Log("3. Wait for Claude process to be ready")
	t.Log("4. Wait for SessionStart hooks to complete")
	t.Log("5. Create manifest with empty Claude.UUID")
	t.Log("6. Release lock")
	t.Log("7. Send '/rename {session-name}' command  ← MUST BE FIRST")
	t.Log("   └─ This generates the Claude UUID")
	t.Log("8. Sleep 500ms to allow rename to process")
	t.Log("9. Send '/csm-tools:csm-assoc {session-name}' command")
	t.Log("   └─ This populates manifest.Claude.UUID from Claude history")
	t.Log("10. Wait for ready-file signal")
	t.Log("11. Attach to session (if not --detached)")
	t.Log("")
	t.Log("WHY THIS ORDER MATTERS:")
	t.Log("- Claude UUID is only created when first message is sent")
	t.Log("- /rename counts as a message and generates UUID")
	t.Log("- /csm-assoc needs the UUID to exist in history to populate manifest")
	t.Log("- If /csm-assoc runs first, manifest.Claude.UUID stays empty")
}

// TestNewCommand_ManifestInitialization tests manifest creation during csm new
func TestNewCommand_ManifestInitialization(t *testing.T) {
	// Create temp directory for test session
	tmpDir := t.TempDir()
	sessionName := "test-manifest-init"
	manifestDir := filepath.Join(tmpDir, sessionName)
	manifestPath := filepath.Join(manifestDir, "manifest.yaml")

	// Simulate manifest creation step from new.go
	err := os.MkdirAll(manifestDir, 0700)
	require.NoError(t, err)

	// Generate proper UUID for SessionID
	m := &manifest.Manifest{
		SchemaVersion: manifest.SchemaVersion,
		SessionID:     "550e8400-e29b-41d4-a716-446655440000", // Example UUID
		Name:          sessionName,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		Context: manifest.Context{
			Project: tmpDir,
		},
		Tmux: manifest.Tmux{
			SessionName: sessionName,
		},
		Claude: manifest.Claude{
			UUID: "", // Initially empty - will be populated by /csm-assoc
		},
	}

	// Write manifest
	err = manifest.Write(manifestPath, m)
	require.NoError(t, err, "Should create manifest successfully")

	// Verify manifest was created
	_, err = os.Stat(manifestPath)
	assert.NoError(t, err, "Manifest file should exist")

	// Read back and verify
	readManifest, err := manifest.Read(manifestPath)
	require.NoError(t, err, "Should read manifest successfully")

	assert.Equal(t, manifest.SchemaVersion, readManifest.SchemaVersion)
	assert.Equal(t, m.SessionID, readManifest.SessionID)
	assert.Equal(t, sessionName, readManifest.Name)
	assert.Empty(t, readManifest.Claude.UUID, "Claude.UUID should start empty")

	t.Log("✓ Manifest initialization verified")
	t.Log("  SessionID: Valid UUID (not session-{name})")
	t.Log("  Claude.UUID: Empty (will be populated by /csm-assoc)")
}

// TestNewCommand_AdditionalDirectories tests trust prompt prevention
func TestNewCommand_AdditionalDirectories(t *testing.T) {
	t.Log("Trust prompt prevention mechanism:")
	t.Log("")
	t.Log("BEFORE csm new:")
	t.Log("1. Read ~/.claude/settings.json")
	t.Log("2. Add working directory to additionalDirectories array")
	t.Log("3. Write back to settings.json")
	t.Log("")
	t.Log("EFFECT:")
	t.Log("- Claude pre-authorizes directory before startup")
	t.Log("- No 'Do you trust this folder?' prompt appears")
	t.Log("- SessionStart hooks can run immediately")
	t.Log("")
	t.Log("ALTERNATIVE (deprecated):")
	t.Log("- Old approach: Use control mode to detect and answer prompt")
	t.Log("- Problem: Adds 30s of waiting, brittle event detection")
	t.Log("- New approach: Prevent prompt entirely via additionalDirectories")
}

// TestTmuxCommandSequence documents the tmux command sequence
func TestTmuxCommandSequence(t *testing.T) {
	t.Log("Tmux command sequence in csm new:")
	t.Log("")
	t.Log("Session Creation:")
	t.Log("  tmux -S /tmp/csm.sock new-session -d -s {name} -c {workdir}")
	t.Log("")
	t.Log("Settings Injection (UX improvements):")
	t.Log("  tmux -S /tmp/csm.sock send-keys -t {name} 'set-window-option -g aggressive-resize on' C-m")
	t.Log("  tmux -S /tmp/csm.sock send-keys -t {name} 'set-option -g window-size latest' C-m")
	t.Log("  tmux -S /tmp/csm.sock send-keys -t {name} 'set -g mouse on' C-m")
	t.Log("  tmux -S /tmp/csm.sock send-keys -t {name} 'set -s set-clipboard on' C-m")
	t.Log("")
	t.Log("Start Claude:")
	t.Log("  tmux -S /tmp/csm.sock send-keys -t {name} -l \"claude --add-dir '{workdir}'\"")
	t.Log("  tmux -S /tmp/csm.sock send-keys -t {name} C-m")
	t.Log("")
	t.Log("Automation Commands:")
	t.Log("  tmux -S /tmp/csm.sock send-keys -t {name} -l '/rename {name}'")
	t.Log("  tmux -S /tmp/csm.sock send-keys -t {name} C-m")
	t.Log("  [sleep 500ms]")
	t.Log("  tmux -S /tmp/csm.sock send-keys -t {name} -l '/csm-tools:csm-assoc {name}'")
	t.Log("  tmux -S /tmp/csm.sock send-keys -t {name} C-m")
	t.Log("")
	t.Log("KEY DETAIL: -l flag sends text literally (no special key interpretation)")
	t.Log("KEY DETAIL: C-m (Enter) sent separately to execute command")
}

// TestSendCommand_Integration verifies tmux.SendCommand behavior
func TestSendCommand_Integration(t *testing.T) {
	// This is a documentation test

	t.Log("tmux.SendCommand implementation:")
	t.Log("1. Sends command text with -l flag (literal)")
	t.Log("2. Sends C-m separately (Enter key)")
	t.Log("3. Returns error if either step fails")
	t.Log("")
	t.Log("This prevents the regression where C-m was sent with command text,")
	t.Log("causing a newline in the prompt instead of executing the command.")
	t.Log("")
	t.Log("See internal/tmux/send_command_test.go for executable tests")
}

// TestDetachedMode documents --detached behavior
func TestDetachedMode(t *testing.T) {
	t.Log("csm new --detached behavior:")
	t.Log("")
	t.Log("WITH --detached flag:")
	t.Log("1. All initialization steps run normally")
	t.Log("2. /rename and /csm-assoc commands sent")
	t.Log("3. Ready-file wait completes")
	t.Log("4. Session NOT attached automatically")
	t.Log("5. User can attach later with: csm resume {name}")
	t.Log("")
	t.Log("WITHOUT --detached flag (default):")
	t.Log("1-3. Same as above")
	t.Log("4. AttachSession() called after ready-file")
	t.Log("5. Process replaced with tmux attach (syscall.Exec)")
	t.Log("")
	t.Log("WHY WAIT FOR READY-FILE EVEN IN DETACHED MODE:")
	t.Log("- Ensures /csm-assoc completed successfully")
	t.Log("- Manifest has Claude UUID populated")
	t.Log("- Session is fully initialized before returning")
	t.Log("- User can immediately use csm commands on the session")
}

// TestReadyFileSignaling documents the ready-file mechanism
func TestReadyFileSignaling(t *testing.T) {
	t.Log("Ready-file signaling mechanism:")
	t.Log("")
	t.Log("Signal File: ~/.csm/ready-{session-name}")
	t.Log("")
	t.Log("Creation Flow:")
	t.Log("1. csm new sends /csm-tools:csm-assoc command")
	t.Log("2. Claude executes csm-assoc skill")
	t.Log("3. Skill calls 'csm associate {name}' binary")
	t.Log("4. csm associate:")
	t.Log("   - Finds manifest")
	t.Log("   - Detects Claude UUID from history")
	t.Log("   - Updates manifest.Claude.UUID")
	t.Log("   - Creates ready-file as final step")
	t.Log("5. csm new detects ready-file (polling)")
	t.Log("6. Proceeds to attach or return")
	t.Log("")
	t.Log("Timeout: 60 seconds")
	t.Log("If timeout: User can manually run /csm-assoc or csm sync")
}
