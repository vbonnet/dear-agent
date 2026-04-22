package rbac

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// DefaultPermissions are safe, read-only commands always pre-approved
// to eliminate the "permission tax" on session startup.
var DefaultPermissions = []string{
	"Bash(git status)",
	"Bash(git status *)",
	"Bash(git -C * status *)",
	"Bash(git log *)",
	"Bash(git -C * log *)",
	"Bash(git branch *)",
	"Bash(git -C * branch *)",
	"Bash(git diff *)",
	"Bash(git -C * diff *)",
	"Bash(git show *)",
	"Bash(git -C * show *)",
	"Bash(git rev-parse *)",
	"Bash(git -C * rev-parse *)",
	"Bash(git worktree list *)",
	"Bash(git -C * worktree list *)",
	"Bash(agm *)",
	"Bash(agm session *)",
	"Bash(go version *)",
	"Bash(go env *)",
	"Bash(chmod +x /tmp/*)",
}

// ResolveOptions configures how permissions are resolved.
type ResolveOptions struct {
	// Explicit permission patterns from --permissions-allow flags.
	Explicit []string

	// ProfileName is the --permission-profile value (a role name).
	ProfileName string

	// InheritParent reads permissions from ~/.claude/settings.json.
	InheritParent bool
}

// ResolvePermissions merges default permissions, explicit flags, profile
// permissions, and optionally inherited parent permissions into a deduplicated
// allowlist.
func ResolvePermissions(opts ResolveOptions) ([]string, error) {
	merged := append([]string{}, DefaultPermissions...)

	// 1. Explicit --permissions-allow entries
	merged = append(merged, opts.Explicit...)

	// 2. Profile permissions (from role name)
	if opts.ProfileName != "" {
		profile, err := LookupProfile(opts.ProfileName)
		if err != nil {
			return nil, err
		}
		merged = append(merged, profile.AllowedTools...)
	}

	// 3. Inherit parent permissions
	if opts.InheritParent {
		parentPerms, err := ReadParentPermissions()
		if err != nil {
			return nil, fmt.Errorf("failed to read parent permissions: %w", err)
		}
		merged = append(merged, parentPerms...)
	}

	return deduplicate(merged), nil
}

// ReadParentPermissions reads permissions.allow from ~/.claude/settings.json.
func ReadParentPermissions() ([]string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read settings.json: %w", err)
	}

	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("failed to parse settings.json: %w", err)
	}

	perms, ok := settings["permissions"]
	if !ok {
		return nil, nil
	}
	permsMap, ok := perms.(map[string]interface{})
	if !ok {
		return nil, nil
	}
	allow, ok := permsMap["allow"]
	if !ok {
		return nil, nil
	}
	allowList, ok := allow.([]interface{})
	if !ok {
		return nil, nil
	}

	var result []string
	for _, item := range allowList {
		if str, ok := item.(string); ok {
			result = append(result, str)
		}
	}
	return result, nil
}

// ConfigureProjectPermissions writes a project-level .claude/settings.local.json
// in the working directory with the specified permissions.allow entries.
// Uses settings.local.json (not settings.json) to avoid modifying git-tracked
// files. settings.local.json is a Claude Code local override that is not
// committed to version control.
func ConfigureProjectPermissions(workDir string, allowList []string) error {
	if len(allowList) == 0 {
		return nil
	}

	claudeDir := filepath.Join(workDir, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("failed to create .claude directory: %w", err)
	}

	settingsPath := filepath.Join(claudeDir, "settings.local.json")

	var settings map[string]interface{}
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			settings = make(map[string]interface{})
		} else {
			return fmt.Errorf("failed to read settings.local.json: %w", err)
		}
	} else {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("failed to parse settings.local.json: %w", err)
		}
	}

	var permsMap map[string]interface{}
	if existing, ok := settings["permissions"]; ok {
		if pm, ok := existing.(map[string]interface{}); ok {
			permsMap = pm
		} else {
			permsMap = make(map[string]interface{})
		}
	} else {
		permsMap = make(map[string]interface{})
	}

	// Merge with existing allow list
	var existingAllow []string
	if ea, ok := permsMap["allow"]; ok {
		if eaList, ok := ea.([]interface{}); ok {
			for _, item := range eaList {
				if str, ok := item.(string); ok {
					existingAllow = append(existingAllow, str)
				}
			}
		}
	}

	merged := deduplicate(append(existingAllow, allowList...))

	// Convert to []interface{} for JSON
	allowIface := make([]interface{}, len(merged))
	for i, s := range merged {
		allowIface[i] = s
	}
	permsMap["allow"] = allowIface
	settings["permissions"] = permsMap

	output, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	if err := os.WriteFile(settingsPath, output, 0600); err != nil {
		return fmt.Errorf("failed to write settings.local.json: %w", err)
	}

	return nil
}

func deduplicate(items []string) []string {
	seen := make(map[string]bool, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}
