package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
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
	"**/[Pp][Rr][Ii][Oo][Rr][Ii][Tt][Ii][Ee][Ss].*",
	"**/*-[Pp][Rr][Ii][Oo][Rr][Ii][Tt][Ii][Ee][Ss].*",
	"**/[Bb][Aa][Cc][Kk][Ll][Oo][Gg].*",
	"**/*-[Bb][Aa][Cc][Kk][Ll][Oo][Gg].*",
	"**/*-[Rr][Ee][Ss][Ee][Aa][Rr][Cc][Hh].*",
	"**/*-[Rr][Ee][Pp][Oo][Rr][Tt].*",
	"**/*_[Tt][Ee][Ss][Tt]_[Rr][Ee][Pp][Oo][Rr][Tt].[Mm][Dd]",
	"**/[Rr][Ee][Ss][Ee][Aa][Rr][Cc][Hh][_A-Z]*.*",
	"**/*_[Rr][Ee][Ss][Ee][Aa][Rr][Cc][Hh].*",
	"**/*_[Rr][Ee][Ss][Ee][Aa][Rr][Cc][Hh]_*.*",
	"**/*[a-z]Research*.*",
	"**/[Rr][Ee][Pp][Oo][Rr][Tt][_A-Z]*.*",
	"**/*_[Rr][Ee][Pp][Oo][Rr][Tt].*",
	"**/*_[Rr][Ee][Pp][Oo][Rr][Tt]_*.*",
	"**/*[a-z]Report*.*",
	"**/[Pp][Ll][Aa][Nn][_A-Z]*.*",
	"**/*_[Pp][Ll][Aa][Nn].*",
	"**/*_[Pp][Ll][Aa][Nn]_*.*",
	"**/*[a-z]Plan*.*",
	"**/[Rr][Oo][Aa][Dd][Mm][Aa][Pp][_A-Z]*.*",
	"**/*_[Rr][Oo][Aa][Dd][Mm][Aa][Pp].*",
	"**/*_[Rr][Oo][Aa][Dd][Mm][Aa][Pp]_*.*",
	"**/*[a-z]Roadmap*.*",
	"**/[Pp][Rr][Ii][Oo][Rr][Ii][Tt][Ii][Ee][Ss][_A-Z]*.*",
	"**/*_[Pp][Rr][Ii][Oo][Rr][Ii][Tt][Ii][Ee][Ss].*",
	"**/*_[Pp][Rr][Ii][Oo][Rr][Ii][Tt][Ii][Ee][Ss]_*.*",
	"**/*[a-z]Priorities*.*",
	"**/[Bb][Aa][Cc][Kk][Ll][Oo][Gg][_A-Z]*.*",
	"**/*_[Bb][Aa][Cc][Kk][Ll][Oo][Gg].*",
	"**/*_[Bb][Aa][Cc][Kk][Ll][Oo][Gg]_*.*",
	"**/*[a-z]Backlog*.*",
	"docs/retros/**",
	"docs/design/**",
	"wf/**",
	"**/.wayfinder/**",
	"**/WAYFINDER-STATUS.md",
	"**/WAYFINDER-HISTORY.jsonl",
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
		{"agm/wayfinder-oss-agm-g2/WAYFINDER-HISTORY.jsonl", true},
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
		{"docs/RESEARCH_NOTES.md", true},
		{"docs/researchNotes.md", true},
		{"docs/sprint_backlog_2026.md", true},
		{"docs/sprintBacklog2026.md", true},
		{"docs/project_research.md", true},
		{"docs/incident_report.md", true},
		{"docs/release_plan.pdf", true},
		{"docs/product_roadmap.txt", true},
		// A roadmap renamed to PRIORITIES is the same temporal artifact.
		{"STRATEGIC-PRIORITIES.md", true},
		{"PRIORITIES.md", true},
		{"docs/PRIORITIES.yaml", true},
		{"docs/PRIORITIES_2026.md", true},
		{"docs/team_priorities.md", true},
		{"docs/sprintPriorities.md", true},
		{"docs/team_backlog.txt", true},
		// Wayfinder TOOL SOURCE and living docs — must NOT be blocked.
		{"wayfinder/SKILL.md", false},
		{"wayfinder/SPEC.md", false},
		{"wayfinder/cmd/wayfinder-session/internal/validator/testdata/d2-valid-100.md", false},
		{"pkg/validator/wayfinderartifact.go", false},
		{"agm/internal/a2a/wayfinder/wayfinder.go", false},
		{"docs/adr/ADR-036-wayfinder-enforcement.md", false},
		{"internal/telemetry/wayfinder_roi_logger.go", false},
		{"README.md", false},
		{"cmd/routing-guard/main.go", false},
		{"wf/run/repro.sh", true},
		{"pkg/demo/.wayfinder/session/generated.go", true},
		{".github/workflows/tofu-plan.yml", false},
		{"agm/test/integration/plan_continuity_test.go", false},
		{"pkg/backlog.go", false},
		{"pkg/scheduler/priorities.go", false},
		{"agm/internal/messages/priority.go", false},
		{"cmd/audit-report.go", false},
		{"infra/plan.tf", false},
		{"web/research.mjs", false},
		{"scripts/report.bats", false},
		{"schema/team_backlog.sql", false},
		{"docs/plan.d2", false},
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
	if _, err := loadPatterns(filepath.Join(dir, "nope.yml")); err == nil {
		t.Error("missing policy must fail closed")
	}
	if err := os.WriteFile(yml, []byte("version: 1\nforbidden-paths: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadPatterns(yml); err == nil {
		t.Error("empty forbidden-paths must fail closed")
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

// TestWayfinderSessionLeavesNoTemporalDebt is the end-to-end regression for
// ce-2sgej. The mandated PR path used to block itself: `wayfinder session
// start` force-added WAYFINDER-STATUS.md and WAYFINDER-HISTORY.jsonl even
// though .gitignore excludes them, TestRepositoryTreeHasNoTemporalDebt then
// rejected them as tracked temporal artifacts, that gate runs inside
// `make preflight-full`, and safe-pr refuses to open a PR when preflight
// fails. Every agent hit it and had to `git rm --cached` the files Wayfinder
// had just committed.
//
// This drives the real binary through start -> start-phase -> complete-phase in
// a repository carrying this repository's own ignore and routing policy, then
// applies the same forbidden-path check the gate above applies.
func TestWayfinderSessionLeavesNoTemporalDebt(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	patterns, err := loadPatterns(filepath.Join(root, ".dear-agent.yml"))
	if err != nil {
		t.Fatal(err)
	}

	binary := filepath.Join(t.TempDir(), "wayfinder")
	build := exec.Command("go", "build", "-o", binary, "./wayfinder/cmd/wayfinder")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build wayfinder: %v\n%s", err, out)
	}

	repo := t.TempDir()
	gittest.Run(t, repo, "init")
	gittest.HardenRepo(t, repo)
	gittest.Run(t, repo, "config", "user.name", "Test User")
	gittest.Run(t, repo, "config", "user.email", "test@example.com")

	// Carry this repository's real policy files so the test measures the
	// shipped rules rather than a paraphrase of them.
	for _, name := range []string{".gitignore", ".dear-agent.yml"} {
		content, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(repo, name), content, 0o600); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	gittest.Run(t, repo, "add", ".gitignore", ".dear-agent.yml")
	gittest.Run(t, repo, "commit", "-m", "Seed policy")

	projectDir := filepath.Join(repo, "wf", "ce-2sgej")
	if err := os.MkdirAll(projectDir, 0o750); err != nil {
		t.Fatalf("create project directory: %v", err)
	}

	telemetry := filepath.Join(t.TempDir(), "telemetry.jsonl")
	for _, args := range [][]string{
		{"-C", projectDir, "session", "start", "ce-2sgej", "--project-type", "bugfix", "--risk-level", "S"},
		{"-C", projectDir, "session", "start-phase", "CHARTER"},
		{"-C", projectDir, "session", "complete-phase", "CHARTER", "--outcome", "success"},
	} {
		cmd := exec.Command(binary, args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "ENGRAM_TELEMETRY_PATH="+telemetry)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("wayfinder %v: %v\n%s", args, err, out)
		}
	}

	tracked, err := gitLines(repo, "ls-files")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range tracked {
		if forbidden(name, patterns) {
			t.Errorf("wayfinder session tracked a temporal artifact: %s "+
				"(preflight-full would fail and safe-pr would refuse the PR)", name)
		}
	}

	// The worktree must also be clean, or the next start-phase refuses with
	// "uncommitted files detected" — the constraint the force-add existed for.
	dirty, err := gitLines(repo, "status", "--porcelain")
	if err != nil {
		t.Fatal(err)
	}
	if len(dirty) != 0 {
		t.Errorf("worktree not clean after the session lifecycle: %v", dirty)
	}
}
