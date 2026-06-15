package driftaudit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAppend_WritesJSONL(t *testing.T) {
	home := t.TempDir()
	rec := Record{
		Time:     "2026-06-15T00:00:00Z",
		HasDrift: true,
		Total:    3,
		Drift:    1,
		Drifted: []DriftTarget{{
			Name:        "stop-hook",
			Deployed:    "~/.claude/hooks/stop-agm-resource-cleanup",
			Source:      "agm/cmd/agm/hooks/stop-agm-resource-cleanup",
			Status:      "drift",
			Remediation: "agm admin install-hooks",
		}},
	}
	if err := Append(home, rec); err != nil {
		t.Fatalf("Append: %v", err)
	}
	// A second line should append, not overwrite.
	if err := Append(home, Record{Time: "2026-06-15T01:00:00Z"}); err != nil {
		t.Fatalf("Append 2: %v", err)
	}

	path := filepath.Join(home, ".local", "state", "dear-agent", LogName)
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer f.Close()

	var lines int
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var got Record
		if err := json.Unmarshal(sc.Bytes(), &got); err != nil {
			t.Fatalf("line %d is not valid JSON: %v", lines, err)
		}
		lines++
	}
	if lines != 2 {
		t.Fatalf("expected 2 JSONL lines, got %d", lines)
	}
}
