package importer

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/dolt"
)

func TestInferWorkspaceFromProject(t *testing.T) {
	tmpDir := t.TempDir()

	// A git-rooted repo: ~/work/myrepo/.git, nested package below it.
	gitRepo := filepath.Join(tmpDir, "work", "myrepo")
	gitPkg := filepath.Join(gitRepo, "pkg", "deep")
	if err := os.MkdirAll(filepath.Join(gitRepo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(gitPkg, 0o755); err != nil {
		t.Fatal(err)
	}

	// A WORKSPACE.yaml-rooted workspace.
	wsRoot := filepath.Join(tmpDir, "ws", "oss")
	if err := os.MkdirAll(wsRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wsRoot, "WORKSPACE.yaml"), []byte("name: oss\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A plain directory with no markers anywhere up to tmpDir.
	plain := filepath.Join(tmpDir, "loose", "dir")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		project string
		want    string
	}{
		{"git root itself", gitRepo, "myrepo"},
		{"nested under git root", gitPkg, "myrepo"},
		{"workspace.yaml root", wsRoot, "oss"},
		{"no marker", plain, ""},
		{"empty path", "", ""},
		{"relative path", "relative/dir", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InferWorkspaceFromProject(tt.project); got != tt.want {
				t.Errorf("InferWorkspaceFromProject(%q) = %q, want %q", tt.project, got, tt.want)
			}
		})
	}
}

func TestInferWorkspaceFromProject_TildeExpansion(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)

	repo := filepath.Join(tmpHome, "src", "dear-agent")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	if got := InferWorkspaceFromProject("~/src/dear-agent"); got != "dear-agent" {
		t.Errorf("InferWorkspaceFromProject(~/src/dear-agent) = %q, want %q", got, "dear-agent")
	}
}

func TestDeriveSessionName(t *testing.T) {
	tests := []struct {
		name        string
		projectPath string
		uuid        string
		want        string
	}{
		{"project basename + short uuid", "/home/u/src/myrepo", "abcdef12-3456-7890-abcd-ef1234567890", "myrepo-abcdef12"},
		{"trailing slash trimmed", "/home/u/src/myrepo/", "abcdef12-rest", "myrepo-abcdef12"},
		{"empty project falls back to uuid", "", "abcdef12-rest", "session-abcdef12"},
		{"short uuid not truncated", "/x/repo", "short", "repo-short"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveSessionName(tt.projectPath, tt.uuid); got != tt.want {
				t.Errorf("deriveSessionName(%q, %q) = %q, want %q", tt.projectPath, tt.uuid, got, tt.want)
			}
		})
	}
}

// setupConversation writes a fake Claude conversation + history entry under a
// temp HOME and returns the project directory the manifest should point at.
// The project dir is created with a .git marker so workspace inference resolves
// to its basename.
func setupConversation(t *testing.T, uuid, projectName string) (home, project string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)

	// Real project dir with a git marker (drives workspace inference).
	project = filepath.Join(home, "repos", projectName)
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Conversation file under ~/.claude/projects/<encoded>/<uuid>.jsonl
	projectsDir := filepath.Join(home, ".claude", "projects", "encoded-"+projectName)
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectsDir, uuid+".jsonl"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	// history.jsonl pointing the session at the real project dir.
	historyFile := filepath.Join(home, ".claude", "history.jsonl")
	line := fmt.Sprintf(`{"display":"x","timestamp":1733500000000,"project":"%s","sessionId":"%s"}`+"\n", project, uuid)
	if err := os.WriteFile(historyFile, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	return home, project
}

func TestRegisterSession_Idempotent(t *testing.T) {
	adapter := dolt.GetTestAdapter(t)
	if adapter == nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	uuid := "idem-1234-5678-90ab-cdef12345678"
	_, project := setupConversation(t, uuid, "alpha")

	// First registration creates a new session.
	first, err := RegisterSession(uuid, "", "", "", "", adapter)
	if err != nil {
		t.Fatalf("first register failed: %v", err)
	}
	if first.AlreadyTracked {
		t.Error("first register should not report AlreadyTracked")
	}
	if first.SessionID == "" {
		t.Fatal("first register returned empty session ID")
	}
	if first.Workspace != "alpha" {
		t.Errorf("inferred workspace = %q, want %q", first.Workspace, "alpha")
	}
	if first.Project != project {
		t.Errorf("project = %q, want %q", first.Project, project)
	}

	// Second registration must be a no-op returning the same session.
	second, err := RegisterSession(uuid, "", "", "", "", adapter)
	if err != nil {
		t.Fatalf("second register failed: %v", err)
	}
	if !second.AlreadyTracked {
		t.Error("second register should report AlreadyTracked")
	}
	if second.SessionID != first.SessionID {
		t.Errorf("re-register minted a new session ID: %q != %q", second.SessionID, first.SessionID)
	}

	// Exactly one manifest should exist for this UUID.
	sessions, err := adapter.ListSessions(&dolt.SessionFilter{})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, m := range sessions {
		if m.Claude.UUID == uuid {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 manifest for UUID, found %d", count)
	}
}

func TestRegisterSession_FallbackWorkspace(t *testing.T) {
	adapter := dolt.GetTestAdapter(t)
	if adapter == nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	// Project dir with NO git/WORKSPACE marker, so inference yields "".
	home := t.TempDir()
	t.Setenv("HOME", home)
	uuid := "nofb-1234-5678-90ab-cdef12345678"
	project := filepath.Join(home, "loose-project")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	projectsDir := filepath.Join(home, ".claude", "projects", "encoded")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectsDir, uuid+".jsonl"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	line := fmt.Sprintf(`{"display":"x","timestamp":1733500000000,"project":"%s","sessionId":"%s"}`+"\n", project, uuid)
	if err := os.WriteFile(filepath.Join(home, ".claude", "history.jsonl"), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := RegisterSession(uuid, "", "", "cfg-default", "", adapter)
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if res.Workspace != "cfg-default" {
		t.Errorf("expected fallback workspace %q, got %q", "cfg-default", res.Workspace)
	}
}

func TestRegisterSession_ExplicitWorkspaceWins(t *testing.T) {
	adapter := dolt.GetTestAdapter(t)
	if adapter == nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	uuid := "expl-1234-5678-90ab-cdef12345678"
	setupConversation(t, uuid, "alpha") // would infer "alpha"

	res, err := RegisterSession(uuid, "custom-name", "explicit-ws", "cfg-default", "", adapter)
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if res.Workspace != "explicit-ws" {
		t.Errorf("explicit workspace should win: got %q", res.Workspace)
	}
	if res.Name != "custom-name" {
		t.Errorf("explicit name should win: got %q", res.Name)
	}
}

// TestRegisterSession_WithProjectDir_NoJSONL covers the SessionStart timing gap:
// at hook fire time, the conversation .jsonl has not been written yet, but the
// hook payload includes cwd which the caller passes as --project-dir.
func TestRegisterSession_WithProjectDir_NoJSONL(t *testing.T) {
	adapter := dolt.GetTestAdapter(t)
	if adapter == nil {
		t.Skip("Dolt not available for testing")
	}
	defer adapter.Close()

	// Set up a home with NO conversation file — simulates SessionStart timing.
	home := t.TempDir()
	t.Setenv("HOME", home)

	// Project dir with a .git marker so workspace inference resolves.
	project := filepath.Join(home, "repos", "my-project")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Deliberately do NOT create ~/.claude/projects/<encoded>/<uuid>.jsonl.

	uuid := "pdir-1234-5678-90ab-cdef12345678"
	res, err := RegisterSession(uuid, "", "", "", project, adapter)
	if err != nil {
		t.Fatalf("register with --project-dir failed when .jsonl absent: %v", err)
	}
	if res.AlreadyTracked {
		t.Error("should not be already tracked on first call")
	}
	if res.Project != project {
		t.Errorf("project = %q, want %q", res.Project, project)
	}
	if res.Workspace != "my-project" {
		t.Errorf("inferred workspace = %q, want %q", res.Workspace, "my-project")
	}
}

func TestRegisterSession_EmptyUUID(t *testing.T) {
	if _, err := RegisterSession("", "", "", "", "", nil); err == nil {
		t.Error("expected error for empty UUID")
	}
}
