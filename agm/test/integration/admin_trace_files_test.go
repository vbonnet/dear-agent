package integration

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/trace"
)

func TestAdminTraceFiles_SingleFile(t *testing.T) {
	tmpDir := setupTraceTestEnv(t)
	defer os.RemoveAll(tmpDir)

	tracer := trace.NewTracer(tmpDir)
	results, err := tracer.TraceFiles(trace.TraceOptions{
		FilePaths:   []string{"/tmp/test-ws/oss/README.md"},
		SessionsDir: tmpDir,
	})

	if err != nil {
		t.Fatalf("TraceFiles failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	result := results[0]
	if result.FilePath != "/tmp/test-ws/oss/README.md" {
		t.Errorf("file path = %s, want /tmp/test-ws/oss/README.md", result.FilePath)
	}

	if len(result.Sessions) == 0 {
		t.Errorf("expected at least 1 session, got 0")
	}

	if len(result.Sessions) > 0 {
		if result.Sessions[0].SessionID != "session-file-edit-001" {
			t.Errorf("session ID = %s, want session-file-edit-001", result.Sessions[0].SessionID)
		}

		if result.Sessions[0].SessionName != "readme-updates" {
			t.Errorf("session name = %s, want readme-updates", result.Sessions[0].SessionName)
		}

		// Should have 2 modifications for README.md
		if len(result.Sessions[0].Modifications) != 2 {
			t.Errorf("expected 2 modifications, got %d", len(result.Sessions[0].Modifications))
		}
	}
}

func TestAdminTraceFiles_MultipleFiles(t *testing.T) {
	tmpDir := setupTraceTestEnv(t)
	defer os.RemoveAll(tmpDir)

	tracer := trace.NewTracer(tmpDir)
	results, err := tracer.TraceFiles(trace.TraceOptions{
		FilePaths: []string{
			"/tmp/test-ws/oss/README.md",
			"/tmp/test-ws/oss/src/main.go",
		},
		SessionsDir: tmpDir,
	})

	if err != nil {
		t.Fatalf("TraceFiles failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	// Verify both files have results
	var readmeResult, mainGoResult *trace.TraceResult
	for _, result := range results {
		if result.FilePath == "/tmp/test-ws/oss/README.md" {
			readmeResult = result
		} else if result.FilePath == "/tmp/test-ws/oss/src/main.go" {
			mainGoResult = result
		}
	}

	if readmeResult == nil {
		t.Error("README.md result not found")
	} else if len(readmeResult.Sessions) == 0 {
		t.Error("README.md should have sessions")
	}

	if mainGoResult == nil {
		t.Error("main.go result not found")
	} else if len(mainGoResult.Sessions) == 0 {
		t.Error("main.go should have sessions")
	}

	// Verify different sessions
	if readmeResult != nil && mainGoResult != nil {
		if len(readmeResult.Sessions) > 0 && len(mainGoResult.Sessions) > 0 {
			if readmeResult.Sessions[0].SessionID == mainGoResult.Sessions[0].SessionID {
				t.Error("README.md and main.go should have different sessions")
			}
		}
	}
}

func TestAdminTraceFiles_NoMatch(t *testing.T) {
	tmpDir := setupTraceTestEnv(t)
	defer os.RemoveAll(tmpDir)

	tracer := trace.NewTracer(tmpDir)
	results, err := tracer.TraceFiles(trace.TraceOptions{
		FilePaths:   []string{"/tmp/test-ws/nonexistent.txt"},
		SessionsDir: tmpDir,
	})

	if err != nil {
		t.Fatalf("TraceFiles failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if len(results[0].Sessions) != 0 {
		t.Errorf("expected no sessions for nonexistent file, got %d", len(results[0].Sessions))
	}
}

func TestAdminTraceFiles_SinceFilter(t *testing.T) {
	tmpDir := setupTraceTestEnv(t)
	defer os.RemoveAll(tmpDir)

	// Timestamps in test data:
	// 1708300000000 = 2024-02-18 23:46:40 UTC (first README.md mod)
	// 1708330000000 = 2024-02-19 08:06:40 UTC (second README.md mod)
	// Filter to show only mods after 2024-02-19 00:00:00 (will exclude first, include second)
	sinceTime := time.Date(2024, 2, 19, 0, 0, 0, 0, time.UTC)

	tracer := trace.NewTracer(tmpDir)
	results, err := tracer.TraceFiles(trace.TraceOptions{
		FilePaths:   []string{"/tmp/test-ws/oss/README.md"},
		Since:       &sinceTime,
		SessionsDir: tmpDir,
	})

	if err != nil {
		t.Fatalf("TraceFiles failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	result := results[0]
	if len(result.Sessions) == 0 {
		t.Error("expected at least 1 session after filtering")
	}

	// Should have only 1 modification (the later one on 2024-02-19)
	if len(result.Sessions) > 0 && len(result.Sessions[0].Modifications) != 1 {
		t.Errorf("expected 1 modification after filtering, got %d", len(result.Sessions[0].Modifications))
	}
}

func TestAdminTraceFiles_WorkspaceFilter(t *testing.T) {
	tmpDir := setupTraceTestEnv(t)
	defer os.RemoveAll(tmpDir)

	// Create second workspace
	acmeDir := filepath.Join(tmpDir, "acme")
	if err := os.MkdirAll(acmeDir, 0755); err != nil {
		t.Fatalf("failed to create acme workspace: %v", err)
	}

	// Create history for acme workspace
	historyPath := filepath.Join(acmeDir, "history.jsonl")
	historyContent := `{"sessionId":"session-acme-001","project":"~/src/ws/acme","timestamp":1708300000000,"files_modified":["/tmp/test-ws/acme/config.yaml"]}
`
	if err := os.WriteFile(historyPath, []byte(historyContent), 0644); err != nil {
		t.Fatalf("failed to write acme history: %v", err)
	}

	// Create manifest for acme session
	sessionDir := filepath.Join(acmeDir, "session-acme-001")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("failed to create session dir: %v", err)
	}

	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	manifestContent := `name: acme-session
workspace: acme
claude:
  uuid: session-acme-001
context:
  project: ~/src/ws/acme
`
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	// Run with workspace filter
	tracer := trace.NewTracer(tmpDir)
	results, err := tracer.TraceFiles(trace.TraceOptions{
		FilePaths:   []string{"/tmp/test-ws/oss/README.md"},
		Workspace:   "oss",
		SessionsDir: tmpDir,
	})

	if err != nil {
		t.Fatalf("TraceFiles failed: %v", err)
	}

	// Should find oss sessions
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if len(results[0].Sessions) == 0 {
		t.Error("expected oss sessions")
	}

	// Verify it's the oss session
	if len(results[0].Sessions) > 0 {
		if results[0].Sessions[0].Workspace != "oss" {
			t.Errorf("session workspace = %s, want oss", results[0].Sessions[0].Workspace)
		}
	}
}

func TestAdminTraceFiles_CorruptedHistory(t *testing.T) {
	tmpDir := setupTraceTestEnv(t)
	defer os.RemoveAll(tmpDir)

	// Append corrupted entry to history
	historyPath := filepath.Join(tmpDir, "oss", "history.jsonl")
	f, err := os.OpenFile(historyPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open history: %v", err)
	}
	defer f.Close()

	// Add corrupted line
	if _, err := f.WriteString("{invalid json}\n"); err != nil {
		t.Fatalf("failed to write corrupted line: %v", err)
	}
	f.Close()

	// Command should still succeed
	tracer := trace.NewTracer(tmpDir)
	results, err := tracer.TraceFiles(trace.TraceOptions{
		FilePaths:   []string{"/tmp/test-ws/oss/README.md"},
		SessionsDir: tmpDir,
	})

	if err != nil {
		t.Fatalf("TraceFiles should not fail on corrupted history: %v", err)
	}

	// Should still show valid results
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if len(results[0].Sessions) == 0 {
		t.Error("expected sessions despite corrupted entry")
	}
}

func TestAdminTraceFiles_MultipleSessionsSameFile(t *testing.T) {
	tmpDir := setupTraceTestEnv(t)
	defer os.RemoveAll(tmpDir)

	// Add another session modifying the same file
	workspaceDir := filepath.Join(tmpDir, "oss")
	historyPath := filepath.Join(workspaceDir, "history.jsonl")
	f, err := os.OpenFile(historyPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open history: %v", err)
	}
	defer f.Close()

	// Add entry from different session
	if _, err := f.WriteString(`{"sessionId":"session-file-edit-003","project":"~/src/ws/oss","timestamp":1708360000000,"files_modified":["/tmp/test-ws/oss/README.md"]}` + "\n"); err != nil {
		t.Fatalf("failed to write additional entry: %v", err)
	}
	f.Close()

	// Create manifest for third session
	createTestManifest(t, workspaceDir, "session-file-edit-003", "another-readme-edit", "oss")

	tracer := trace.NewTracer(tmpDir)
	results, err := tracer.TraceFiles(trace.TraceOptions{
		FilePaths:   []string{"/tmp/test-ws/oss/README.md"},
		SessionsDir: tmpDir,
	})

	if err != nil {
		t.Fatalf("TraceFiles failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	// Should have 2 sessions (session-file-edit-001 and session-file-edit-003)
	if len(results[0].Sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(results[0].Sessions))
	}

	// Sessions should be ordered by first modification time
	if len(results[0].Sessions) >= 2 {
		if results[0].Sessions[0].SessionID != "session-file-edit-001" {
			t.Errorf("first session should be session-file-edit-001, got %s", results[0].Sessions[0].SessionID)
		}
		if results[0].Sessions[1].SessionID != "session-file-edit-003" {
			t.Errorf("second session should be session-file-edit-003, got %s", results[0].Sessions[1].SessionID)
		}
	}
}

func TestAdminTraceFiles_OrphanedSession(t *testing.T) {
	tmpDir := setupTraceTestEnv(t)
	defer os.RemoveAll(tmpDir)

	// Add history entry for session without manifest
	workspaceDir := filepath.Join(tmpDir, "oss")
	historyPath := filepath.Join(workspaceDir, "history.jsonl")
	f, err := os.OpenFile(historyPath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open history: %v", err)
	}
	defer f.Close()

	// Add orphaned session entry
	if _, err := f.WriteString(`{"sessionId":"orphan-session-999","project":"~/src/ws/oss","timestamp":1708370000000,"files_modified":["/tmp/test-ws/oss/orphan.txt"]}` + "\n"); err != nil {
		t.Fatalf("failed to write orphan entry: %v", err)
	}
	f.Close()

	tracer := trace.NewTracer(tmpDir)
	results, err := tracer.TraceFiles(trace.TraceOptions{
		FilePaths:   []string{"/tmp/test-ws/oss/orphan.txt"},
		SessionsDir: tmpDir,
	})

	if err != nil {
		t.Fatalf("TraceFiles failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if len(results[0].Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(results[0].Sessions))
	}

	session := results[0].Sessions[0]
	if session.SessionID != "orphan-session-999" {
		t.Errorf("session ID = %s, want orphan-session-999", session.SessionID)
	}

	if session.SessionName != "<no manifest>" {
		t.Errorf("session name = %s, want <no manifest>", session.SessionName)
	}
}

// setupTraceTestEnv creates a test environment with sample history and manifests
func setupTraceTestEnv(t *testing.T) string {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "agm-trace-test-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Create workspace directory
	workspaceDir := filepath.Join(tmpDir, "oss")
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		t.Fatalf("failed to create workspace dir: %v", err)
	}

	// Create history.jsonl with file modifications
	historyPath := filepath.Join(workspaceDir, "history.jsonl")
	historyContent := `{"sessionId":"session-file-edit-001","project":"~/src/ws/oss","timestamp":1708300000000,"files_modified":["/tmp/test-ws/oss/README.md","/tmp/test-ws/oss/package.json"]}
{"sessionId":"session-file-edit-002","project":"~/src/ws/oss","timestamp":1708310000000,"files_modified":["/tmp/test-ws/oss/src/main.go","/tmp/test-ws/oss/go.mod"]}
{"sessionId":"session-file-edit-001","project":"~/src/ws/oss","timestamp":1708330000000,"files_modified":["/tmp/test-ws/oss/README.md","/tmp/test-ws/oss/CONTRIBUTING.md"]}
{"sessionId":"session-file-edit-004","project":"~/src/ws/oss","timestamp":1708340000000,"files_modified":["/tmp/test-ws/oss/internal/manifest/manifest.go"]}
{"sessionId":"session-file-edit-002","project":"~/src/ws/oss","timestamp":1708350000000,"files_modified":["/tmp/test-ws/oss/src/main.go"]}
`
	if err := os.WriteFile(historyPath, []byte(historyContent), 0644); err != nil {
		t.Fatalf("failed to write history: %v", err)
	}

	// Create manifests
	createTestManifest(t, workspaceDir, "session-file-edit-001", "readme-updates", "oss")
	createTestManifest(t, workspaceDir, "session-file-edit-002", "main-refactor", "oss")

	return tmpDir
}

// createTestManifest creates a test manifest.yaml
func createTestManifest(t *testing.T, workspaceDir, sessionID, name, workspace string) {
	t.Helper()

	sessionDir := filepath.Join(workspaceDir, sessionID)
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("failed to create session dir: %v", err)
	}

	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	manifestContent := `schema_version: "2.0"
name: ` + name + `
workspace: ` + workspace + `
claude:
  uuid: ` + sessionID + `
context:
  project: ~/src/ws/oss
created_at: 2024-02-19T08:00:00Z
updated_at: 2024-02-19T14:00:00Z
`
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}
}
