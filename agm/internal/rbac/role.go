// Package rbac implements role-based access control for AGM sessions.
//
// Each session role maps to a permission profile that defines which Claude Code
// tools and commands are pre-approved. Profiles follow least-privilege: only the
// permissions needed for the role's specific function are granted.
package rbac

import "fmt"

// Role represents an AGM session role in the VROOM agent hierarchy.
type Role string

// Recognized session role values.
const (
	RoleMetaOrchestrator Role = "meta-orchestrator"
	RoleOrchestrator     Role = "orchestrator"
	RoleOverseer         Role = "overseer"
	RoleSupervisor       Role = "supervisor"
	RoleWorker           Role = "worker"
	RoleImplementer      Role = "implementer"
	RoleResearcher       Role = "researcher"
	RoleVerifier         Role = "verifier"
	RoleRequester        Role = "requester"
	RoleAuditor          Role = "auditor"
	RoleMonitor          Role = "monitor"
)

// TrustLevel indicates the privilege tier for a role.
type TrustLevel int

// Trust level values, ordered from least to most privileged.
const (
	TrustSandboxed TrustLevel = 1
	TrustStandard  TrustLevel = 2
	TrustElevated  TrustLevel = 3
	TrustTrusted   TrustLevel = 4
)

// Profile defines the permission scope for a role.
type Profile struct {
	Name        Role
	Description string
	TrustLevel  TrustLevel

	// AllowedTools are Claude Code permission patterns written to
	// the project's .claude/settings.json permissions.allow list.
	AllowedTools []string
}

// allRoles lists every known role for validation and completion.
var allRoles = []Role{
	RoleMetaOrchestrator,
	RoleOrchestrator,
	RoleOverseer,
	RoleSupervisor,
	RoleWorker,
	RoleImplementer,
	RoleResearcher,
	RoleVerifier,
	RoleRequester,
	RoleAuditor,
	RoleMonitor,
}

// IsSupervisorRole returns true if the role is a supervisor-tier role
// (meta-orchestrator, orchestrator, overseer, or the supervisor alias).
func IsSupervisorRole(name string) bool {
	//nolint:exhaustive // intentional partial: handles the relevant subset
	switch Role(name) {
	case RoleMetaOrchestrator, RoleOrchestrator, RoleOverseer, RoleSupervisor:
		return true
	}
	return false
}

// AllRoleNames returns all valid role name strings.
func AllRoleNames() []string {
	names := make([]string, len(allRoles))
	for i, r := range allRoles {
		names[i] = string(r)
	}
	return names
}

// ValidRole returns true if the given string is a recognized role.
func ValidRole(name string) bool {
	_, ok := profiles[Role(name)]
	return ok
}

// LookupProfile returns the permission profile for a role name.
// Returns an error if the role is not recognized.
func LookupProfile(name string) (*Profile, error) {
	p, ok := profiles[Role(name)]
	if !ok {
		return nil, fmt.Errorf("unknown role %q: valid roles are %v", name, AllRoleNames())
	}
	return &p, nil
}

// ProfileNames returns all profile names that can be used with --permission-profile.
// This includes both role names and legacy profile aliases.
func ProfileNames() []string {
	seen := make(map[string]bool)
	var names []string
	for k := range profiles {
		s := string(k)
		if !seen[s] {
			seen[s] = true
			names = append(names, s)
		}
	}
	return names
}
