package bus

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

// ACL decides whether a send from one session id to another is permitted.
// The zero value denies every cross-session send (default-deny); explicit
// Allow/Rule entries opt specific pairs into the permitted set.
//
// A session may always send to itself (loopback — useful for self-ping
// heartbeats). A nil *ACL on Server means no ACL enforcement — all sends
// allowed — suitable for single-user dev setups and tests.
type ACL struct {
	// Rules is an ordered list of allow rules. First match wins. A rule
	// with Wildcard From or To matches any session for that side.
	Rules []ACLRule `yaml:"rules"`

	// DefaultAllow flips the decision from default-deny to default-allow
	// when no rule matches. Leave false for production; true for
	// low-friction local dev where the operator trusts all their own sessions.
	DefaultAllow bool `yaml:"default_allow"`
}

// ACLRule is a single allow rule in an ACL. Empty or "*" matches anything.
// Matching is exact-string on non-wildcard values; no prefix/glob syntax.
type ACLRule struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
}

// ACLDecision carries the outcome of Check so logs can include the rule
// that matched (for auditing). Allowed=false with Reason=="default-deny"
// means no rule matched and DefaultAllow was false.
type ACLDecision struct {
	Allowed bool
	Reason  string
}

// Check returns an ACLDecision for sending from sender to target.
// Self-sends are always allowed. A nil ACL receiver means no enforcement
// (allow-all); this branch makes it ergonomic to hand an optional ACL
// to the server without needing nil-checks at call sites.
func (a *ACL) Check(sender, target string) ACLDecision {
	if a == nil {
		return ACLDecision{Allowed: true, Reason: "no-acl"}
	}
	if sender == target {
		return ACLDecision{Allowed: true, Reason: "self-send"}
	}
	for i, r := range a.Rules {
		if ruleMatches(r.From, sender) && ruleMatches(r.To, target) {
			return ACLDecision{Allowed: true, Reason: fmt.Sprintf("rule %d", i)}
		}
	}
	if a.DefaultAllow {
		return ACLDecision{Allowed: true, Reason: "default-allow"}
	}
	return ACLDecision{Allowed: false, Reason: "default-deny"}
}

// ruleMatches returns true when pattern is "*" or "" (both treated as
// wildcards) or when pattern equals actual exactly.
func ruleMatches(pattern, actual string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	return pattern == actual
}

// LoadACL reads an ACL from path. Returns (nil, nil) if the file does not
// exist — callers interpret that as "no ACL configured, no enforcement".
// YAML parse errors are returned verbatim so ops can fix their config.
func LoadACL(path string) (*ACL, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("acl: read %s: %w", path, err)
	}
	var acl ACL
	if err := yaml.Unmarshal(b, &acl); err != nil {
		return nil, fmt.Errorf("acl: parse %s: %w", path, err)
	}
	return &acl, nil
}

// ReloadableACL wraps an ACL with a mutex so the server can hot-swap the
// policy without restarting. Reload re-reads the backing file and replaces
// the in-memory policy atomically on success; on parse failure the old
// policy remains in force and Reload returns the error.
type ReloadableACL struct {
	Path string

	mu  sync.RWMutex
	acl *ACL
}

// NewReloadableACL constructs a ReloadableACL that eagerly loads path.
// A missing file is fine: the wrapper holds a nil *ACL, which Check treats
// as allow-all. Call Reload later to pick up a newly-created file.
func NewReloadableACL(path string) (*ReloadableACL, error) {
	r := &ReloadableACL{Path: path}
	if err := r.Reload(); err != nil {
		return nil, err
	}
	return r, nil
}

// Reload re-reads the ACL file and swaps the policy. Returns any parse
// error; the previous policy is preserved on failure.
func (r *ReloadableACL) Reload() error {
	acl, err := LoadACL(r.Path)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.acl = acl
	r.mu.Unlock()
	return nil
}

// Check forwards to the current ACL under a read lock.
func (r *ReloadableACL) Check(sender, target string) ACLDecision {
	r.mu.RLock()
	acl := r.acl
	r.mu.RUnlock()
	return acl.Check(sender, target)
}
