package statusread

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFromDirRejectsIncompleteCanonicalStatus(t *testing.T) {
	dir := t.TempDir()
	contents := `---
schema_version: "2.0"
status: completed
current_waypoint: RETRO
---
`
	if err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := ParseFromDir(dir)
	if err == nil || !strings.Contains(err.Error(), "project_name is required") {
		t.Fatalf("ParseFromDir() error = %v, want missing project_name", err)
	}
}
