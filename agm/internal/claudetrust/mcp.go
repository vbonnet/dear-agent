package claudetrust

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// mcpApprovalKey is the Claude Code project setting that pre-answers the
// "N new MCP servers found in this project" prompt.
const mcpApprovalKey = "enableAllProjectMcpServers"

// ApproveProjectMcpServers pre-answers Claude Code's project MCP-server prompt
// for a sandbox workspace.
//
// A repository that ships a .mcp.json makes Claude ask, on first run in each new
// directory, which of its servers to enable. The prompt owns input exactly like
// the trust dialog does, so an unattended spawn stops there having never reached
// its composer. Seeding trust alone therefore only moves the stall one dialog
// later.
//
// This grants the servers the project itself declares, in a throwaway sandbox
// clone of that project, alongside the permission allowlist AGM already
// pre-approves into the same file. It is still a capability grant — MCP servers
// can execute code — so it is deliberately confined to sandbox workspaces and is
// never written into a real checkout the user works in.
func ApproveProjectMcpServers(workDir string) error {
	settingsPath := filepath.Join(workDir, ".claude", "settings.local.json")

	// The sandbox clone carries whatever the cloned project contained,
	// symlinks included. Following a linked .claude directory or settings leaf
	// would write this capability grant through to a user-writable path
	// outside the sandbox, which is exactly the real checkout this function
	// promises never to touch. Refuse the link rather than resolving it; the
	// caller degrades to Claude's own prompt.
	if err := refuseSymlinkedPath(filepath.Dir(settingsPath)); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return fmt.Errorf("create %q: %w", filepath.Dir(settingsPath), err)
	}
	if err := refuseSymlinkedPath(settingsPath); err != nil {
		return err
	}

	settings, err := readSettings(settingsPath)
	if err != nil {
		return err
	}
	if settings[mcpApprovalKey] == true {
		return nil
	}
	settings[mcpApprovalKey] = true

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %q: %w", settingsPath, err)
	}
	if err := os.WriteFile(settingsPath, data, 0o600); err != nil {
		return fmt.Errorf("write %q: %w", settingsPath, err)
	}
	return nil
}

// readSettings preserves whatever AGM's permission pass already wrote, and any
// settings the cloned project ships of its own.
func readSettings(settingsPath string) (map[string]any, error) {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("read %q: %w", settingsPath, err)
	}
	if len(data) == 0 {
		return map[string]any{}, nil
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parse %q: %w", settingsPath, err)
	}
	if settings == nil {
		settings = map[string]any{}
	}
	return settings, nil
}

// refuseSymlinkedPath reports an error when path exists and is a symlink. An
// absent path is fine: it is about to be created inside the sandbox.
func refuseSymlinkedPath(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect %q: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refuse symlinked %q: a linked path can escape the sandbox", path)
	}
	return nil
}
