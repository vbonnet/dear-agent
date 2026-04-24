package errormemory

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleLog = `[2026-03-09 05:22:30] Hook invoked
[2026-03-09 05:22:30] Raw input: {"tool_name":"Bash","tool_input":{"command":"cd /tmp && go test"},"session_id":"sess-001"}
[2026-03-09 05:22:31] Processing command: cd /tmp && go test
[2026-03-09 05:22:31] DENIED: cd command - Use absolute paths instead: e.g., 'go -C /path test' not 'cd /path && go test'
[2026-03-09 05:22:45] Hook invoked
[2026-03-09 05:22:45] Raw input: {"tool_name":"Bash","tool_input":{"command":"ls -la /tmp"},"session_id":"sess-001"}
[2026-03-09 05:22:45] Processing command: ls -la /tmp
[2026-03-09 05:22:45] DENIED: text processing (ls/grep/sed/awk/head/tail/wc/cut/sort/uniq) - Use Grep tool (for grep), Glob tool (for ls/find), Read tool (for head/tail/cat)
[2026-03-09 05:23:00] Hook invoked
[2026-03-09 05:23:00] Raw input: {"tool_name":"Bash","tool_input":{"command":"cd /var && ls"},"session_id":"sess-002"}
[2026-03-09 05:23:00] Processing command: cd /var && ls
[2026-03-09 05:23:00] DENIED: cd command - Use absolute paths instead: e.g., 'go -C /path test' not 'cd /path && go test'
[2026-03-09 05:23:15] Hook invoked
[2026-03-09 05:23:15] Raw input: {"tool_name":"Bash","tool_input":{"command":"grep -r foo ."},"session_id":"sess-002"}
[2026-03-09 05:23:15] Processing command: grep -r foo .
[2026-03-09 05:23:15] DENIED: text processing (ls/grep/sed/awk/head/tail/wc/cut/sort/uniq) - Use Grep tool (for grep), Glob tool (for ls/find), Read tool (for head/tail/cat)
`

func writeSampleLog(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "pretool-bash-blocker.log")
	if err := os.WriteFile(path, []byte(sampleLog), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBackfillFromLog(t *testing.T) {
	path := writeSampleLog(t)

	result, err := BackfillFromLog(path)
	if err != nil {
		t.Fatalf("BackfillFromLog failed: %v", err)
	}

	if result.DeniedFound != 4 {
		t.Errorf("expected 4 denied entries, got %d", result.DeniedFound)
	}

	if result.UniquePatterns != 2 {
		t.Errorf("expected 2 unique patterns, got %d", result.UniquePatterns)
	}

	if len(result.Records) != 2 {
		t.Fatalf("expected 2 records, got %d", len(result.Records))
	}

	// Verify patterns exist
	patterns := map[string]bool{}
	for _, rec := range result.Records {
		patterns[rec.Pattern] = true
	}
	if !patterns["cd command"] {
		t.Error("missing 'cd command' pattern")
	}
	if !patterns["text processing (ls/grep/sed/awk/head/tail/wc/cut/sort/uniq)"] {
		t.Error("missing 'text processing' pattern")
	}
}

func TestBackfillDedup(t *testing.T) {
	path := writeSampleLog(t)

	result, err := BackfillFromLog(path)
	if err != nil {
		t.Fatalf("BackfillFromLog failed: %v", err)
	}

	for _, rec := range result.Records {
		if rec.Pattern == "cd command" {
			if rec.Count != 2 {
				t.Errorf("cd command: expected count 2, got %d", rec.Count)
			}
		}
		if rec.Pattern == "text processing (ls/grep/sed/awk/head/tail/wc/cut/sort/uniq)" {
			if rec.Count != 2 {
				t.Errorf("text processing: expected count 2, got %d", rec.Count)
			}
		}
	}
}

func TestBackfillSessionID(t *testing.T) {
	path := writeSampleLog(t)

	result, err := BackfillFromLog(path)
	if err != nil {
		t.Fatalf("BackfillFromLog failed: %v", err)
	}

	for _, rec := range result.Records {
		if rec.Pattern == "cd command" {
			if len(rec.SessionIDs) != 2 {
				t.Errorf("cd command: expected 2 session IDs, got %d: %v", len(rec.SessionIDs), rec.SessionIDs)
			}
			hasS1 := false
			hasS2 := false
			for _, sid := range rec.SessionIDs {
				if sid == "sess-001" {
					hasS1 = true
				}
				if sid == "sess-002" {
					hasS2 = true
				}
			}
			if !hasS1 || !hasS2 {
				t.Errorf("cd command: expected session IDs [sess-001, sess-002], got %v", rec.SessionIDs)
			}
		}
	}
}

func TestBackfillNonexistentFile(t *testing.T) {
	_, err := BackfillFromLog("/tmp/nonexistent-backfill-log-12345.log")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}
