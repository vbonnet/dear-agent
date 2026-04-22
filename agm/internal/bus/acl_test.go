package bus

import (
	"os"
	"path/filepath"
	"testing"
)

func TestACLCheckDefaultDeny(t *testing.T) {
	acl := &ACL{}
	d := acl.Check("s1", "s2")
	if d.Allowed {
		t.Errorf("empty ACL with default-deny should reject s1->s2, got %+v", d)
	}
	if d.Reason != "default-deny" {
		t.Errorf("Reason = %q, want default-deny", d.Reason)
	}
}

func TestACLCheckDefaultAllow(t *testing.T) {
	acl := &ACL{DefaultAllow: true}
	if d := acl.Check("s1", "s2"); !d.Allowed {
		t.Errorf("default-allow ACL should permit, got %+v", d)
	}
}

func TestACLCheckNilReceiverAllowsAll(t *testing.T) {
	var acl *ACL
	if d := acl.Check("any", "other"); !d.Allowed {
		t.Errorf("nil ACL should allow-all, got %+v", d)
	}
}

func TestACLCheckSelfSend(t *testing.T) {
	acl := &ACL{} // default-deny
	if d := acl.Check("s1", "s1"); !d.Allowed {
		t.Errorf("self-send should always be allowed, got %+v", d)
	}
}

func TestACLCheckRuleExact(t *testing.T) {
	acl := &ACL{
		Rules: []ACLRule{
			{From: "w1", To: "s1"},
		},
	}
	if d := acl.Check("w1", "s1"); !d.Allowed {
		t.Errorf("exact match should allow, got %+v", d)
	}
	if d := acl.Check("w2", "s1"); d.Allowed {
		t.Errorf("non-matching sender should deny, got %+v", d)
	}
	if d := acl.Check("w1", "s2"); d.Allowed {
		t.Errorf("non-matching target should deny, got %+v", d)
	}
}

func TestACLCheckRuleWildcard(t *testing.T) {
	acl := &ACL{
		Rules: []ACLRule{
			{From: "*", To: "s1"}, // any sender -> s1
			{From: "s2", To: "*"}, // s2 -> any
		},
	}
	if d := acl.Check("anyone", "s1"); !d.Allowed {
		t.Errorf("*-to-s1 should allow, got %+v", d)
	}
	if d := acl.Check("s2", "random"); !d.Allowed {
		t.Errorf("s2-to-* should allow, got %+v", d)
	}
	if d := acl.Check("w1", "w2"); d.Allowed {
		t.Errorf("no rule matches, should deny, got %+v", d)
	}
}

func TestACLCheckEmptyFieldTreatedAsWildcard(t *testing.T) {
	acl := &ACL{Rules: []ACLRule{{From: "s1", To: ""}}}
	if d := acl.Check("s1", "anything"); !d.Allowed {
		t.Errorf("empty To should be wildcard, got %+v", d)
	}
}

func TestACLFirstMatchWins(t *testing.T) {
	acl := &ACL{
		Rules: []ACLRule{
			{From: "s1", To: "s2"},
			{From: "s1", To: "s2"}, // duplicate
		},
	}
	d := acl.Check("s1", "s2")
	if !d.Allowed || d.Reason != "rule 0" {
		t.Errorf("first match should be rule 0, got %+v", d)
	}
}

func TestLoadACLMissingFile(t *testing.T) {
	acl, err := LoadACL("/does/not/exist/acl.yaml")
	if err != nil {
		t.Errorf("LoadACL on missing file: %v", err)
	}
	if acl != nil {
		t.Errorf("LoadACL on missing file should return nil, got %+v", acl)
	}
}

func TestLoadACLValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "acl.yaml")
	contents := `
default_allow: false
rules:
  - {from: "w1", to: "s1"}
  - {from: "s1", to: "*"}
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	acl, err := LoadACL(path)
	if err != nil {
		t.Fatalf("LoadACL: %v", err)
	}
	if len(acl.Rules) != 2 {
		t.Fatalf("Rules = %d, want 2", len(acl.Rules))
	}
	if acl.Rules[0].From != "w1" || acl.Rules[1].To != "*" {
		t.Errorf("rules parsed wrong: %+v", acl.Rules)
	}
}

func TestLoadACLMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("rules: [not-a-map"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadACL(path)
	if err == nil {
		t.Error("LoadACL on bad YAML should error")
	}
}

func TestReloadableACL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "acl.yaml")

	// Start with an empty file — allow-all (nil ACL).
	rac, err := NewReloadableACL(path)
	if err != nil {
		t.Fatal(err)
	}
	if d := rac.Check("a", "b"); !d.Allowed {
		t.Errorf("missing-file ACL should allow, got %+v", d)
	}

	// Now write a default-deny policy and reload.
	if err := os.WriteFile(path, []byte("default_allow: false\nrules: []"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := rac.Reload(); err != nil {
		t.Fatal(err)
	}
	if d := rac.Check("a", "b"); d.Allowed {
		t.Errorf("after reload to default-deny, got %+v", d)
	}

	// Malformed reload should preserve the prior policy.
	if err := os.WriteFile(path, []byte("rules: [malformed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := rac.Reload(); err == nil {
		t.Error("malformed reload should error")
	}
	// Still the old (default-deny) policy.
	if d := rac.Check("a", "b"); d.Allowed {
		t.Errorf("policy should survive malformed reload, got %+v", d)
	}
}
