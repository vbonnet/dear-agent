package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The forbidden globs as declared in .dear-agent.yml > forbidden-paths.
var testPatterns = []string{
	"**/[Rr][Ee][Ss][Ee][Aa][Rr][Cc][Hh]/**",
	"**/[Rr][Ee][Ss][Ee][Aa][Rr][Cc][Hh].*",
	"**/[Rr][Ee][Pp][Oo][Rr][Tt].*",
	"**/[Pp][Ll][Aa][Nn].[Mm][Dd]",
	"**/*-[Pp][Ll][Aa][Nn].[Mm][Dd]",
	"**/[Pp][Ll][Aa][Nn].[Tt][Xx][Tt]",
	"**/*-[Pp][Ll][Aa][Nn].[Tt][Xx][Tt]",
	"**/[Pp][Ll][Aa][Nn].[Rr][Ss][Tt]",
	"**/*-[Pp][Ll][Aa][Nn].[Rr][Ss][Tt]",
	"**/[Pp][Ll][Aa][Nn].[Pp][Dd][Ff]",
	"**/*-[Pp][Ll][Aa][Nn].[Pp][Dd][Ff]",
	"**/[Pp][Ll][Aa][Nn].[Hh][Tt][Mm][Ll]",
	"**/*-[Pp][Ll][Aa][Nn].[Hh][Tt][Mm][Ll]",
	"**/docs/**/*-[Pp][Ll][Aa][Nn].*",
	"**/[Rr][Oo][Aa][Dd][Mm][Aa][Pp].*",
	"**/*-[Rr][Oo][Aa][Dd][Mm][Aa][Pp].*",
	"**/[Bb][Aa][Cc][Kk][Ll][Oo][Gg].*",
	"**/*-[Bb][Aa][Cc][Kk][Ll][Oo][Gg].*",
	"**/*-[Rr][Ee][Ss][Ee][Aa][Rr][Cc][Hh].*",
	"**/*-[Rr][Ee][Pp][Oo][Rr][Tt].*",
	"**/*_[Tt][Ee][Ss][Tt]_[Rr][Ee][Pp][Oo][Rr][Tt].[Mm][Dd]",
	"docs/retros/**",
	"docs/design/**",
	"wf/**",
	"**/.wayfinder/**",
	"**/WAYFINDER-STATUS.md",
	"**/WAYFINDER-HISTORY.md",
}

func TestForbidden(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		// Wayfinder SDLC run artifacts — must be blocked.
		{"wf/some-run/W0-charter.md", true},
		{"wf/ce-11fi/WAYFINDER-STATUS.md", true},
		{"agm/.wayfinder/foo/S6-design.md", true},
		{".wayfinder/run/D1.md", true},
		{"agm/wayfinder-oss-agm-g2/WAYFINDER-STATUS.md", true},
		{"agm/wayfinder-oss-agm-g2/WAYFINDER-HISTORY.md", true},
		{"WAYFINDER-STATUS.md", true}, // root-level still caught
		// Other temporal artifacts.
		{"docs/retros/2026-06-19-x.md", true},
		{"docs/design/y.md", true},
		{"research/notes.md", true},
		{"research/data.txt", true},
		{"packages/a/Research/raw-data.json", true},
		{"agm/docs/nested/release-plan.rst", true},
		{"ROADMAP.yaml", true},
		{"notes/team-backlog.toml", true},
		{"reports/incident-research.pdf", true},
		{"reports/incident-report.html", true},
		{"agm/internal/dolt/WORKSPACE_ISOLATION_TEST_REPORT.md", true},
		{"RESEARCH.md", true},
		{"RESEARCH.pdf", true},
		{"reports/REPORT.md", true},
		{"reports/REPORT.html", true},
		{"docs/PLAN.md", true},
		{"agm/docs/ops/PLAN.md", true},
		{"agm/TEST-PLAN.md", true},
		{"agm/CENTRALIZED-STORAGE-TEST-PLAN.md", true},
		{"engram/hooks-bin/GO-MIGRATION-PLAN.md", true},
		{"engram/release-plan.rst", true},
		{"agm/TEST-PLAN.txt", true},
		{"research.pdf", true},
		{"Research.md", true},
		{"report.html", true},
		// Wayfinder TOOL SOURCE and living docs — must NOT be blocked.
		{"wayfinder/SKILL.md", false},
		{"wayfinder/SPEC.md", false},
		{"wayfinder/cmd/wayfinder-session/internal/validator/testdata/d2-valid-100.md", false},
		{"pkg/validator/wayfinderartifact.go", false},
		{"agm/internal/a2a/wayfinder/wayfinder.go", false},
		{"docs/adr/ADR-035-wayfinder-enforcement.md", false},
		{"internal/telemetry/wayfinder_roi_logger.go", false},
		{"README.md", false},
		{"cmd/routing-guard/main.go", false},
		{"wf/run/repro.sh", true},
		{"pkg/demo/.wayfinder/session/generated.go", true},
		{".github/workflows/tofu-plan.yml", false},
		{"agm/test/integration/plan_continuity_test.go", false},
		{"pkg/backlog.go", false},
		{"cmd/audit-report.go", false},
	}
	for _, c := range cases {
		if got := forbidden(c.path, testPatterns); got != c.want {
			t.Errorf("forbidden(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestGlobPathMatch(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"**/*-plan.md", "docs/release-plan.md", true},
		{"**/*-plan.md", "agm/docs/ops/release-plan.rst", false},
		{"**/*-plan.md", "agm/ops/release-plan.md", true},
		{"**/research/**", "research/note.any", true},
		{"**/research/**", "a/research/nested/data.json", true},
		{"docs/design/**", "docs/design.md", false},
	}
	for _, c := range cases {
		if got := globPathMatch(c.pattern, c.path); got != c.want {
			t.Errorf("globPathMatch(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

func TestParseArgsRejectsRetiredBaselineFlag(t *testing.T) {
	if _, _, err := parseArgs([]string{"--all", "--baseline", "old.txt"}); err == nil {
		t.Fatal("retired --baseline flag accepted")
	}
}

func TestLoadPatterns(t *testing.T) {
	dir := t.TempDir()
	yml := filepath.Join(dir, ".dear-agent.yml")
	content := `version: 1
forbidden-paths:
  research:
    - wf/**
    - "**/WAYFINDER-STATUS.md"
`
	if err := os.WriteFile(yml, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := loadPatterns(yml)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("loadPatterns returned %d globs, want 2: %v", len(got), got)
	}
	// A missing file is not an error (nothing to enforce).
	if p, err := loadPatterns(filepath.Join(dir, "nope.yml")); err != nil || p != nil {
		t.Errorf("loadPatterns(missing) = (%v, %v), want (nil, nil)", p, err)
	}
}

func TestRepositoryTreeHasNoTemporalDebt(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	patterns, err := loadPatterns(filepath.Join(root, ".dear-agent.yml"))
	if err != nil {
		t.Fatal(err)
	}
	tracked, err := gitLines(root, "ls-files")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range tracked {
		if forbidden(name, patterns) {
			t.Errorf("tracked temporal artifact: %s", name)
		}
	}
}
