package safepr

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeStatus(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func canonicalTestStatus(project, status, extra string) string {
	currentWaypoint := "CHARTER"
	history := ""
	if status == "completed" {
		currentWaypoint = "RETRO"
		var entries strings.Builder
		for _, waypoint := range []string{"CHARTER", "PROBLEM", "RESEARCH", "DESIGN", "SPEC", "PLAN", "SETUP", "BUILD", "RETRO"} {
			fmt.Fprintf(&entries, "  - {name: %s, status: completed, started_at: 2026-07-20T00:00:00Z, completed_at: 2026-07-20T00:01:00Z}\n", waypoint)
		}
		history = "waypoint_history:\n" + entries.String()
	}
	return fmt.Sprintf(`---
schema_version: "2.0"
project_name: %s
project_type: feature
risk_level: S
current_waypoint: %s
status: %s
created_at: 2026-07-20T00:00:00Z
updated_at: 2026-07-20T00:00:00Z
%s
%s
---
`, project, currentWaypoint, status, extra, history)
}

var inProgressStatus = canonicalTestStatus("safepr-test", "in-progress", "")

func TestLoadSession_InProgress(t *testing.T) {
	dir := t.TempDir()
	writeStatus(t, dir, inProgressStatus)
	s, err := LoadSession(dir)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if s.ID != "safepr-test" {
		t.Errorf("session id = %q", s.ID)
	}
	if s.ProjectPath == "" || !filepath.IsAbs(s.ProjectPath) {
		t.Errorf("project path not absolute: %q", s.ProjectPath)
	}
}

func TestLoadSession_Failures(t *testing.T) {
	missingSchema := strings.Replace(canonicalTestStatus("foo", "in-progress", ""), "schema_version: \"2.0\"\n", "", 1)
	cases := []struct {
		name, content, wantErr string
	}{
		{"missing file", "", "cannot load"},
		{"partial canonical status", "---\nschema_version: \"2.0\"\nproject_name: x\nstatus: in-progress\n---\n", "project_type is required"},
		{"completed session", canonicalTestStatus("x", "completed", "completion_date: 2026-07-20T00:00:00Z"), "wayfinder session start <project-name>"},
		{"no project name", canonicalTestStatus("", "in-progress", ""), "project_name is required"},
		{"abandoned", canonicalTestStatus("foo", "abandoned", ""), "not active"},
		{"blocked", canonicalTestStatus("foo", "blocked", "blocked_reason: waiting"), "not active"},
		{"missing schema", missingSchema, "schema_version is required"},
		{"no frontmatter", "# just markdown\n", "frontmatter"},
		{"unterminated frontmatter", "---\nschema_version: \"2.0\"\nproject_name: x\n", "unterminated"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.content != "" {
				writeStatus(t, dir, tc.content)
			}
			_, err := LoadSession(dir)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("err = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestResolveSessionDir(t *testing.T) {
	t.Setenv("WAYFINDER_PROJECT_DIR", "/env/dir")
	if got, _ := ResolveSessionDir("/flag/dir"); got != "/flag/dir" {
		t.Errorf("flag should win, got %q", got)
	}
	if got, _ := ResolveSessionDir(""); got != "/env/dir" {
		t.Errorf("env fallback, got %q", got)
	}
	t.Setenv("WAYFINDER_PROJECT_DIR", "")
	_, err := ResolveSessionDir("")
	if err == nil {
		t.Fatal("want teaching error, got nil")
	}
	for _, want := range []string{"agm escalate ask", "--session <registered-session>", "ask the current user directly"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("teaching error = %q, want substring %q", err, want)
		}
	}
}

func sess() *Session { return &Session{ID: "abc-123", ProjectPath: "/x/proj"} }

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		req     Request
		wantErr string // empty = valid
	}{
		{"create ok", Request{Verb: "create", Session: sess(),
			GhArgs: []string{"--title", "t", "--body", "b"}}, ""},
		{"close ok", Request{Verb: "close", Session: sess(),
			GhArgs: []string{"123", "--comment", "superseded by #456"}}, ""},
		{"close needs comment", Request{Verb: "close", Session: sess(),
			GhArgs: []string{"123"}}, "--comment"},
		{"close delete-branch refused", Request{Verb: "close", Session: sess(),
			GhArgs: []string{"123", "--comment", "done", "--delete-branch"}}, "irreversible"},
		{"reopen ok", Request{Verb: "reopen", Session: sess(),
			GhArgs: []string{"123", "--comment", "CI event delivery recovery"}}, ""},
		{"reopen needs comment", Request{Verb: "reopen", Session: sess(),
			GhArgs: []string{"123"}}, "--comment"},
		{"merge refused", Request{Verb: "merge", Session: sess()}, "only supports"},
		{"web refused", Request{Verb: "create", Session: sess(),
			GhArgs: []string{"--title", "t", "--web"}}, "browser"},
		{"fill refused", Request{Verb: "create", Session: sess(),
			GhArgs: []string{"--fill", "--title", "t"}}, "stamped"},
		{"body-file refused", Request{Verb: "create", Session: sess(),
			GhArgs: []string{"--title", "t", "-F", "b.md"}}, "--body instead"},
		{"body-file= refused", Request{Verb: "create", Session: sess(),
			GhArgs: []string{"--title", "t", "--body-file=b.md"}}, "--body instead"},
		{"title required", Request{Verb: "create", Session: sess(),
			GhArgs: []string{"--body", "b"}}, "--title"},
		{"no session", Request{Verb: "create",
			GhArgs: []string{"--title", "t"}}, "wrapper bug"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("err = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func TestTrailer(t *testing.T) {
	r := Request{Verb: "create", Session: sess()}
	tr := r.Trailer()
	if !strings.Contains(tr, "Wayfinder-Session: abc-123") ||
		!strings.Contains(tr, "Wayfinder-Project: proj") {
		t.Errorf("trailer = %q", tr)
	}
}

func TestStampedArgs(t *testing.T) {
	t.Run("create stamps existing body", func(t *testing.T) {
		r := Request{Verb: "create", Session: sess(),
			GhArgs: []string{"--title", "t", "--body", "hello"}}
		got := r.StampedArgs()
		if got[0] != "pr" || got[1] != "create" {
			t.Fatalf("argv prefix = %v", got[:2])
		}
		joined := strings.Join(got, "\x00")
		if !strings.Contains(joined, "hello\n\n---\nWayfinder-Session: abc-123") {
			t.Errorf("body not stamped: %v", got)
		}
	})
	t.Run("create stamps inline body form", func(t *testing.T) {
		r := Request{Verb: "create", Session: sess(),
			GhArgs: []string{"--title", "t", "--body=hello"}}
		joined := strings.Join(r.StampedArgs(), "\x00")
		if !strings.Contains(joined, "--body=hello\n\n---\nWayfinder-Session") {
			t.Errorf("inline body not stamped: %q", joined)
		}
	})
	t.Run("create adds body when absent", func(t *testing.T) {
		r := Request{Verb: "create", Session: sess(), GhArgs: []string{"--title", "t"}}
		got := r.StampedArgs()
		joined := strings.Join(got, "\x00")
		if !strings.Contains(joined, "--body\x00---\nWayfinder-Session: abc-123") {
			t.Errorf("body flag not appended: %v", got)
		}
	})
	t.Run("close stamps existing comment", func(t *testing.T) {
		r := Request{Verb: "close", Session: sess(),
			GhArgs: []string{"42", "--comment", "superseded"}}
		joined := strings.Join(r.StampedArgs(), "\x00")
		if !strings.Contains(joined, "superseded\n\n---\nWayfinder-Session: abc-123") {
			t.Errorf("comment not stamped: %q", joined)
		}
	})
	t.Run("close stamps comment when absent", func(t *testing.T) {
		r := Request{Verb: "close", Session: sess(), GhArgs: []string{"42"}}
		joined := strings.Join(r.StampedArgs(), "\x00")
		if !strings.Contains(joined, "--comment\x00---\nWayfinder-Session: abc-123") {
			t.Errorf("comment not appended: %q", joined)
		}
	})
	t.Run("reopen stamps existing comment", func(t *testing.T) {
		r := Request{Verb: "reopen", Session: sess(),
			GhArgs: []string{"42", "--comment", "retry CI delivery"}}
		got := r.StampedArgs()
		joined := strings.Join(got, "\x00")
		if got[0] != "pr" || got[1] != "reopen" {
			t.Fatalf("argv prefix = %v", got[:2])
		}
		if !strings.Contains(joined, "retry CI delivery\n\n---\nWayfinder-Session: abc-123") {
			t.Errorf("comment not stamped: %q", joined)
		}
	})
}

var beadStatus = canonicalTestStatus("safepr-bead-test", "in-progress", `beads:
  - ce-5vje
  - ce-9999
`)

func TestLoadSession_Bead(t *testing.T) {
	t.Run("first bead populates BeadID", func(t *testing.T) {
		dir := t.TempDir()
		writeStatus(t, dir, beadStatus)
		s, err := LoadSession(dir)
		if err != nil {
			t.Fatalf("LoadSession: %v", err)
		}
		if s.BeadID != "ce-5vje" {
			t.Errorf("BeadID = %q, want ce-5vje (first of beads list)", s.BeadID)
		}
	})
	t.Run("no beads leaves BeadID empty", func(t *testing.T) {
		dir := t.TempDir()
		writeStatus(t, dir, inProgressStatus)
		s, err := LoadSession(dir)
		if err != nil {
			t.Fatalf("LoadSession: %v", err)
		}
		if s.BeadID != "" {
			t.Errorf("BeadID = %q, want empty", s.BeadID)
		}
	})
}

const v2InProgressStatus = `---
schema_version: "2.0"
project_name: my-feature
project_type: feature
risk_level: S
current_waypoint: CHARTER
status: in-progress
created_at: 2026-06-24T00:00:00Z
updated_at: 2026-06-24T00:00:00Z
beads:
  - ce-abcd
---
`

func TestLoadSession_V2(t *testing.T) {
	dir := t.TempDir()
	writeStatus(t, dir, v2InProgressStatus)
	s, err := LoadSession(dir)
	if err != nil {
		t.Fatalf("LoadSession V2: %v", err)
	}
	if s.ID != "my-feature" {
		t.Errorf("session id = %q, want my-feature (project_name fallback)", s.ID)
	}
	if s.BeadID != "ce-abcd" {
		t.Errorf("BeadID = %q, want ce-abcd", s.BeadID)
	}
	if s.ProjectPath == "" || !filepath.IsAbs(s.ProjectPath) {
		t.Errorf("project path not absolute: %q", s.ProjectPath)
	}
}

func TestLoadSession_V2Planning(t *testing.T) {
	dir := t.TempDir()
	writeStatus(t, dir, strings.Replace(v2InProgressStatus, "status: in-progress", "status: planning", 1))
	s, err := LoadSession(dir)
	if err != nil {
		t.Fatalf("LoadSession V2 planning: %v", err)
	}
	if s.ID != "my-feature" {
		t.Errorf("session id = %q, want my-feature", s.ID)
	}
}

func TestIsActiveStatus(t *testing.T) {
	cases := map[string]bool{
		"planning": true, "in-progress": true, "in_progress": false,
		"blocked": false, "completed": false, "abandoned": false, "": false,
	}
	for status, want := range cases {
		if got := isActiveStatus(status); got != want {
			t.Errorf("isActiveStatus(%q) = %v, want %v", status, got, want)
		}
	}
}

func TestStampedArgs_Bead(t *testing.T) {
	beadSess := func() *Session {
		return &Session{ID: "abc-123", ProjectPath: "/x/proj", BeadID: "ce-5vje"}
	}
	t.Run("create folds Closes above trailer", func(t *testing.T) {
		r := Request{Verb: "create", Session: beadSess(),
			GhArgs: []string{"--title", "t", "--body", "hello"}}
		joined := strings.Join(r.StampedArgs(), "\x00")
		if !strings.Contains(joined, "hello\n\nCloses ce-5vje\n\n---\nWayfinder-Session: abc-123") {
			t.Errorf("Closes not folded above trailer: %q", joined)
		}
	})
	t.Run("create without body still adds Closes", func(t *testing.T) {
		r := Request{Verb: "create", Session: beadSess(), GhArgs: []string{"--title", "t"}}
		joined := strings.Join(r.StampedArgs(), "\x00")
		if !strings.Contains(joined, "--body\x00Closes ce-5vje\n\n---\nWayfinder-Session") {
			t.Errorf("Closes not added when body absent: %q", joined)
		}
	})
	t.Run("no duplicate when caller already referenced bead", func(t *testing.T) {
		r := Request{Verb: "create", Session: beadSess(),
			GhArgs: []string{"--title", "t", "--body", "fixes ce-5vje now"}}
		joined := strings.Join(r.StampedArgs(), "\x00")
		if strings.Contains(joined, "Closes ce-5vje") {
			t.Errorf("should not add Closes when bead already referenced: %q", joined)
		}
	})
	t.Run("close never gets Closes line", func(t *testing.T) {
		r := Request{Verb: "close", Session: beadSess(),
			GhArgs: []string{"42", "--comment", "superseded"}}
		joined := strings.Join(r.StampedArgs(), "\x00")
		if strings.Contains(joined, "Closes ce-5vje") {
			t.Errorf("close should not stamp Closes: %q", joined)
		}
	})
	t.Run("no bead means no Closes line", func(t *testing.T) {
		r := Request{Verb: "create", Session: sess(),
			GhArgs: []string{"--title", "t", "--body", "hello"}}
		joined := strings.Join(r.StampedArgs(), "\x00")
		if strings.Contains(joined, "Closes") {
			t.Errorf("no bead should mean no Closes line: %q", joined)
		}
	})
}

func TestAppendAudit(t *testing.T) {
	home := t.TempDir()
	rec := AuditRecord{Time: "2026-06-12T00:00:00Z", Verb: "create",
		SessionID: "abc", PRURL: "https://github.com/x/y/pull/1", ExitCode: 0}
	if err := AppendAudit(home, rec); err != nil {
		t.Fatalf("AppendAudit: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(home, ".local", "state", "dear-agent", "safe-pr.log"))
	if err != nil {
		t.Fatal(err)
	}
	var got AuditRecord
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(raw))), &got); err != nil {
		t.Fatalf("audit line is not valid JSON: %v", err)
	}
	if got.SessionID != "abc" || got.PRURL == "" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}
