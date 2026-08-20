package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixture writes the four evidence files a plan run reads and returns the
// directory holding them.
type fixture struct {
	dir           string
	inventory     string
	state         string
	canonical     string
	rulesetsDir   string
	rulesetsFiles map[string]string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	f := &fixture{
		dir:         dir,
		inventory:   `{"active":["dear-agent"],"archived":[]}`,
		state:       `{}`,
		canonical:   `{"name":"main-zero-bypass"}`,
		rulesetsDir: filepath.Join(dir, "rulesets"),
		rulesetsFiles: map[string]string{
			"dear-agent": `[[{"id":18061003,"name":"main-zero-bypass"}]]`,
		},
	}
	return f
}

// write materializes the fixture and returns the argument list for `plan`.
func (f *fixture) write(t *testing.T) []string {
	t.Helper()
	if err := os.MkdirAll(f.rulesetsDir, 0o755); err != nil {
		t.Fatalf("mkdir rulesets: %v", err)
	}
	paths := map[string]string{
		filepath.Join(f.dir, "inventory.json"): f.inventory,
		filepath.Join(f.dir, "state.json"):     f.state,
		filepath.Join(f.dir, "canonical.json"): f.canonical,
	}
	for repo, body := range f.rulesetsFiles {
		paths[filepath.Join(f.rulesetsDir, repo+".json")] = body
	}
	for path, body := range paths {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return []string{
		"plan",
		"--inventory", filepath.Join(f.dir, "inventory.json"),
		"--state", filepath.Join(f.dir, "state.json"),
		"--canonical-ruleset", filepath.Join(f.dir, "canonical.json"),
		"--rulesets-dir", f.rulesetsDir,
	}
}

func TestPlanEmitsTabSeparatedRecords(t *testing.T) {
	f := newFixture(t)
	args := f.write(t)

	var out bytes.Buffer
	if err := run(args, &out); err != nil {
		t.Fatalf("run: %v", err)
	}

	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 records, got %d: %q", len(lines), out.String())
	}
	for _, line := range lines {
		if fields := strings.Split(line, "\t"); len(fields) != 4 {
			t.Fatalf("record %q has %d tab-separated fields, want 4", line, len(fields))
		}
	}
	if !strings.Contains(out.String(), "dear-agent:18061003") {
		t.Fatalf("plan does not bind the ruleset to its provider id: %q", out.String())
	}
}

func TestPlanJSONMatchesTheRecordStream(t *testing.T) {
	f := newFixture(t)
	args := append(f.write(t), "--json")

	var out bytes.Buffer
	if err := run(args, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	var steps []struct {
		Verb     string `json:"verb"`
		Address  string `json:"address"`
		ImportID string `json:"import_id"`
	}
	if err := json.Unmarshal(out.Bytes(), &steps); err != nil {
		t.Fatalf("plan --json is not valid JSON: %v\n%s", err, out.String())
	}
	if len(steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(steps))
	}
}

func TestPlanFailsClosedOnUnusableEvidence(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*fixture)
		wantErr string
	}{
		{
			// A missing listing is evidence the collection step failed, not
			// evidence the repository has no ruleset.
			name:    "a missing ruleset listing is not an absence",
			mutate:  func(f *fixture) { f.rulesetsFiles = map[string]string{} },
			wantErr: "read ruleset listing",
		},
		{
			name:    "a canonical document without a name is rejected",
			mutate:  func(f *fixture) { f.canonical = `{}` },
			wantErr: "no name",
		},
		{
			name:    "an inventory omitting dear-agent is rejected",
			mutate:  func(f *fixture) { f.inventory = `{"active":["other"],"archived":[]}` },
			wantErr: "omits dear-agent",
		},
		{
			name:    "unreadable state is rejected rather than treated as empty",
			mutate:  func(f *fixture) { f.state = `not json` },
			wantErr: "unreadable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			tt.mutate(f)
			args := f.write(t)

			var out bytes.Buffer
			err := run(args, &out)
			if err == nil {
				t.Fatalf("run unexpectedly succeeded: %q", out.String())
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err, tt.wantErr)
			}
			// A caller must never be handed a partial plan to execute.
			if out.Len() != 0 {
				t.Fatalf("a failed plan emitted records: %q", out.String())
			}
		})
	}
}

// TestPlanTreatsAnAbsentStateFileAsEmpty covers the first run, where no state
// exists yet.
func TestPlanTreatsAnAbsentStateFileAsEmpty(t *testing.T) {
	f := newFixture(t)
	args := f.write(t)
	for i, a := range args {
		if a == "--state" {
			args[i+1] = filepath.Join(f.dir, "does-not-exist.json")
		}
	}

	var out bytes.Buffer
	if err := run(args, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.Contains(out.String(), "skip\t") {
		t.Fatalf("an absent state file must produce no skips: %q", out.String())
	}
}

func TestReposPrintsTheActiveInventory(t *testing.T) {
	f := newFixture(t)
	f.inventory = `{"active":["dear-agent","engram-research"],"archived":["old"]}`
	f.write(t)

	var out bytes.Buffer
	if err := run([]string{"repos", "--inventory", filepath.Join(f.dir, "inventory.json")}, &out); err != nil {
		t.Fatalf("run: %v", err)
	}
	got := strings.Fields(out.String())
	if len(got) != 2 || got[0] != "dear-agent" || got[1] != "engram-research" {
		t.Fatalf("repos printed %v, want the two active repositories in sorted order", got)
	}
}

func TestClassifySeparatesAbsenceFromFailure(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		wantErr bool
	}{
		{name: "not found is an absence", output: "Error: Not Found"},
		{name: "404 is an absence", output: "GET https://api.github.com/repos/x: 404"},
		{name: "no associated configuration is an absence", output: "no configuration associated"},
		{name: "forbidden is a real failure", output: "Error: 403 Forbidden", wantErr: true},
		{name: "rate limiting is a real failure", output: "Error: rate limit exceeded", wantErr: true},
		{name: "empty output is a real failure", output: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "provider.log")
			if err := os.WriteFile(path, []byte(tt.output), 0o600); err != nil {
				t.Fatalf("write provider output: %v", err)
			}

			var out bytes.Buffer
			err := run([]string{"classify", "--provider-output", path}, &out)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("classify unexpectedly accepted %q as an absence", tt.output)
				}
				return
			}
			if err != nil {
				t.Fatalf("classify rejected a recognized absence %q: %v", tt.output, err)
			}
			if !strings.Contains(out.String(), "absent") {
				t.Fatalf("classify did not report absence: %q", out.String())
			}
		})
	}
}

func TestRunRejectsUnusableInvocations(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "no subcommand", args: nil},
		{name: "unknown subcommand", args: []string{"apply"}},
		{name: "plan without an inventory", args: []string{"plan", "--canonical-ruleset", "x", "--rulesets-dir", "y"}},
		{name: "plan without a canonical ruleset", args: []string{"plan", "--inventory", "x"}},
		{name: "repos without an inventory", args: []string{"repos"}},
		{name: "classify without provider output", args: []string{"classify"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := run(tt.args, &out); err == nil {
				t.Fatalf("run unexpectedly succeeded: %q", out.String())
			}
		})
	}
}
