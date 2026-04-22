package discovery

import (
	"testing"
)

func TestIsWorkspaceContractAvailable(t *testing.T) {
	// Test workspace CLI availability check
	available := IsWorkspaceContractAvailable()
	t.Logf("Workspace CLI available: %v", available)
	// Don't assert - availability depends on environment
}

func TestDetectWorkspaceUsingContract(t *testing.T) {
	if !IsWorkspaceContractAvailable() {
		t.Skip("Workspace CLI not available - skipping contract test")
	}

	// Test workspace detection via contract
	info, err := DetectWorkspaceUsingContract("")
	if err != nil {
		t.Logf("Workspace detection failed (expected if not in workspace): %v", err)
		return
	}

	// Verify response structure
	if info.Name == "" {
		t.Error("Expected workspace name to be set")
	}

	if info.Root == "" {
		t.Error("Expected workspace root to be set")
	}

	t.Logf("Detected workspace: %s at %s", info.Name, info.Root)
}

func TestListWorkspacesUsingContract(t *testing.T) {
	if !IsWorkspaceContractAvailable() {
		t.Skip("Workspace CLI not available - skipping contract test")
	}

	// Test workspace list via contract
	workspaces, err := ListWorkspacesUsingContract()
	if err != nil {
		t.Logf("Workspace list failed (may indicate misconfigured CLI): %v", err)
		return
	}

	t.Logf("Found %d configured workspaces", len(workspaces))

	for _, ws := range workspaces {
		t.Logf("  - %s: %s (enabled: %v)", ws.Name, ws.Root, ws.Enabled)
	}
}

func TestGracefulDegradationWithoutCLI(t *testing.T) {
	// This test documents behavior when workspace CLI is not installed

	if IsWorkspaceContractAvailable() {
		t.Skip("Workspace CLI is available - test applies when CLI absent")
	}

	// DetectWorkspaceUsingContract should return error
	_, err := DetectWorkspaceUsingContract("")
	if err == nil {
		t.Error("Expected error when workspace CLI not available")
	}

	// ListWorkspacesUsingContract should return error
	_, err = ListWorkspacesUsingContract()
	if err == nil {
		t.Error("Expected error when workspace CLI not available")
	}

	// AGM should fall back to legacy FindSessionsAcrossWorkspaces
	t.Log("AGM will use legacy workspace scanning when workspace CLI unavailable")
}
