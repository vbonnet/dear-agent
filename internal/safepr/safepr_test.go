package safepr

import (
	"encoding/json"
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

const inProgressStatus = `---
schema_version: "2.0"
session_id: 87a5378c-2398-412c-af48-094287b11b79
status: in_progress
---

# Wayfinder Session
`

func TestLoadSession_InProgress(t *testing.T) {
	dir := t.TempDir()
	writeStatus(t, dir, inProgressStatus)
	s, err := LoadSession(dir)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if s.ID != "87a5378c-2398-412c-af48-094287b11b79" {
		t.Errorf("session id = %q", s.ID)
	}
	if s.ProjectPath == "" || !filepath.IsAbs(s.ProjectPath) {
		t.Errorf("project path not absolute: %q", s.ProjectPath)
	}
}

func TestLoadSession_Failures(t *testing.T) {
	cases := []struct {
		name, content, wantErr string
	}{
		{"missing file", "", "cannot read"},
		{"completed session", "---\nsession_id: x\nstatus: completed\n---\n", "not in_progress"},
		{"no session id", "---\nstatus: in_progress\n---\n", "no session_id"},
		{"no frontmatter", "# just markdown\n", "frontmatter"},
		{"unterminated frontmatter", "---\nsession_id: x\n", "unterminated"},
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
	if err == nil || !strings.Contains(err.Error(), "--emergency") {
		t.Errorf("want teaching error mentioning the emergency hatch, got %v", err)
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
		{"close ok", Request{Verb: "close", Session: sess(), GhArgs: []string{"123"}}, ""},
		{"merge refused", Request{Verb: "merge", Session: sess()}, "only supports"},
		{"reopen refused", Request{Verb: "reopen", Session: sess()}, "only supports"},
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
		{"emergency needs reason", Request{Verb: "create", Emergency: true,
			GhArgs: []string{"--title", "t"}}, "--reason"},
		{"emergency with reason ok", Request{Verb: "create", Emergency: true, Reason: "hotfix",
			GhArgs: []string{"--title", "t"}}, ""},
		{"no session no emergency", Request{Verb: "create",
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
	e := Request{Verb: "create", Emergency: true, Reason: "prod down"}
	if !strings.Contains(e.Trailer(), "EMERGENCY (no wayfinder session): prod down") {
		t.Errorf("emergency trailer = %q", e.Trailer())
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
	t.Run("close stamps comment", func(t *testing.T) {
		r := Request{Verb: "close", Session: sess(), GhArgs: []string{"42"}}
		joined := strings.Join(r.StampedArgs(), "\x00")
		if !strings.Contains(joined, "--comment\x00---\nWayfinder-Session: abc-123") {
			t.Errorf("comment not stamped: %q", joined)
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
