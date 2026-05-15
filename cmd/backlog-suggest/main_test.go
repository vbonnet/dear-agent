package main

import (
	"os"
	"path/filepath"
	"strings"
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
	if got := parseFiles(""); len(got) != 2 {
		t.Errorf("default files = %v, want 2 entries", got)
	}
	got := parseFiles(" a.md , , b.md ")
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

func TestCmdSuggestEmitsVROOM(t *testing.T) {
	p := writeBacklog(t)
	out := filepath.Join(t.TempDir(), "nested", "vroom.jsonl")
	code := cmdSuggest([]string{
		"--files", p, "--emit-vroom", "--vroom-out", out, "--worker", "claude-code",
	})
	if code != 0 {
		t.Fatalf("cmdSuggest exit = %d, want 0", code)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read vroom out: %v", err)
	}
	line := strings.TrimSpace(string(b))
	if !strings.Contains(line, `"topic":"vroom.decision.dispatched"`) {
		t.Errorf("jsonl missing dispatched topic: %s", line)
	}
	if !strings.Contains(line, `"task_id":"1.1"`) {
		t.Errorf("jsonl should dispatch top pick 1.1: %s", line)
	}
	if !strings.Contains(line, `"worker":"claude-code"`) {
		t.Errorf("jsonl missing worker: %s", line)
	}
}

func TestJSONLPublisherAppends(t *testing.T) {
	out := filepath.Join(t.TempDir(), "p.jsonl")
	pub, err := newJSONLPublisher(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := pub.Publish("t.one", map[string]interface{}{"a": 1}); err != nil {
		t.Fatal(err)
	}
	if err := pub.Publish("t.two", map[string]interface{}{"b": 2}); err != nil {
		t.Fatal(err)
	}
	if err := pub.Close(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 jsonl lines, got %d: %q", len(lines), string(b))
	}
}
