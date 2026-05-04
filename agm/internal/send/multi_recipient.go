package send

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

// RecipientSpec represents parsed recipient input
type RecipientSpec struct {
	Raw           string   // Original input (e.g., "session1,session2" or "*.research")
	Type          string   // "direct", "comma_list", "glob", "workspace"
	Recipients    []string // Resolved session names/IDs
	ExcludeSender string   // Sender session name to exclude from recipients (Bug fix 2026-04-03)
}

// SessionResolver interface for dependency injection (enables testing)
type SessionResolver interface {
	// ResolveIdentifier finds session by ID, name, or tmux name
	ResolveIdentifier(identifier string) (*manifest.Manifest, error)
	// ListAllSessions returns all active (non-archived) sessions
	ListAllSessions() ([]*manifest.Manifest, error)
}

// ParseRecipients parses recipient specification from command args
// Supports:
//   - Direct single recipient: ["session-name"]
//   - Comma-separated list: ["session1,session2,session3"]
//   - Explicit --to flag: toFlag="session1,session2"
//   - All active sessions: allFlag=true (equivalent to glob "*")
//   - Workspace filtering: workspaceFlag="oss" (requires --to, args[0], or --all)
func ParseRecipients(args []string, toFlag string, workspaceFlag string, allFlag bool) (*RecipientSpec, error) {
	spec := &RecipientSpec{}

	// Handle --all flag (transforms to glob "*")
	if allFlag {
		spec.Raw = "*"
		spec.Type = "glob"
		spec.Recipients = []string{"*"}
		return spec, nil
	}

	// Priority: --to flag > args[0]
	var rawInput string
	switch {
	case toFlag != "":
		rawInput = toFlag
		spec.Raw = toFlag
	case len(args) > 0:
		rawInput = args[0]
		spec.Raw = args[0]
	default:
		return nil, fmt.Errorf("no recipient specified (use recipient, --to, or --all flag)")
	}

	// Detect input type and parse
	switch {
	case strings.Contains(rawInput, ","):
		// Comma-separated list: "session1,session2,session3"
		spec.Type = "comma_list"
		recipients := strings.Split(rawInput, ",")
		for _, r := range recipients {
			r = strings.TrimSpace(r)
			if r != "" {
				spec.Recipients = append(spec.Recipients, r)
			}
		}
	case strings.Contains(rawInput, "*") || strings.Contains(rawInput, "?"):
		// Glob pattern: "*research*", "test-*"
		spec.Type = "glob"
		spec.Recipients = []string{rawInput} // Store pattern, will resolve later
	default:
		// Direct single recipient: "session-name"
		spec.Type = "direct"
		spec.Recipients = []string{rawInput}
	}

	// Handle workspace filtering (combines with existing recipients)
	if workspaceFlag != "" {
		spec.Type = "workspace"
		// Store workspace filter for later resolution
		// We'll combine it with the main recipient list during resolution
	}

	// Validate we have at least one recipient
	if len(spec.Recipients) == 0 {
		return nil, fmt.Errorf("no valid recipients found in input: %s", rawInput)
	}

	return spec, nil
}

// ResolveRecipients resolves spec to actual session names/IDs
// - Validates all recipients exist
// - Expands glob patterns
// - Filters by workspace
// - Deduplicates results
// - Skips archived sessions
func ResolveRecipients(spec *RecipientSpec, resolver SessionResolver) (*RecipientSpec, error) {
	if spec == nil {
		return nil, fmt.Errorf("spec cannot be nil")
	}
	if resolver == nil {
		return nil, fmt.Errorf("resolver cannot be nil")
	}

	resolved := &RecipientSpec{
		Raw:  spec.Raw,
		Type: spec.Type,
	}

	// Get all sessions for glob/workspace filtering
	var allSessions []*manifest.Manifest
	if spec.Type == "glob" || spec.Type == "workspace" {
		sessions, err := resolver.ListAllSessions()
		if err != nil {
			return nil, fmt.Errorf("failed to list sessions: %w", err)
		}
		allSessions = sessions
	}

	// Resolve based on type
	switch spec.Type {
	case "direct":
		// Single direct recipient - validate it exists
		if len(spec.Recipients) != 1 {
			return nil, fmt.Errorf("direct type should have exactly 1 recipient")
		}
		m, err := resolver.ResolveIdentifier(spec.Recipients[0])
		if err != nil {
			return nil, fmt.Errorf("recipient '%s' not found: %w", spec.Recipients[0], err)
		}
		resolved.Recipients = []string{m.Tmux.SessionName}

	case "comma_list":
		// Comma-separated list - validate each exists
		seen := make(map[string]bool)
		for _, recipient := range spec.Recipients {
			m, err := resolver.ResolveIdentifier(recipient)
			if err != nil {
				return nil, fmt.Errorf("recipient '%s' not found: %w", recipient, err)
			}
			// Deduplicate using tmux session name
			if !seen[m.Tmux.SessionName] {
				seen[m.Tmux.SessionName] = true
				resolved.Recipients = append(resolved.Recipients, m.Tmux.SessionName)
			}
		}

	case "glob":
		// Glob pattern - expand to matching sessions
		if len(spec.Recipients) != 1 {
			return nil, fmt.Errorf("glob type should have exactly 1 pattern")
		}
		pattern := spec.Recipients[0]
		matches := resolveGlob(pattern, allSessions)
		if len(matches) == 0 {
			return nil, fmt.Errorf("no sessions match pattern: %s", pattern)
		}
		resolved.Recipients = matches

	case "workspace":
		// Workspace filter - filter sessions by workspace
		// Note: workspace filtering is handled by the adapter's ListSessions
		// This case shouldn't normally occur as workspace is combined with other types
		return nil, fmt.Errorf("workspace-only filtering not yet implemented")

	default:
		return nil, fmt.Errorf("unknown recipient type: %s", spec.Type)
	}

	// Exclude sender from recipients (Bug fix 2026-04-03: --all was delivering to self)
	if spec.ExcludeSender != "" {
		var filtered []string
		for _, r := range resolved.Recipients {
			if r != spec.ExcludeSender {
				filtered = append(filtered, r)
			}
		}
		resolved.Recipients = filtered
	}

	// Final validation
	if len(resolved.Recipients) == 0 {
		if spec.ExcludeSender != "" {
			return nil, fmt.Errorf("no recipients after excluding sender '%s'", spec.ExcludeSender)
		}
		return nil, fmt.Errorf("no valid recipients found after resolution")
	}

	return resolved, nil
}

// resolveGlob expands glob pattern to matching sessions
// Supports basic wildcards:
//   - * matches any characters (including none)
//   - ? matches exactly one character
//
// Filters out archived sessions automatically
func resolveGlob(pattern string, allSessions []*manifest.Manifest) []string {
	var matches []string
	seen := make(map[string]bool)

	for _, session := range allSessions {
		// Skip archived sessions
		if session.Lifecycle == manifest.LifecycleArchived {
			continue
		}

		// Try matching against tmux session name, manifest name, and session ID
		names := []string{
			session.Tmux.SessionName,
			session.Name,
			session.SessionID,
		}

		for _, name := range names {
			if matchGlob(pattern, name) {
				// Deduplicate using tmux session name
				if !seen[session.Tmux.SessionName] {
					seen[session.Tmux.SessionName] = true
					matches = append(matches, session.Tmux.SessionName)
				}
				break
			}
		}
	}

	return matches
}


// matchGlob performs simple glob pattern matching
// Supports * (any chars) and ? (single char)
func matchGlob(pattern, name string) bool {
	// Use filepath.Match for glob matching
	// filepath.Match supports * and ? wildcards
	matched, err := filepath.Match(pattern, name)
	if err != nil {
		// Invalid pattern, no match
		return false
	}
	return matched
}
