package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeStatus creates a complete canonical WAYFINDER-STATUS.md.
func writeStatus(t *testing.T, dir, projectName, status, waypoint string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	completion := ""
	if status == "completed" {
		completion = "completion_date: 2026-07-20T00:00:00Z\n"
	}
	var history strings.Builder
	for _, predecessor := range []string{"CHARTER", "PROBLEM", "RESEARCH", "DESIGN", "SPEC", "PLAN", "SETUP", "BUILD", "RETRO"} {
		if predecessor == waypoint {
			if status == "completed" {
				fmt.Fprintf(&history, "  - {name: %s, status: completed, started_at: 2026-07-20T00:00:00Z, completed_at: 2026-07-20T00:01:00Z}\n", predecessor)
			}
			break
		}
		fmt.Fprintf(&history, "  - {name: %s, status: completed, started_at: 2026-07-20T00:00:00Z, completed_at: 2026-07-20T00:01:00Z}\n", predecessor)
	}
	historyYAML := "waypoint_history: []\n"
	if history.Len() > 0 {
		historyYAML = "waypoint_history:\n" + history.String()
	}
	content := fmt.Sprintf(`---
schema_version: "2.0"
project_name: %s
project_type: feature
risk_level: S
status: %s
current_waypoint: %s
created_at: 2026-07-20T00:00:00Z
updated_at: 2026-07-20T00:00:00Z
%s%s---
`, projectName, status, waypoint, completion, historyYAML)
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

	writeStatus(t, filepath.Join(root, "alpha"), "alpha", "in-progress", "DESIGN")
	writeStatus(t, filepath.Join(root, "beta"), "beta", "completed", "RETRO")
	writeStatus(t, filepath.Join(root, "gamma"), "gamma", "in-progress", "CHARTER")

	sessions, err := listWayfinderSessions(root, "", 0)
	if err != nil {
		t.Fatalf("listWayfinderSessions: %v", err)
	}
	if len(sessions) != 3 {
		t.Errorf("got %d sessions, want 3", len(sessions))
	}
	if sessions[0].CreatedAt != "2026-07-20T00:00:00Z" || sessions[0].UpdatedAt != "2026-07-20T00:00:00Z" {
		t.Errorf("canonical timestamps were not preserved: %+v", sessions[0])
	}
}

func TestListWayfinderSessions_StatusFilter(t *testing.T) {
	root := t.TempDir()

	writeStatus(t, filepath.Join(root, "active1"), "active1", "in-progress", "BUILD")
	writeStatus(t, filepath.Join(root, "done1"), "done1", "completed", "RETRO")
	writeStatus(t, filepath.Join(root, "active2"), "active2", "in-progress", "PLAN")

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
		writeStatus(t, filepath.Join(root, name), name, "in-progress", "BUILD")
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

	writeStatus(t, filepath.Join(root, "real-session"), "real", "in-progress", "BUILD")
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
	writeStatus(t, filepath.Join(root, "my-project"), "my-project", "in-progress", "DESIGN")

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

func TestListWayfinderSessions_SkipsIncompleteCanonicalStatus(t *testing.T) {
	root := t.TempDir()
	incompleteDir := filepath.Join(root, "incomplete")
	if err := os.MkdirAll(incompleteDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("---\nschema_version: \"2.0\"\nproject_name: incomplete\nstatus: in-progress\ncurrent_waypoint: BUILD\n---\n")
	if err := os.WriteFile(filepath.Join(incompleteDir, wayfinderStatusFile), content, 0o644); err != nil {
		t.Fatal(err)
	}

	sessions, err := listWayfinderSessions(root, "", 0)
	if err != nil {
		t.Fatalf("listWayfinderSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("listWayfinderSessions returned incomplete status: %+v", sessions)
	}
}

func TestGetWayfinderSessionDetail_RejectsInvalidCanonicalEnum(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "invalid")
	writeStatus(t, dir, "invalid", "in-progress", "BUILD")
	path := filepath.Join(dir, wayfinderStatusFile)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content = []byte(strings.Replace(string(content), "risk_level: S", "risk_level: impossible", 1))
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := getWayfinderSessionDetail(root, "invalid"); err == nil {
		t.Fatal("getWayfinderSessionDetail accepted an invalid canonical enum")
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

func TestFmString_CanonicalTimestamp(t *testing.T) {
	stamp := time.Date(2026, 7, 20, 1, 2, 3, 0, time.UTC)
	if got := fmString(map[string]any{"created_at": stamp}, "created_at"); got != "2026-07-20T01:02:03Z" {
		t.Fatalf("fmString timestamp = %q", got)
	}
}
