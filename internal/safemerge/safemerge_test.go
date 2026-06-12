package safemerge

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestValidate(t *testing.T) {
	sess := &Session{ID: "abc", ProjectPath: "/p/foo"}
	cases := []struct {
		name    string
		req     Request
		wantErr string
	}{
		{"ok squash", Request{Repo: "/r", Branch: "b", Mode: ModeSquash, Session: sess}, ""},
		{"ok no-ff", Request{Repo: "/r", Branch: "b", Mode: ModeNoFF, Session: sess}, ""},
		{"missing repo", Request{Branch: "b", Mode: ModeSquash, Session: sess}, "--repo is required"},
		{"missing branch", Request{Repo: "/r", Mode: ModeSquash, Session: sess}, "--branch is required"},
		{"branch looks like flag", Request{Repo: "/r", Branch: "--force", Mode: ModeSquash, Session: sess}, "looks like a flag"},
		{"bad mode", Request{Repo: "/r", Branch: "b", Mode: "rebase", Session: sess}, "not valid"},
		{"emergency no reason", Request{Repo: "/r", Branch: "b", Mode: ModeSquash, Emergency: true}, "requires --reason"},
		{"emergency with reason", Request{Repo: "/r", Branch: "b", Mode: ModeSquash, Emergency: true, Reason: "prod down"}, ""},
		{"no session no emergency", Request{Repo: "/r", Branch: "b", Mode: ModeSquash}, "wrapper bug"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("want nil, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestTrailerAndCommitMessage(t *testing.T) {
	// Emergency trailer.
	r := &Request{Emergency: true, Reason: "  prod down  "}
	if got := r.Trailer(); !strings.Contains(got, "EMERGENCY") || !strings.Contains(got, "prod down") {
		t.Fatalf("emergency trailer = %q", got)
	}

	// Session with id.
	r = &Request{Session: &Session{ID: "deadbeef", ProjectPath: "/x/projects/foo"}}
	tr := r.Trailer()
	if !strings.Contains(tr, "Wayfinder-Session: deadbeef") || !strings.Contains(tr, "Wayfinder-Project: foo") {
		t.Fatalf("session trailer = %q", tr)
	}

	// Session without id falls back to the honest ce-6kl1 note.
	r = &Request{Session: &Session{ID: "", ProjectPath: "/x/foo"}}
	if !strings.Contains(r.Trailer(), "ce-6kl1") {
		t.Fatalf("missing-id trailer should cite ce-6kl1, got %q", r.Trailer())
	}

	// Derived default headline when none given.
	r = &Request{Branch: "feat/x", Session: &Session{ID: "id"}}
	msg := r.CommitMessage("main")
	if !strings.HasPrefix(msg, "Merge branch 'feat/x' into main") {
		t.Fatalf("default headline = %q", msg)
	}
	// Explicit headline preserved.
	r.Message = "feat: do the thing"
	if !strings.HasPrefix(r.CommitMessage("main"), "feat: do the thing\n\n") {
		t.Fatalf("explicit headline not preserved: %q", r.CommitMessage("main"))
	}
}

func TestFrontmatter(t *testing.T) {
	good := "---\nstatus: in_progress\nsession_id: abc\n---\nbody"
	fm, err := frontmatter(good)
	if err != nil || !strings.Contains(fm, "session_id: abc") {
		t.Fatalf("frontmatter(good) = %q, %v", fm, err)
	}
	if _, err := frontmatter("no frontmatter"); err == nil {
		t.Fatal("expected error for missing frontmatter")
	}
	if _, err := frontmatter("---\nunterminated"); err == nil {
		t.Fatal("expected error for unterminated frontmatter")
	}
}

func TestLoadSession(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// Missing file.
	if _, err := LoadSession(dir); err == nil {
		t.Fatal("expected error for missing status file")
	}

	// Not in_progress.
	write("WAYFINDER-STATUS.md", "---\nstatus: complete\nsession_id: x\n---\n")
	if _, err := LoadSession(dir); err == nil || !strings.Contains(err.Error(), "not in_progress") {
		t.Fatalf("want not in_progress error, got %v", err)
	}

	// In progress with id.
	write("WAYFINDER-STATUS.md", "---\nstatus: in_progress\nsession_id: sid-1\n---\n")
	s, err := LoadSession(dir)
	if err != nil || s.ID != "sid-1" {
		t.Fatalf("LoadSession = %+v, %v", s, err)
	}

	// In progress, id dropped (ce-6kl1) — recovered from a deliverable.
	// Real wayfinder session ids are hex/UUID-shaped (the recovery regex only
	// matches [0-9a-fA-F-]).
	write("WAYFINDER-STATUS.md", "---\nstatus: in_progress\n---\n")
	write("D4-spec.md", "---\nwayfinder_session_id: \"abc123-def456\"\n---\nspec")
	s, err = LoadSession(dir)
	if err != nil || s.ID != "abc123-def456" {
		t.Fatalf("recovered LoadSession = %+v, %v", s, err)
	}
}

func TestResolveSessionDir(t *testing.T) {
	if d, _ := ResolveSessionDir("/flag/dir"); d != "/flag/dir" {
		t.Fatalf("flag should win, got %q", d)
	}
	t.Setenv("WAYFINDER_PROJECT_DIR", "/env/dir")
	if d, _ := ResolveSessionDir(""); d != "/env/dir" {
		t.Fatalf("env fallback, got %q", d)
	}
	t.Setenv("WAYFINDER_PROJECT_DIR", "")
	if _, err := ResolveSessionDir(""); err == nil {
		t.Fatal("expected error when neither flag nor env set")
	}
}

// fakeGit records git invocations and returns scripted responses keyed by the
// first non -C argument.
type fakeGit struct {
	responses map[string]gitResp
	calls     []string
}

type gitResp struct {
	out string
	err error
}

func (f *fakeGit) run(_ context.Context, _ string, args ...string) (string, error) {
	key := strings.Join(args, " ")
	f.calls = append(f.calls, key)
	// Match by the most specific scripted prefix.
	for k, v := range f.responses {
		if strings.HasPrefix(key, k) {
			return v.out, v.err
		}
	}
	return "", nil
}

func cleanRepoGit() *fakeGit {
	return &fakeGit{responses: map[string]gitResp{
		"rev-parse --is-inside-work-tree":               {out: "true"},
		"status --porcelain":                            {out: ""},
		"symbolic-ref --short refs/remotes/origin/HEAD": {out: "origin/main"},
		"rev-parse --abbrev-ref HEAD":                   {out: "main"},
	}}
}

func TestRunSquashSuccess(t *testing.T) {
	fg := cleanRepoGit()
	fg.responses["merge --squash"] = gitResp{out: ""}
	fg.responses["commit -m"] = gitResp{out: "[main abc] landed"}
	m := &Merger{git: fg.run, timeout: time.Second}
	res, err := m.Run(&Request{Repo: "/r", Branch: "feat/x", Mode: ModeSquash, Session: &Session{ID: "s"}})
	if err != nil {
		t.Fatalf("Run err = %v", err)
	}
	if res.DefaultBranch != "main" || res.RolledBack {
		t.Fatalf("res = %+v", res)
	}
	if !contains(fg.calls, "merge --squash feat/x") || !hasPrefixCall(fg.calls, "commit -m") {
		t.Fatalf("expected squash+commit, calls=%v", fg.calls)
	}
}

func TestRunSquashConflictRollsBack(t *testing.T) {
	fg := cleanRepoGit()
	fg.responses["merge --squash"] = gitResp{out: "CONFLICT", err: errString("exit 1")}
	m := &Merger{git: fg.run, timeout: time.Second}
	res, err := m.Run(&Request{Repo: "/r", Branch: "feat/x", Mode: ModeSquash, Session: &Session{ID: "s"}})
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("want rollback error, got %v", err)
	}
	if res == nil || !res.RolledBack {
		t.Fatalf("expected RolledBack, res=%+v", res)
	}
	if !contains(fg.calls, "reset --hard HEAD") {
		t.Fatalf("expected reset --hard rollback, calls=%v", fg.calls)
	}
}

func TestRunRefusesDirtyTree(t *testing.T) {
	fg := cleanRepoGit()
	fg.responses["status --porcelain"] = gitResp{out: " M foo.go"}
	m := &Merger{git: fg.run, timeout: time.Second}
	_, err := m.Run(&Request{Repo: "/r", Branch: "b", Mode: ModeSquash, Session: &Session{ID: "s"}})
	if err == nil || !strings.Contains(err.Error(), "not clean") {
		t.Fatalf("want dirty-tree refusal, got %v", err)
	}
}

func TestRunRefusesOffDefaultBranch(t *testing.T) {
	fg := cleanRepoGit()
	fg.responses["rev-parse --abbrev-ref HEAD"] = gitResp{out: "some-feature"}
	m := &Merger{git: fg.run, timeout: time.Second}
	_, err := m.Run(&Request{Repo: "/r", Branch: "b", Mode: ModeSquash, Session: &Session{ID: "s"}})
	if err == nil || !strings.Contains(err.Error(), "default branch") {
		t.Fatalf("want off-branch refusal, got %v", err)
	}
}

func TestRunNoFFConflictAborts(t *testing.T) {
	fg := cleanRepoGit()
	fg.responses["merge --no-ff"] = gitResp{out: "CONFLICT", err: errString("exit 1")}
	m := &Merger{git: fg.run, timeout: time.Second}
	_, err := m.Run(&Request{Repo: "/r", Branch: "b", Mode: ModeNoFF, Session: &Session{ID: "s"}})
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("want rollback error, got %v", err)
	}
	if !contains(fg.calls, "merge --abort") {
		t.Fatalf("expected merge --abort, calls=%v", fg.calls)
	}
}

func TestAppendAudit(t *testing.T) {
	home := t.TempDir()
	rec := AuditRecord{Time: "t", Repo: "/r", Branch: "b", Mode: "squash", SessionID: "s", ExitCode: 0}
	if err := AppendAudit(home, rec); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".local", "state", "dear-agent", "safe-merge.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"branch":"b"`) {
		t.Fatalf("audit line = %s", data)
	}
}

// helpers

type errString string

func (e errString) Error() string { return string(e) }

func contains(s []string, want string) bool {
	return slices.Contains(s, want)
}

func hasPrefixCall(s []string, prefix string) bool {
	for _, v := range s {
		if strings.HasPrefix(v, prefix) {
			return true
		}
	}
	return false
}
