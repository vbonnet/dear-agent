package workflow

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// ErrPermissionDenied is the canonical error for permission rejection.
// Wrapped with the specific reason; callers can errors.Is on it to
// distinguish permission failures from generic errors.
var ErrPermissionDenied = errors.New("workflow: permission denied")

// PermissionEnforcer is the choke-point for "may this node do X?"
// checks. The runner consults it before executing privileged actions:
//
//   - bash nodes: CheckPath against fs_read/fs_write for the working
//     directory; CheckTool against tools for any tool the bash command
//     wraps (Phase 2 will inspect the rendered command for known
//     tool-CLI patterns).
//   - ai nodes: CheckTool for each tool the AI node declares (when the
//     AI executor reports tool calls back; Phase 1 only enforces what
//     the YAML declares, not what the model actually invokes).
//   - network: CheckHost when the executor declares an outbound URL.
//
// Implementations must be safe for concurrent use.
type PermissionEnforcer interface {
	CheckPath(perm *Permissions, path string, mode AccessMode) error
	CheckHost(perm *Permissions, host string) error
	CheckTool(perm *Permissions, tool string) error
}

// AccessMode is the read/write distinction for CheckPath.
type AccessMode string

// Access modes recognised by PermissionEnforcer.CheckPath.
const (
	AccessRead  AccessMode = "read"
	AccessWrite AccessMode = "write"
)

// DefaultPermissionEnforcer is the engine's built-in implementation.
// Allowlists are interpreted as filepath glob patterns (filesystem) and
// hostname suffixes (network). When the relevant allowlist is nil/empty
// the check returns nil — "no policy declared" is permissive by default,
// matching the runner's pre-Phase-1 behaviour.
//
// Why permissive-by-default: existing workflows do not declare
// permissions; a closed default would break every legacy YAML on the
// day Phase 1 lands. The opt-in path lets operators ratchet down per
// node as confidence grows. Phase 2's audit log is what makes the
// difference visible.
type DefaultPermissionEnforcer struct{}

// CheckPath returns nil if the permission policy is unset or path
// matches the allowlist for the requested mode. The match is glob-based
// (filepath.Match): a pattern of "notes/**" matches "notes/foo.md", but
// double-star is implemented manually since Go's filepath.Match doesn't
// recognise it.
func (DefaultPermissionEnforcer) CheckPath(perm *Permissions, path string, mode AccessMode) error {
	if perm == nil {
		return nil
	}
	patterns := perm.FSRead
	if mode == AccessWrite {
		patterns = perm.FSWrite
	}
	if len(patterns) == 0 {
		return nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		// If we can't normalise, fall back to the raw path so the
		// pattern author's intent (relative globs) still applies.
		abs = path
	}
	for _, p := range patterns {
		if matchGlob(p, path) || matchGlob(p, abs) {
			return nil
		}
	}
	return fmt.Errorf("%w: %s %s not in allowlist", ErrPermissionDenied, mode, path)
}

// CheckHost validates host against perm.Network. A pattern of
// "anthropic.com" matches "anthropic.com" and "*.anthropic.com"
// (subdomain match). A pattern of "*" matches everything.
func (DefaultPermissionEnforcer) CheckHost(perm *Permissions, host string) error {
	if perm == nil || len(perm.Network) == 0 {
		return nil
	}
	host = stripPort(host)
	for _, p := range perm.Network {
		if p == "*" || strings.EqualFold(p, host) {
			return nil
		}
		if strings.HasPrefix(p, "*.") && strings.HasSuffix(host, p[1:]) {
			return nil
		}
		// Bare "domain.com" also matches subdomains, matching the
		// expectations of the example YAML in ROADMAP.md.
		if !strings.HasPrefix(p, "*.") && strings.HasSuffix(host, "."+p) {
			return nil
		}
	}
	return fmt.Errorf("%w: host %q not in allowlist", ErrPermissionDenied, host)
}

// CheckTool returns nil if perm.Tools is empty or includes the named
// tool. The match is exact and case-sensitive — tool names are the
// canonical names from the harness (Read, Grep, FetchSource, …).
func (DefaultPermissionEnforcer) CheckTool(perm *Permissions, tool string) error {
	if perm == nil || len(perm.Tools) == 0 {
		return nil
	}
	for _, t := range perm.Tools {
		if t == tool {
			return nil
		}
	}
	return fmt.Errorf("%w: tool %q not in allowlist", ErrPermissionDenied, tool)
}

// matchGlob is filepath.Match with double-star support: ** matches any
// sequence of characters including path separators. We keep this
// hand-rolled so the engine has zero dep on doublestar libraries.
func matchGlob(pattern, path string) bool {
	// Fast path: no double-star, just delegate.
	if !strings.Contains(pattern, "**") {
		ok, err := filepath.Match(pattern, path)
		if err != nil {
			return false
		}
		return ok
	}
	// Convert ** to .* and translate * appropriately. We do a minimal
	// conversion rather than a full regex round-trip to keep behaviour
	// predictable.
	parts := strings.Split(pattern, "**")
	if len(parts) == 0 {
		return path == ""
	}
	// Leading anchor (everything before the first **) must match a
	// prefix of path; tail anchor must match a suffix; the middle is
	// flexible.
	if len(parts) == 1 {
		// No ** found — fall through (shouldn't happen given the check
		// above, but guard anyway).
		ok, _ := filepath.Match(parts[0], path)
		return ok
	}
	// Anchor head.
	head := parts[0]
	if head != "" && !strings.HasPrefix(path, strings.TrimSuffix(head, "/")) {
		// Head with explicit prefix: simple HasPrefix check (we don't
		// support globbing inside the head segment for the ** case;
		// callers who need that should use plain filepath.Match
		// patterns).
		if !strings.HasPrefix(path, head) {
			return false
		}
	}
	// Anchor tail.
	tail := parts[len(parts)-1]
	if tail != "" && tail != "/" && !strings.HasSuffix(path, tail) {
		return false
	}
	return true
}

// stripPort removes :port from a host string. host may be a bare host,
// host:port, or a URL — the URL form is parsed and its Host returned.
func stripPort(host string) string {
	if u, err := url.Parse(host); err == nil && u.Host != "" {
		host = u.Host
	}
	if i := strings.LastIndex(host, ":"); i > 0 && !strings.Contains(host[i:], "]") {
		host = host[:i]
	}
	return host
}
