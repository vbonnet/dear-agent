package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/backlog"
)

const sampleMD = `## Phase 0
| # | Title | Files | Dep | Size | Status |
|---|---|---|---|---|---|
| 0.1 | Base | a | — | S | done |

## Phase 1
| # | Title | Files | Dep | Size | Status |
|---|---|---|---|---|---|
| 1.1 | Ready work | b | 0.1 | S | pending |
| 1.2 | Blocked work | c | 1.1 | M | pending |
`

func writeBacklog(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "BACKLOG.md")
	if err := os.WriteFile(p, []byte(sampleMD), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParseFiles(t *testing.T) {
	if _, err := parseFiles(""); err == nil {
		t.Fatal("empty --files accepted")
	}
	got, err := parseFiles(" a.md , , b.md ")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "a.md" || got[1] != "b.md" {
		t.Errorf("parseFiles = %v, want [a.md b.md]", got)
	}
}

func TestParseEffortFlag(t *testing.T) {
	cases := map[string]backlog.Effort{
		"S": backlog.EffortSmall, "m": backlog.EffortMedium,
		"L": backlog.EffortLarge, "": backlog.EffortUnknown, "xl": backlog.EffortUnknown,
	}
	for in, want := range cases {
		if got := parseEffortFlag(in); got != want {
			t.Errorf("parseEffortFlag(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestRunUsage(t *testing.T) {
	old := os.Args
	defer func() { os.Args = old }()
	os.Args = []string{"backlog-suggest"}
	if got := run(); got != 2 {
		t.Errorf("run() with no args = %d, want 2", got)
	}
	os.Args = []string{"backlog-suggest", "bogus"}
	if got := run(); got != 2 {
		t.Errorf("run() unknown subcommand = %d, want 2", got)
	}
}

func TestCommandsRequireExplicitFiles(t *testing.T) {
	if got := cmdList(nil); got != 2 {
		t.Errorf("cmdList without --files = %d, want 2", got)
	}
	if got := cmdSuggest(nil); got != 2 {
		t.Errorf("cmdSuggest without --files = %d, want 2", got)
	}
	missing := filepath.Join(t.TempDir(), "missing.md")
	if got := cmdList([]string{"--files", missing}); got != 1 {
		t.Errorf("cmdList with missing explicit file = %d, want 1", got)
	}
	if got := cmdSuggest([]string{"--files", missing}); got != 1 {
		t.Errorf("cmdSuggest with missing explicit file = %d, want 1", got)
	}
}

func TestCmdList(t *testing.T) {
	p := writeBacklog(t)
	if got := cmdList([]string{"--files", p}); got != 0 {
		t.Errorf("cmdList exit = %d, want 0", got)
	}
	if got := cmdList([]string{"--files", p, "--json"}); got != 0 {
		t.Errorf("cmdList --json exit = %d, want 0", got)
	}
}

func TestCmdSuggest(t *testing.T) {
	p := writeBacklog(t)
	if got := cmdSuggest([]string{"--files", p}); got != 0 {
		t.Errorf("cmdSuggest exit = %d, want 0", got)
	}
	if got := cmdSuggest([]string{"--files", p, "--json"}); got != 0 {
		t.Errorf("cmdSuggest --json exit = %d, want 0", got)
	}
}
