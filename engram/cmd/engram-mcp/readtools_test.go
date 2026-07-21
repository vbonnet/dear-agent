package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWayfinderStatus_ParsesStatusFile(t *testing.T) {
	dir := t.TempDir()
	content := `---
schema_version: "2.0"
project_name: test-project
project_type: feature
risk_level: M
current_waypoint: SPEC
status: in-progress
created_at: 2026-07-20T12:00:00Z
updated_at: 2026-07-20T12:30:00Z
waypoint_history:
  - {name: CHARTER, status: completed, started_at: 2026-07-20T12:00:00Z, completed_at: 2026-07-20T12:01:00Z}
  - {name: PROBLEM, status: completed, started_at: 2026-07-20T12:01:00Z, completed_at: 2026-07-20T12:02:00Z}
  - {name: RESEARCH, status: completed, started_at: 2026-07-20T12:02:00Z, completed_at: 2026-07-20T12:03:00Z}
  - {name: DESIGN, status: completed, started_at: 2026-07-20T12:03:00Z, completed_at: 2026-07-20T12:04:00Z}
---
`
	if err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := wayfinderStatus(dir)
	if err != nil {
		t.Fatalf("wayfinderStatus: %v", err)
	}
	if res.Phase != "SPEC" || res.Progress != "44%" || res.Status != "in-progress" {
		t.Errorf("parsed %+v, want SPEC / 44%% / in-progress", res)
	}
}

func TestWayfinderStatus_RejectsNonCanonicalStatusFile(t *testing.T) {
	dir := t.TempDir()
	content := "---\nstatus: in-progress\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := wayfinderStatus(dir); err == nil {
		t.Fatal("want error for non-canonical status")
	}
}

func TestWayfinderStatus_RejectsInvalidCanonicalFields(t *testing.T) {
	dir := t.TempDir()
	content := `---
schema_version: "2.0"
current_waypoint: UNKNOWN
status: typo
---
`
	if err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := wayfinderStatus(dir); err == nil {
		t.Fatal("wayfinderStatus accepted incomplete status with invalid enums")
	}
}

func TestWayfinderStatus_MissingFileIsError(t *testing.T) {
	if _, err := wayfinderStatus(t.TempDir()); err == nil {
		t.Fatal("want error for missing WAYFINDER-STATUS.md")
	}
}

func TestPluginsList_ScansCoreAndUser(t *testing.T) {
	root := t.TempDir()
	writePlugin := func(location, dir, yaml string) {
		p := filepath.Join(root, location, "plugins", dir)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "plugin.yaml"), []byte(yaml), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writePlugin("core", "alpha", "name: alpha\ntype: go\ndescription: A plugin\nversion: 1.0.0\n")
	writePlugin("user", "beta", "description: user plugin\n")

	res := pluginsList(root)
	if res.Count != 2 {
		t.Fatalf("Count = %d, want 2 (%+v)", res.Count, res.Plugins)
	}
	if res.Plugins[0].Name != "alpha" || res.Plugins[0].Location != "core" || res.Plugins[0].Type != "go" {
		t.Errorf("core plugin = %+v", res.Plugins[0])
	}
	if res.Plugins[1].Name != "beta" || res.Plugins[1].Location != "user" || res.Plugins[1].Type != "unknown" {
		t.Errorf("user plugin should default name from dir and type to unknown: %+v", res.Plugins[1])
	}
}

func TestPluginsList_EmptyRootIsEmptyResult(t *testing.T) {
	res := pluginsList(t.TempDir())
	if res.Count != 0 || len(res.SearchedPaths) != 2 {
		t.Errorf("res = %+v, want empty plugins with both searched paths", res)
	}
}

func TestParseRetrieveOutput_Shapes(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want int
	}{
		{"list", `[{"file":"a.ai.md"},{"file":"b.why.md"}]`, 2},
		{"object with results", `{"results":[{"file":"a.ai.md"}]}`, 1},
		{"bare object", `{"file":"a.ai.md"}`, 1},
		{"plain text", "not json at all", 1},
		{"empty", "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseRetrieveOutput(tc.raw); len(got) != tc.want {
				t.Errorf("len = %d, want %d (%v)", len(got), tc.want, got)
			}
		})
	}
}

func TestRetrieveItemMatchesSuffix(t *testing.T) {
	item := map[string]any{"path": "docs/errors.ai.md"}
	if !retrieveItemMatchesSuffix(item, ".ai.md") {
		t.Error("want match on path key")
	}
	if retrieveItemMatchesSuffix(item, ".why.md") {
		t.Error("want no match for wrong suffix")
	}
}

func TestParseFlatYAML_SkipsNestedAndComments(t *testing.T) {
	meta := parseFlatYAML("# comment\nname: alpha\nnested:\n  key: val\ndescription: \"quoted\"\n")
	if meta["name"] != "alpha" || meta["description"] != "quoted" {
		t.Errorf("meta = %+v", meta)
	}
	if _, ok := meta["key"]; ok {
		t.Error("indented keys must be skipped")
	}
}
