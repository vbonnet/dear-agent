package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeStatus creates a WAYFINDER-STATUS.md in dir with the given frontmatter.
func writeStatus(t *testing.T, dir, frontmatter string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nschema_version: \"2.0\"\n" + frontmatter + "\n---\n"
	if err := os.WriteFile(filepath.Join(dir, wayfinderStatusFile), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestParseFrontmatter_SchemaV2(t *testing.T) {
	content := []byte("---\nschema_version: \"2.0\"\nproject_name: my-project\nstatus: in-progress\ncurrent_waypoint: DESIGN\n---\n# Rest of file")
	fm, err := parseFrontmatter(content)
	if err != nil {
		t.Fatalf("parseFrontmatter: %v", err)
	}
	if fm["project_name"] != "my-project" {
		t.Errorf("project_name = %q, want my-project", fm["project_name"])
	}
	if fm["status"] != "in-progress" {
		t.Errorf("status = %q, want in-progress", fm["status"])
	}
}

func TestReadWayfinderSession_RejectsNonCanonicalSchema(t *testing.T) {
	dir := t.TempDir()
	content := []byte("---\nproject_name: old-project\nstatus: completed\ncurrent_waypoint: RETRO\n---\n")
	if err := os.WriteFile(filepath.Join(dir, wayfinderStatusFile), content, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readWayfinderSession(dir); err == nil {
		t.Fatal("readWayfinderSession accepted a status without schema_version")
	}
}

func TestParseFrontmatter_NoFrontmatter(t *testing.T) {
	_, err := parseFrontmatter([]byte("# Just a heading\nno frontmatter here"))
	if err == nil {
		t.Error("expected error for file without frontmatter")
	}
}

func TestListWayfinderSessions_Basic(t *testing.T) {
	root := t.TempDir()

	writeStatus(t, filepath.Join(root, "alpha"), "project_name: alpha\nstatus: in-progress\ncurrent_waypoint: DESIGN\n")
	writeStatus(t, filepath.Join(root, "beta"), "project_name: beta\nstatus: completed\ncurrent_waypoint: RETRO\n")
	writeStatus(t, filepath.Join(root, "gamma"), "project_name: gamma\nstatus: in-progress\ncurrent_waypoint: CHARTER\n")

	sessions, err := listWayfinderSessions(root, "", 0)
	if err != nil {
		t.Fatalf("listWayfinderSessions: %v", err)
	}
	if len(sessions) != 3 {
		t.Errorf("got %d sessions, want 3", len(sessions))
	}
}

func TestListWayfinderSessions_StatusFilter(t *testing.T) {
	root := t.TempDir()

	writeStatus(t, filepath.Join(root, "active1"), "project_name: active1\nstatus: in-progress\ncurrent_waypoint: BUILD\n")
	writeStatus(t, filepath.Join(root, "done1"), "project_name: done1\nstatus: completed\ncurrent_waypoint: RETRO\n")
	writeStatus(t, filepath.Join(root, "active2"), "project_name: active2\nstatus: in-progress\ncurrent_waypoint: PLAN\n")

	sessions, err := listWayfinderSessions(root, "in-progress", 0)
	if err != nil {
		t.Fatalf("listWayfinderSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("got %d sessions, want 2 in-progress", len(sessions))
	}
	for _, s := range sessions {
		if s.Status != "in-progress" {
			t.Errorf("session %q has status %q, want in-progress", s.ID, s.Status)
		}
	}
}

func TestListWayfinderSessions_Limit(t *testing.T) {
	root := t.TempDir()

	for _, name := range []string{"a", "b", "c", "d", "e"} {
		writeStatus(t, filepath.Join(root, name), "project_name: "+name+"\nstatus: in-progress\ncurrent_waypoint: BUILD\n")
	}

	sessions, err := listWayfinderSessions(root, "", 3)
	if err != nil {
		t.Fatalf("listWayfinderSessions: %v", err)
	}
	if len(sessions) != 3 {
		t.Errorf("got %d sessions, want 3 (limit)", len(sessions))
	}
}

func TestListWayfinderSessions_SkipsNonDirs(t *testing.T) {
	root := t.TempDir()

	writeStatus(t, filepath.Join(root, "real-session"), "project_name: real\nstatus: in-progress\ncurrent_waypoint: BUILD\n")
	// Create a plain file (should be skipped)
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# root readme"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Create a dir without WAYFINDER-STATUS.md (should be skipped)
	if err := os.MkdirAll(filepath.Join(root, "no-status"), 0o755); err != nil {
		t.Fatal(err)
	}

	sessions, err := listWayfinderSessions(root, "", 0)
	if err != nil {
		t.Fatalf("listWayfinderSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Errorf("got %d sessions, want 1 (only real-session)", len(sessions))
	}
}

func TestGetWayfinderSessionDetail_Found(t *testing.T) {
	root := t.TempDir()
	writeStatus(t, filepath.Join(root, "my-project"), "project_name: my-project\nstatus: in-progress\ncurrent_waypoint: DESIGN\n")

	detail, err := getWayfinderSessionDetail(root, "my-project")
	if err != nil {
		t.Fatalf("getWayfinderSessionDetail: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(detail), &out); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}
	if out["id"] != "my-project" {
		t.Errorf("id = %v, want my-project", out["id"])
	}
	if out["status"] != "in-progress" {
		t.Errorf("status = %v, want in-progress", out["status"])
	}
}

func TestGetWayfinderSessionDetail_NotFound(t *testing.T) {
	root := t.TempDir()
	_, err := getWayfinderSessionDetail(root, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestGetWayfinderSessionDetail_PathTraversal(t *testing.T) {
	root := t.TempDir()
	_, err := getWayfinderSessionDetail(root, "../escape")
	if err == nil {
		t.Error("expected error for path traversal attempt")
	}
}

func TestFmString_CanonicalKey(t *testing.T) {
	fm := map[string]any{
		"project_name": "canonical-name",
	}
	got := fmString(fm, "project_name")
	if got != "canonical-name" {
		t.Errorf("fmString = %q, want canonical-name", got)
	}
}
