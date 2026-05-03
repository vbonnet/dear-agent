package history

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestEncodeDashSubstitution tests the Claude Code path encoding algorithm
func TestEncodeDashSubstitution(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple absolute path",
			input:    "/home/alice/src",
			expected: "-home-alice-src",
		},
		{
			name:     "path with spaces",
			input:    "/home/alice/My Project",
			expected: "-home-alice-My-Project",
		},
		{
			name:     "path with underscores",
			input:    "/home/alice/src_code",
			expected: "-home-alice-src-code",
		},
		{
			name:     "path with dots",
			input:    "/home/alice/project-1.0",
			expected: "-home-alice-project-1-0",
		},
		{
			name:     "path with special chars",
			input:    "/tmp/test@2026",
			expected: "-tmp-test-2026",
		},
		{
			name:     "path with multiple consecutive special chars",
			input:    "/home/alice/my...project",
			expected: "-home-alice-my---project",
		},
		{
			name:     "unicode characters",
			input:    "/home/alice/プロジェクト",
			expected: "-home-alice-------", // Each unicode char becomes one dash
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EncodeDashSubstitution(tt.input)
			if result != tt.expected {
				t.Errorf("EncodeDashSubstitution(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestHashDirectory tests the Gemini CLI hashing algorithm
func TestHashDirectory(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string // First 8 characters of hex
	}{
		{
			name:  "simple path",
			input: "/home/alice/src",
			// SHA256 of "/home/alice/src" = d2d2d6d8e4a5f6d8...
			// First 8 chars will be consistent
		},
		{
			name:  "different path produces different hash",
			input: "/home/alice/project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HashDirectory(tt.input)

			// Verify hash is 8 characters long
			if len(result) != 8 {
				t.Errorf("HashDirectory(%q) length = %d, want 8", tt.input, len(result))
			}

			// Verify hash contains only hex characters
			for _, c := range result {
				if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
					t.Errorf("HashDirectory(%q) = %q contains non-hex character %c", tt.input, result, c)
				}
			}

			// Verify consistency: same input produces same hash
			result2 := HashDirectory(tt.input)
			if result != result2 {
				t.Errorf("HashDirectory(%q) inconsistent: %q != %q", tt.input, result, result2)
			}
		})
	}

	// Test that different inputs produce different hashes
	hash1 := HashDirectory("/home/alice/src")
	hash2 := HashDirectory("/home/alice/project")
	if hash1 == hash2 {
		t.Error("Different paths produced same hash")
	}
}

// TestExtractDateFromUUID tests UUID date extraction
func TestExtractDateFromUUID(t *testing.T) {
	tests := []struct {
		name      string
		uuid      string
		wantYear  int
		wantMonth int
		wantDay   int
		wantErr   bool
	}{
		{
			name:      "rollout pattern with date",
			uuid:      "rollout-2026-03-18-abc123",
			wantYear:  2026,
			wantMonth: 3,
			wantDay:   18,
			wantErr:   false,
		},
		{
			name:      "date in middle of UUID",
			uuid:      "ses_2026-03-18_def456",
			wantYear:  2026,
			wantMonth: 3,
			wantDay:   18,
			wantErr:   false,
		},
		{
			name:      "standard UUID without date",
			uuid:      "54790b4a-1234-5678-9abc-def012345678",
			wantYear:  0,
			wantMonth: 0,
			wantDay:   0,
			wantErr:   true,
		},
		{
			name:      "empty UUID",
			uuid:      "",
			wantYear:  0,
			wantMonth: 0,
			wantDay:   0,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			year, month, day, err := ExtractDateFromUUID(tt.uuid)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ExtractDateFromUUID(%q) expected error, got nil", tt.uuid)
				}
			} else {
				if err != nil {
					t.Errorf("ExtractDateFromUUID(%q) unexpected error: %v", tt.uuid, err)
				}
				if year != tt.wantYear || month != tt.wantMonth || day != tt.wantDay {
					t.Errorf("ExtractDateFromUUID(%q) = (%d, %d, %d), want (%d, %d, %d)",
						tt.uuid, year, month, day, tt.wantYear, tt.wantMonth, tt.wantDay)
				}
			}
		})
	}
}

// TestGetClaudeCodePaths tests Claude Code path construction
func TestGetClaudeCodePaths(t *testing.T) {
	uuid := "54790b4a-1234-5678-9abc-def012345678"
	workingDir := "/home/alice/src"

	paths, metadata, err := getClaudeCodePaths(uuid, workingDir)

	if err != nil {
		t.Fatalf("getClaudeCodePaths() error = %v", err)
	}

	// Verify paths count
	if len(paths) != 2 {
		t.Errorf("getClaudeCodePaths() paths count = %d, want 2", len(paths))
	}

	// Verify primary conversation file path
	homeDir, _ := os.UserHomeDir()
	expectedPrimary := filepath.Join(homeDir, ".claude", "projects", "-home-alice-src", uuid+".jsonl")
	if paths[0] != expectedPrimary {
		t.Errorf("getClaudeCodePaths() primary path = %q, want %q", paths[0], expectedPrimary)
	}

	// Verify metadata file path
	expectedMetadata := filepath.Join(homeDir, ".claude", "projects", "-home-alice-src", "sessions-index.json")
	if paths[1] != expectedMetadata {
		t.Errorf("getClaudeCodePaths() metadata path = %q, want %q", paths[1], expectedMetadata)
	}

	// Verify metadata
	if metadata["harness"] != "claude" {
		t.Errorf("metadata[harness] = %q, want 'claude'", metadata["harness"])
	}
	if metadata["encoding_method"] != "dash-substitution" {
		t.Errorf("metadata[encoding_method] = %q, want 'dash-substitution'", metadata["encoding_method"])
	}
	if metadata["encoded_directory"] != "-home-alice-src" {
		t.Errorf("metadata[encoded_directory] = %q, want '-home-alice-src'", metadata["encoded_directory"])
	}
}

// TestGetClaudeCodePaths_MissingWorkingDir tests error when working dir missing
func TestGetClaudeCodePaths_MissingWorkingDir(t *testing.T) {
	uuid := "54790b4a-1234-5678-9abc-def012345678"

	_, _, err := getClaudeCodePaths(uuid, "")

	if err == nil {
		t.Error("getClaudeCodePaths() expected error for empty working dir, got nil")
	}

	locErr := &LocationError{}
	ok := errors.As(err, &locErr)
	if !ok {
		t.Errorf("getClaudeCodePaths() error type = %T, want *LocationError", err)
	} else {
		if locErr.Code != "WORKING_DIR_MISSING" {
			t.Errorf("LocationError.Code = %q, want 'WORKING_DIR_MISSING'", locErr.Code)
		}
	}
}

// TestGetGeminiCLIPaths tests Gemini CLI path construction
func TestGetGeminiCLIPaths(t *testing.T) {
	uuid := "ses_abc123"
	workingDir := "/home/alice/project"

	paths, metadata, err := getGeminiCLIPaths(uuid, workingDir)

	if err != nil {
		t.Fatalf("getGeminiCLIPaths() error = %v", err)
	}

	// Verify paths count
	if len(paths) != 2 {
		t.Errorf("getGeminiCLIPaths() paths count = %d, want 2", len(paths))
	}

	// Verify metadata contains hash
	hash, ok := metadata["project_hash"]
	if !ok {
		t.Error("metadata missing 'project_hash'")
	}
	if len(hash) != 8 {
		t.Errorf("metadata[project_hash] length = %d, want 8", len(hash))
	}

	// Verify paths contain hash
	homeDir, _ := os.UserHomeDir()
	expectedChats := filepath.Join(homeDir, ".gemini", "tmp", hash, "chats") + string(filepath.Separator)
	if paths[0] != expectedChats {
		t.Errorf("getGeminiCLIPaths() chats path = %q, want %q", paths[0], expectedChats)
	}

	// Verify metadata
	if metadata["harness"] != "gemini" {
		t.Errorf("metadata[harness] = %q, want 'gemini'", metadata["harness"])
	}
}

// TestGetOpenCodePaths tests OpenCode path construction
func TestGetOpenCodePaths(t *testing.T) {
	uuid := "ses_def456"

	paths, metadata, err := getOpenCodePaths(uuid)

	if err != nil {
		t.Fatalf("getOpenCodePaths() error = %v", err)
	}

	// Verify paths count
	if len(paths) != 2 {
		t.Errorf("getOpenCodePaths() paths count = %d, want 2", len(paths))
	}

	// Verify default base directory
	homeDir, _ := os.UserHomeDir()
	expectedBase := filepath.Join(homeDir, ".local", "share", "opencode")
	if metadata["base_dir"] != expectedBase {
		t.Errorf("metadata[base_dir] = %q, want %q", metadata["base_dir"], expectedBase)
	}

	// Verify metadata
	if metadata["harness"] != "opencode" {
		t.Errorf("metadata[harness] = %q, want 'opencode'", metadata["harness"])
	}
}

// TestGetOpenCodePaths_CustomDataDir tests OpenCode with custom OPENCODE_DATA_DIR
func TestGetOpenCodePaths_CustomDataDir(t *testing.T) {
	// Set custom data dir
	customDir := "/custom/opencode/path"
	os.Setenv("OPENCODE_DATA_DIR", customDir)
	defer os.Unsetenv("OPENCODE_DATA_DIR")

	uuid := "ses_ghi789"

	paths, metadata, err := getOpenCodePaths(uuid)

	if err != nil {
		t.Fatalf("getOpenCodePaths() error = %v", err)
	}

	// Verify custom base directory used
	if metadata["base_dir"] != customDir {
		t.Errorf("metadata[base_dir] = %q, want %q", metadata["base_dir"], customDir)
	}

	if metadata["env_override"] != "true" {
		t.Errorf("metadata[env_override] = %q, want 'true'", metadata["env_override"])
	}

	// Verify paths use custom directory
	expectedMessages := filepath.Join(customDir, "storage", "message", uuid) + string(filepath.Separator)
	if paths[0] != expectedMessages {
		t.Errorf("getOpenCodePaths() messages path = %q, want %q", paths[0], expectedMessages)
	}
}

// TestGetCodexPaths tests Codex path construction
func TestGetCodexPaths(t *testing.T) {
	uuid := "rollout-2026-03-18-jkl012"

	paths, metadata, err := getCodexPaths(uuid)

	if err != nil {
		t.Fatalf("getCodexPaths() error = %v", err)
	}

	// Verify paths count
	if len(paths) != 1 {
		t.Errorf("getCodexPaths() paths count = %d, want 1", len(paths))
	}

	// Verify date extraction from UUID
	if metadata["year"] != "2026" {
		t.Errorf("metadata[year] = %q, want '2026'", metadata["year"])
	}
	if metadata["month"] != "03" {
		t.Errorf("metadata[month] = %q, want '03'", metadata["month"])
	}
	if metadata["day"] != "18" {
		t.Errorf("metadata[day] = %q, want '18'", metadata["day"])
	}

	// Verify path contains date components
	homeDir, _ := os.UserHomeDir()
	expectedPath := filepath.Join(homeDir, ".codex", "sessions", "2026", "03", "18", "rollout-*.jsonl")
	if paths[0] != expectedPath {
		t.Errorf("getCodexPaths() path = %q, want %q", paths[0], expectedPath)
	}

	// Verify metadata
	if metadata["harness"] != "codex" {
		t.Errorf("metadata[harness] = %q, want 'codex'", metadata["harness"])
	}
}

// TestGetHistoryPaths tests the main GetHistoryPaths function
func TestGetHistoryPaths(t *testing.T) {
	tests := []struct {
		name        string
		agent       string
		uuid        string
		workingDir  string
		verify      bool
		wantErr     bool
		wantHarness string
	}{
		{
			name:        "Claude agent",
			agent:       "claude",
			uuid:        "54790b4a-1234-5678-9abc-def012345678",
			workingDir:  "/home/alice/src",
			verify:      false,
			wantErr:     false,
			wantHarness: "claude",
		},
		{
			name:        "Gemini agent",
			agent:       "gemini",
			uuid:        "ses_abc123",
			workingDir:  "/home/alice/project",
			verify:      false,
			wantErr:     false,
			wantHarness: "gemini",
		},
		{
			name:        "OpenCode agent",
			agent:       "opencode",
			uuid:        "ses_def456",
			workingDir:  "",
			verify:      false,
			wantErr:     false,
			wantHarness: "opencode",
		},
		{
			name:        "Codex agent",
			agent:       "codex",
			uuid:        "rollout-2026-03-18-ghi789",
			workingDir:  "",
			verify:      false,
			wantErr:     false,
			wantHarness: "codex",
		},
		{
			name:       "Unknown agent",
			agent:      "unknown",
			uuid:       "some-uuid",
			workingDir: "",
			verify:     false,
			wantErr:    true,
		},
		{
			name:       "Empty agent",
			agent:      "",
			uuid:       "some-uuid",
			workingDir: "",
			verify:     false,
			wantErr:    true,
		},
		{
			name:       "Empty UUID",
			agent:      "claude",
			uuid:       "",
			workingDir: "/home/alice/src",
			verify:     false,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := GetHistoryPaths(tt.agent, tt.uuid, tt.workingDir, tt.verify)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetHistoryPaths() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("GetHistoryPaths() unexpected error: %v", err)
				}
				if result == nil {
					t.Fatal("GetHistoryPaths() result is nil")
				}
				if result.Harness != tt.wantHarness {
					t.Errorf("GetHistoryPaths() agent = %q, want %q", result.Harness, tt.wantHarness)
				}
				if result.UUID != tt.uuid {
					t.Errorf("GetHistoryPaths() uuid = %q, want %q", result.UUID, tt.uuid)
				}
				if len(result.Paths) == 0 {
					t.Error("GetHistoryPaths() paths is empty")
				}
			}
		})
	}
}

// TestVerifyPathsExist tests path verification
func TestVerifyPathsExist(t *testing.T) {
	// Create temporary test directory
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.jsonl")
	os.WriteFile(testFile, []byte("test"), 0644)

	tests := []struct {
		name  string
		paths []string
		want  bool
	}{
		{
			name:  "existing file",
			paths: []string{testFile},
			want:  true,
		},
		{
			name:  "non-existing file",
			paths: []string{filepath.Join(tmpDir, "nonexistent.jsonl")},
			want:  false,
		},
		{
			name:  "mixed existing and non-existing",
			paths: []string{testFile, filepath.Join(tmpDir, "nonexistent.jsonl")},
			want:  false,
		},
		{
			name:  "empty paths",
			paths: []string{},
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := verifyPathsExist(tt.paths)
			if result != tt.want {
				t.Errorf("verifyPathsExist() = %v, want %v", result, tt.want)
			}
		})
	}
}
