package orphan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// These tests exercise the filesystem-backed pure logic of the orphan detector
// (history scanning, conversation-file lookup, and the nil-adapter guard)
// without requiring a live Dolt server. They drive behaviour through a
// temporary HOME directory. (ce-6as.44)

func TestFindConversationFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	projectsDir := filepath.Join(home, ".claude", "projects", "-Users-x-src-repo")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	uuid := "abc-123"
	jsonlPath := filepath.Join(projectsDir, uuid+".jsonl")
	if err := os.WriteFile(jsonlPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("finds existing conversation file", func(t *testing.T) {
		got := findConversationFile(uuid)
		if got != jsonlPath {
			t.Errorf("findConversationFile(%q) = %q, want %q", uuid, got, jsonlPath)
		}
	})

	t.Run("returns empty for unknown uuid", func(t *testing.T) {
		if got := findConversationFile("does-not-exist"); got != "" {
			t.Errorf("expected empty string, got %q", got)
		}
	})

	t.Run("returns empty when projects dir missing", func(t *testing.T) {
		emptyHome := t.TempDir()
		t.Setenv("HOME", emptyHome)
		if got := findConversationFile(uuid); got != "" {
			t.Errorf("expected empty string for missing projects dir, got %q", got)
		}
	})
}

func TestScanHistoryForUUIDs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	claudeDir := filepath.Join(home, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// timestamp in milliseconds
	const tsMillis = int64(1_700_000_000_000)
	historyLines := `{"sessionId":"uuid-oss","timestamp":1700000000000,"project":"/home/x/src/ws/oss/repo"}
{"sessionId":"uuid-plain","timestamp":1700000000000,"project":"/home/x/src/standalone"}
{"sessionId":"","timestamp":1700000000000,"project":"/home/x/ignored"}
`
	if err := os.WriteFile(filepath.Join(claudeDir, "history.jsonl"), []byte(historyLines), 0o644); err != nil {
		t.Fatal(err)
	}

	uuidSet, sessionMap, err := scanHistoryForUUIDs()
	if err != nil {
		t.Fatalf("scanHistoryForUUIDs returned error: %v", err)
	}

	if !uuidSet["uuid-oss"] || !uuidSet["uuid-plain"] {
		t.Errorf("expected both session UUIDs in set, got %v", uuidSet)
	}
	if uuidSet[""] {
		t.Error("empty session ID should be skipped")
	}

	oss := sessionMap["uuid-oss"]
	if oss == nil {
		t.Fatal("expected sessionInfo for uuid-oss")
	}
	if oss.Workspace != "oss" {
		t.Errorf("expected workspace 'oss', got %q", oss.Workspace)
	}
	if oss.ProjectPath != "/home/x/src/ws/oss/repo" {
		t.Errorf("unexpected project path: %q", oss.ProjectPath)
	}
	wantTime := time.Unix(0, tsMillis*int64(time.Millisecond))
	if !oss.LastModified.Equal(wantTime) {
		t.Errorf("LastModified = %v, want %v", oss.LastModified, wantTime)
	}

	if plain := sessionMap["uuid-plain"]; plain == nil || plain.Workspace != "" {
		t.Errorf("expected empty workspace for non-ws path, got %+v", plain)
	}
}

func TestScanHistoryForUUIDs_EmptyWhenNoHistory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	uuidSet, sessionMap, err := scanHistoryForUUIDs()
	if err != nil {
		t.Fatalf("expected no error for missing history file, got %v", err)
	}
	if len(uuidSet) != 0 || len(sessionMap) != 0 {
		t.Errorf("expected empty results, got uuids=%v sessions=%v", uuidSet, sessionMap)
	}
}

func TestDetectOrphansRequiresAdapter(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// With a nil adapter the detector must fail fast rather than panic, even
	// after a successful (empty) history scan.
	_, err := DetectOrphans(home, "", nil)
	if err == nil {
		t.Fatal("expected error when adapter is nil")
	}
	if !strings.Contains(err.Error(), "dolt adapter required") {
		t.Errorf("unexpected error message: %v", err)
	}
}
