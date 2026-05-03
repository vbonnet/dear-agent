package trace

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseHistoryFile(t *testing.T) {
	tests := []struct {
		name           string
		content        string
		wantEntries    int
		wantFirstID    string
		wantFirstFiles []string
	}{
		{
			name: "valid entries",
			content: `{"sessionId":"session-001","project":"/home/user","timestamp":1708300000000,"files_modified":["~/README.md"]}
{"sessionId":"session-002","project":"/home/user","timestamp":1708310000000,"files_modified":["~/main.go"]}
`,
			wantEntries:    2,
			wantFirstID:    "session-001",
			wantFirstFiles: []string{"~/README.md"},
		},
		{
			name: "empty lines",
			content: `{"sessionId":"session-001","project":"/home/user","timestamp":1708300000000,"files_modified":["~/README.md"]}

{"sessionId":"session-002","project":"/home/user","timestamp":1708310000000,"files_modified":["~/main.go"]}
`,
			wantEntries:    2,
			wantFirstID:    "session-001",
			wantFirstFiles: []string{"~/README.md"},
		},
		{
			name:           "null byte corruption",
			content:        "{\x00\"sessionId\":\"session-001\",\"project\":\"/home/user\",\"timestamp\":1708300000000,\"files_modified\":[\"~/README.md\"]}\n",
			wantEntries:    1,
			wantFirstID:    "session-001",
			wantFirstFiles: []string{"~/README.md"},
		},
		{
			name: "no files_modified field",
			content: `{"sessionId":"session-001","project":"/home/user","timestamp":1708300000000}
{"sessionId":"session-002","project":"/home/user","timestamp":1708310000000,"files_modified":["~/main.go"]}
`,
			wantEntries:    1,
			wantFirstID:    "session-002",
			wantFirstFiles: []string{"~/main.go"},
		},
		{
			name: "malformed JSON",
			content: `{"sessionId":"session-001","project":"/home/user","timestamp":1708300000000,"files_modified":["~/README.md"]}
{invalid json}
{"sessionId":"session-002","project":"/home/user","timestamp":1708310000000,"files_modified":["~/main.go"]}
`,
			wantEntries:    2,
			wantFirstID:    "session-001",
			wantFirstFiles: []string{"~/README.md"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tmpFile, err := os.CreateTemp(t.TempDir(), "history-*.jsonl")
			if err != nil {
				t.Fatalf("failed to create temp file: %v", err)
			}
			defer os.Remove(tmpFile.Name())
			defer tmpFile.Close()

			// Write content
			if _, err := tmpFile.WriteString(tt.content); err != nil {
				t.Fatalf("failed to write temp file: %v", err)
			}
			tmpFile.Close()

			// Parse
			tracer := NewTracer("")
			entries, err := tracer.parseHistoryFile(tmpFile.Name())
			if err != nil {
				t.Fatalf("parseHistoryFile() error = %v", err)
			}

			if len(entries) != tt.wantEntries {
				t.Errorf("got %d entries, want %d", len(entries), tt.wantEntries)
			}

			if len(entries) > 0 {
				if entries[0].SessionID != tt.wantFirstID {
					t.Errorf("first entry sessionId = %s, want %s", entries[0].SessionID, tt.wantFirstID)
				}

				if len(entries[0].FilesModified) != len(tt.wantFirstFiles) {
					t.Errorf("first entry has %d files, want %d", len(entries[0].FilesModified), len(tt.wantFirstFiles))
				} else {
					for i, file := range entries[0].FilesModified {
						if file != tt.wantFirstFiles[i] {
							t.Errorf("first entry file[%d] = %s, want %s", i, file, tt.wantFirstFiles[i])
						}
					}
				}
			}
		})
	}
}

func TestMatchesPath(t *testing.T) {
	tests := []struct {
		name         string
		modifiedPath string
		targetPath   string
		want         bool
	}{
		{
			name:         "exact match",
			modifiedPath: "~/src/README.md",
			targetPath:   "~/src/README.md",
			want:         true,
		},
		{
			name:         "substring match",
			modifiedPath: "~/src/ws/oss/README.md",
			targetPath:   "README.md",
			want:         true,
		},
		{
			name:         "suffix match",
			modifiedPath: "~/src/project/main.go",
			targetPath:   "project/main.go",
			want:         true,
		},
		{
			name:         "no match",
			modifiedPath: "~/src/README.md",
			targetPath:   "~/doc/README.md",
			want:         false,
		},
		{
			name:         "case sensitive",
			modifiedPath: "~/src/README.md",
			targetPath:   "~/src/readme.md",
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesPath(tt.modifiedPath, tt.targetPath)
			if got != tt.want {
				t.Errorf("matchesPath(%q, %q) = %v, want %v", tt.modifiedPath, tt.targetPath, got, tt.want)
			}
		})
	}
}

func TestTraceFile(t *testing.T) {
	// Create test entries
	entries := []HistoryEntry{
		{
			SessionID:     "session-001",
			Project:       "~/src",
			Timestamp:     1708300000000, // 2024-02-19 08:00:00
			FilesModified: []string{"~/src/README.md", "~/src/package.json"},
		},
		{
			SessionID:     "session-002",
			Project:       "~/src",
			Timestamp:     1708310000000, // 2024-02-19 10:46:40
			FilesModified: []string{"~/src/main.go"},
		},
		{
			SessionID:     "session-001",
			Project:       "~/src",
			Timestamp:     1708320000000, // 2024-02-19 13:33:20
			FilesModified: []string{"~/src/README.md"},
		},
	}

	// Create test manifests
	manifests := map[string]SessionInfo{
		"session-001": {
			Name:      "readme-updates",
			Workspace: "oss",
			Project:   "~/src",
		},
		"session-002": {
			Name:      "main-refactor",
			Workspace: "oss",
			Project:   "~/src",
		},
	}

	tests := []struct {
		name         string
		targetFile   string
		opts         TraceOptions
		wantSessions int
		wantFirstID  string
		wantMods     int
	}{
		{
			name:         "single session, multiple modifications",
			targetFile:   "~/src/README.md",
			opts:         TraceOptions{},
			wantSessions: 1,
			wantFirstID:  "session-001",
			wantMods:     2,
		},
		{
			name:         "single session, single modification",
			targetFile:   "~/src/main.go",
			opts:         TraceOptions{},
			wantSessions: 1,
			wantFirstID:  "session-002",
			wantMods:     1,
		},
		{
			name:         "no match",
			targetFile:   "~/src/nonexistent.txt",
			opts:         TraceOptions{},
			wantSessions: 0,
		},
		{
			name:       "with since filter",
			targetFile: "~/src/README.md",
			opts: TraceOptions{
				Since: timePtr(time.UnixMilli(1708315000000)), // After first mod, before second
			},
			wantSessions: 1,
			wantFirstID:  "session-001",
			wantMods:     1, // Only second modification
		},
		{
			name:       "with workspace filter",
			targetFile: "~/src/README.md",
			opts: TraceOptions{
				Workspace: "oss",
			},
			wantSessions: 1,
			wantFirstID:  "session-001",
			wantMods:     2,
		},
		{
			name:       "workspace filter no match",
			targetFile: "~/src/README.md",
			opts: TraceOptions{
				Workspace: "acme",
			},
			wantSessions: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracer := NewTracer("")
			result := tracer.traceFile(tt.targetFile, entries, manifests, tt.opts)

			if len(result.Sessions) != tt.wantSessions {
				t.Errorf("got %d sessions, want %d", len(result.Sessions), tt.wantSessions)
			}

			if tt.wantSessions > 0 {
				if result.Sessions[0].SessionID != tt.wantFirstID {
					t.Errorf("first session ID = %s, want %s", result.Sessions[0].SessionID, tt.wantFirstID)
				}

				if len(result.Sessions[0].Modifications) != tt.wantMods {
					t.Errorf("got %d modifications, want %d", len(result.Sessions[0].Modifications), tt.wantMods)
				}
			}
		})
	}
}

func TestTraceFile_OrphanedSession(t *testing.T) {
	entries := []HistoryEntry{
		{
			SessionID:     "orphan-session",
			Project:       "~/src",
			Timestamp:     1708300000000,
			FilesModified: []string{"~/src/file.txt"},
		},
	}

	manifests := map[string]SessionInfo{} // Empty - no manifest for this session

	tracer := NewTracer("")
	result := tracer.traceFile("~/src/file.txt", entries, manifests, TraceOptions{})

	if len(result.Sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(result.Sessions))
	}

	if result.Sessions[0].SessionName != "<no manifest>" {
		t.Errorf("session name = %s, want <no manifest>", result.Sessions[0].SessionName)
	}

	if result.Sessions[0].SessionID != "orphan-session" {
		t.Errorf("session ID = %s, want orphan-session", result.Sessions[0].SessionID)
	}
}

func TestTraceFile_MultipleSessionsSamefile(t *testing.T) {
	entries := []HistoryEntry{
		{
			SessionID:     "session-001",
			Project:       "~/src",
			Timestamp:     1708300000000,
			FilesModified: []string{"~/src/shared.go"},
		},
		{
			SessionID:     "session-002",
			Project:       "~/src",
			Timestamp:     1708310000000,
			FilesModified: []string{"~/src/shared.go"},
		},
		{
			SessionID:     "session-003",
			Project:       "~/src",
			Timestamp:     1708320000000,
			FilesModified: []string{"~/src/shared.go"},
		},
	}

	manifests := map[string]SessionInfo{
		"session-001": {Name: "session-1"},
		"session-002": {Name: "session-2"},
		"session-003": {Name: "session-3"},
	}

	tracer := NewTracer("")
	result := tracer.traceFile("~/src/shared.go", entries, manifests, TraceOptions{})

	if len(result.Sessions) != 3 {
		t.Fatalf("got %d sessions, want 3", len(result.Sessions))
	}

	// Sessions should be ordered by first modification time
	expectedOrder := []string{"session-001", "session-002", "session-003"}
	for i, session := range result.Sessions {
		if session.SessionID != expectedOrder[i] {
			t.Errorf("session[%d] = %s, want %s", i, session.SessionID, expectedOrder[i])
		}
	}
}

func TestParseManifest(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		wantName      string
		wantWorkspace string
		wantUUID      string
		wantErr       bool
	}{
		{
			name: "valid manifest",
			content: `schema_version: "2.0"
name: test-session
workspace: oss
claude:
  uuid: test-uuid-123
context:
  project: ~/src
`,
			wantName:      "test-session",
			wantWorkspace: "oss",
			wantUUID:      "test-uuid-123",
			wantErr:       false,
		},
		{
			name: "manifest without workspace",
			content: `schema_version: "2.0"
name: test-session
claude:
  uuid: test-uuid-123
context:
  project: ~/src
`,
			wantName:      "test-session",
			wantWorkspace: "",
			wantUUID:      "test-uuid-123",
			wantErr:       false,
		},
		{
			name:    "empty manifest",
			content: ``,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tmpFile, err := os.CreateTemp(t.TempDir(), "manifest-*.yaml")
			if err != nil {
				t.Fatalf("failed to create temp file: %v", err)
			}
			defer os.Remove(tmpFile.Name())
			defer tmpFile.Close()

			// Write content
			if _, err := tmpFile.WriteString(tt.content); err != nil {
				t.Fatalf("failed to write temp file: %v", err)
			}
			tmpFile.Close()

			tracer := NewTracer("")
			info, err := tracer.parseManifest(tmpFile.Name())

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("parseManifest() error = %v", err)
			}

			if info.Name != tt.wantName {
				t.Errorf("name = %s, want %s", info.Name, tt.wantName)
			}

			if info.Workspace != tt.wantWorkspace {
				t.Errorf("workspace = %s, want %s", info.Workspace, tt.wantWorkspace)
			}

			if info.UUID != tt.wantUUID {
				t.Errorf("uuid = %s, want %s", info.UUID, tt.wantUUID)
			}
		})
	}
}

func TestNullByteResilience(t *testing.T) {
	// Create history file with null bytes
	content := bytes.NewBuffer(nil)
	content.WriteString("{\"sessionId\":\"session-001\",")
	content.WriteByte(0) // null byte
	content.WriteString("\"project\":\"/home/user\",\"timestamp\":1708300000000,\"files_modified\":[\"~/README.md\"]}\n")

	tmpFile, err := os.CreateTemp(t.TempDir(), "history-*.jsonl")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err := tmpFile.Write(content.Bytes()); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	tmpFile.Close()

	tracer := NewTracer("")
	entries, err := tracer.parseHistoryFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("parseHistoryFile() error = %v", err)
	}

	if len(entries) != 1 {
		t.Errorf("got %d entries, want 1", len(entries))
	}

	if len(entries) > 0 && entries[0].SessionID != "session-001" {
		t.Errorf("sessionId = %s, want session-001", entries[0].SessionID)
	}
}

func TestTraceFiles_Integration(t *testing.T) {
	// Create temp directory structure
	tmpDir := t.TempDir()

	// Create workspace directory
	workspaceDir := filepath.Join(tmpDir, "oss")
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		t.Fatalf("failed to create workspace dir: %v", err)
	}

	// Create history.jsonl
	historyPath := filepath.Join(workspaceDir, "history.jsonl")
	historyContent := `{"sessionId":"session-001","project":"/tmp/test-src","timestamp":1708300000000,"files_modified":["/tmp/test-src/README.md"]}
{"sessionId":"session-002","project":"/tmp/test-src","timestamp":1708310000000,"files_modified":["/tmp/test-src/main.go"]}
`
	if err := os.WriteFile(historyPath, []byte(historyContent), 0644); err != nil {
		t.Fatalf("failed to write history file: %v", err)
	}

	// Create manifest
	sessionDir := filepath.Join(workspaceDir, "session-001")
	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		t.Fatalf("failed to create session dir: %v", err)
	}

	manifestPath := filepath.Join(sessionDir, "manifest.yaml")
	manifestContent := `name: test-session
workspace: oss
claude:
  uuid: session-001
context:
  project: /tmp/test-src
`
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	// Run trace
	tracer := NewTracer(tmpDir)
	results, err := tracer.TraceFiles(TraceOptions{
		FilePaths:   []string{"/tmp/test-src/README.md"},
		SessionsDir: tmpDir,
	})

	if err != nil {
		t.Fatalf("TraceFiles() error = %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	if len(results[0].Sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(results[0].Sessions))
	}

	if results[0].Sessions[0].SessionName != "test-session" {
		t.Errorf("session name = %s, want test-session", results[0].Sessions[0].SessionName)
	}
}

// Helper function
func timePtr(t time.Time) *time.Time {
	return &t
}
