package router

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoleSpec_Candidates(t *testing.T) {
	cases := []struct {
		spec RoleSpec
		want []string
	}{
		{RoleSpec{}, nil},
		{RoleSpec{Primary: "a"}, []string{"a"}},
		{RoleSpec{Primary: "a", Tertiary: "c"}, []string{"a", "c"}},
		{RoleSpec{Primary: "a", Secondary: "b", Tertiary: "c"}, []string{"a", "b", "c"}},
		{RoleSpec{Secondary: "b"}, []string{"b"}},
	}
	for _, c := range cases {
		got := c.spec.Candidates()
		if len(got) != len(c.want) {
			t.Errorf("Candidates(%+v) length = %d, want %d", c.spec, len(got), len(c.want))
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("Candidates(%+v)[%d] = %q, want %q", c.spec, i, got[i], c.want[i])
			}
		}
	}
}

func TestParseConfig_Minimal(t *testing.T) {
	data := []byte(`version: 1
roles:
  research:
    primary: claude-opus-4-7
`)
	cfg, err := ParseConfig(data)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Version != 1 {
		t.Errorf("version = %d, want 1", cfg.Version)
	}
	if cfg.Roles["research"].Primary != "claude-opus-4-7" {
		t.Errorf("research.primary = %q", cfg.Roles["research"].Primary)
	}
}

func TestParseConfig_EmptyVersionDefaultsToOne(t *testing.T) {
	cfg, err := ParseConfig([]byte("roles: {orchestrator: {primary: gpt-4o}}\n"))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.Version != 1 {
		t.Errorf("version = %d, want 1 (default)", cfg.Version)
	}
}

func TestParseConfig_RejectsUnknownVersion(t *testing.T) {
	_, err := ParseConfig([]byte("version: 99\nroles: {}"))
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
}

func TestParseConfig_RejectsRoleWithoutCandidates(t *testing.T) {
	_, err := ParseConfig([]byte("version: 1\nroles:\n  empty: {}\n"))
	if err == nil {
		t.Fatal("expected error for role with no candidates")
	}
}

func TestParseConfig_RejectsUnknownDefaultRole(t *testing.T) {
	data := []byte(`version: 1
default_role: missing
roles:
  research: {primary: gpt-4o}
`)
	if _, err := ParseConfig(data); err == nil {
		t.Fatal("expected error for default_role not in roles")
	}
}

func TestParseConfig_AcceptsKnownDefaultRole(t *testing.T) {
	data := []byte(`version: 1
default_role: research
roles:
  research: {primary: gpt-4o}
`)
	cfg, err := ParseConfig(data)
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	if cfg.DefaultRole != "research" {
		t.Errorf("default_role = %q", cfg.DefaultRole)
	}
}

func TestLoadConfig_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "roles.yaml")
	const body = `version: 1
default_role: orchestrator
roles:
  orchestrator:
    primary: claude-sonnet-4-6
    secondary: gemini-2.5-flash
  implementer:
    primary: claude-opus-4-7
    tertiary: gpt-5.5-pro
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got := cfg.Roles["implementer"].Candidates(); len(got) != 2 || got[0] != "claude-opus-4-7" || got[1] != "gpt-5.5-pro" {
		t.Errorf("implementer candidates = %+v", got)
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	if _, err := LoadConfig(filepath.Join(t.TempDir(), "no-such-file.yaml")); err == nil {
		t.Fatal("expected error reading missing file")
	}
}
