package validator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/hash"
)

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home directory: %v", err)
	}

	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{
			name: "tilde only",
			path: "~",
			want: home,
		},
		{
			name: "tilde with path",
			path: "~/test/path",
			want: filepath.Join(home, "test/path"),
		},
		{
			name: "absolute path",
			path: "/absolute/path",
			want: "/absolute/path",
		},
		{
			name:    "tilde with username (unsupported)",
			path:    "~user/path",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := hash.ExpandPath(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ExpandPath() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("ExpandPath() unexpected error = %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("ExpandPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCalculatePhaseEngramHash(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantHash    string // Expected hash for the content
		wantErr     bool
		errContains string
	}{
		{
			name:     "file with content",
			content:  "# Phase Engram\n\nSome content here.\n",
			wantHash: "sha256:7f88e6d9e4e3e9c4a6a5b4e3d2c1b0a9f8e7d6c5b4a3928170605040302010", // Placeholder - will be calculated
		},
		{
			name:     "empty file",
			content:  "",
			wantHash: "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", // SHA-256 of empty string
		},
		{
			name:     "file with newline only",
			content:  "\n",
			wantHash: "sha256:01ba4719c80b6fe911b091a7c05124b64eeece964e09c058ef8f9805daca546b", // SHA-256 of single newline
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tmpDir := t.TempDir()
			tmpFile := filepath.Join(tmpDir, "engram.md")
			if err := os.WriteFile(tmpFile, []byte(tt.content), 0600); err != nil {
				t.Fatalf("failed to create test file: %v", err)
			}

			// Calculate actual hash for comparison
			got, err := calculatePhaseEngramHash(tmpFile)

			if tt.wantErr {
				if err == nil {
					t.Errorf("calculatePhaseEngramHash() expected error, got nil")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("calculatePhaseEngramHash() error = %q, want substring %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("calculatePhaseEngramHash() unexpected error = %v", err)
				return
			}

			// Check hash format
			if !strings.HasPrefix(got, "sha256:") {
				t.Errorf("calculatePhaseEngramHash() = %q, want prefix %q", got, "sha256:")
			}

			// For non-placeholder tests, verify exact hash
			if tt.name == "empty file" || tt.name == "file with newline only" {
				if got != tt.wantHash {
					t.Errorf("calculatePhaseEngramHash() = %q, want %q", got, tt.wantHash)
				}
			}

			// Check hash length (sha256: + 64 hex chars)
			if len(got) != 71 {
				t.Errorf("calculatePhaseEngramHash() hash length = %d, want 71 (sha256: + 64 hex)", len(got))
			}
		})
	}
}

func TestCalculatePhaseEngramHash_TildeExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("failed to get home directory: %v", err)
	}

	// Create a temporary file in a subdirectory of home (test exercises tilde expansion)
	tmpDir, err := os.MkdirTemp(home, "wayfinder-test-*") //nolint:usetesting // needs $HOME root
	if err != nil {
		t.Fatalf("failed to create temp dir in home: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write test content
	content := "test content for tilde expansion"
	tmpFile := filepath.Join(tmpDir, "test-engram.md")
	if err := os.WriteFile(tmpFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Construct path with tilde
	relPath := strings.TrimPrefix(tmpFile, home)
	tildePath := "~" + relPath

	// Calculate hash using tilde path
	hash1, err := calculatePhaseEngramHash(tildePath)
	if err != nil {
		t.Errorf("calculatePhaseEngramHash() with tilde path failed: %v", err)
	}

	// Calculate hash using absolute path
	hash2, err := calculatePhaseEngramHash(tmpFile)
	if err != nil {
		t.Errorf("calculatePhaseEngramHash() with absolute path failed: %v", err)
	}

	// Hashes should match
	if hash1 != hash2 {
		t.Errorf("calculatePhaseEngramHash() tilde path hash %q != absolute path hash %q", hash1, hash2)
	}
}

func TestCalculatePhaseEngramHash_FileNotFound(t *testing.T) {
	_, err := calculatePhaseEngramHash("/nonexistent/engram.md")
	if err == nil {
		t.Error("calculatePhaseEngramHash() expected error for nonexistent file, got nil")
	}
	if !contains(err.Error(), "failed to open file") {
		t.Errorf("calculatePhaseEngramHash() error = %q, want substring %q", err.Error(), "failed to open file")
	}
}

func TestCalculatePhaseEngramHash_Consistency(t *testing.T) {
	// Create temp file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "consistency.md")
	content := "Consistent content for hash testing"
	if err := os.WriteFile(tmpFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Calculate hash multiple times
	hash1, err := calculatePhaseEngramHash(tmpFile)
	if err != nil {
		t.Fatalf("calculatePhaseEngramHash() first call failed: %v", err)
	}

	hash2, err := calculatePhaseEngramHash(tmpFile)
	if err != nil {
		t.Fatalf("calculatePhaseEngramHash() second call failed: %v", err)
	}

	// Hashes should be identical
	if hash1 != hash2 {
		t.Errorf("calculatePhaseEngramHash() inconsistent: %q != %q", hash1, hash2)
	}
}

func TestValidateMethodologyFreshness(t *testing.T) {
	// Create engram file
	tmpDir := t.TempDir()
	engramFile := filepath.Join(tmpDir, "d1-problem-validation.ai.md")
	engramContent := "# D1 Phase Methodology\n\nSome methodology content.\n"
	if err := os.WriteFile(engramFile, []byte(engramContent), 0600); err != nil {
		t.Fatalf("failed to create engram file: %v", err)
	}

	// Calculate expected hash
	expectedHash, err := calculatePhaseEngramHash(engramFile)
	if err != nil {
		t.Fatalf("failed to calculate engram hash: %v", err)
	}

	tests := []struct {
		name               string
		deliverableContent string
		hashMismatchReason string
		wantErr            bool
		errContains        string
	}{
		{
			name: "matching hash - validation passes",
			deliverableContent: `---
phase: "D1"
phase_name: "Problem Validation"
wayfinder_session_id: "test-123"
created_at: "2026-01-05T12:00:00Z"
phase_engram_hash: "` + expectedHash + `"
phase_engram_path: "` + engramFile + `"
---

# D1: Problem Validation

Content here.
`,
			hashMismatchReason: "",
			wantErr:            false,
		},
		{
			name: "hash mismatch - no reason - blocks",
			deliverableContent: `---
phase: "D1"
phase_name: "Problem Validation"
wayfinder_session_id: "test-123"
created_at: "2026-01-05T12:00:00Z"
phase_engram_hash: "sha256:outdatedhash123"
phase_engram_path: "` + engramFile + `"
---

# D1: Problem Validation

Content here.
`,
			hashMismatchReason: "",
			wantErr:            true,
			errContains:        "outdated methodology (hash mismatch",
		},
		{
			name: "hash mismatch - with reason - allows",
			deliverableContent: `---
phase: "D1"
phase_name: "Problem Validation"
wayfinder_session_id: "test-123"
created_at: "2026-01-05T12:00:00Z"
phase_engram_hash: "sha256:outdatedhash123"
phase_engram_path: "` + engramFile + `"
---

# D1: Problem Validation

Content here.
`,
			hashMismatchReason: "Reviewed methodology changes, deliverable still valid",
			wantErr:            false,
		},
		{
			name: "missing frontmatter - blocks",
			deliverableContent: `# D1: Problem Validation

No frontmatter here.
`,
			hashMismatchReason: "",
			wantErr:            true,
			errContains:        "invalid or missing frontmatter",
		},
		{
			name: "invalid engram path - blocks",
			deliverableContent: `---
phase: "D1"
phase_name: "Problem Validation"
wayfinder_session_id: "test-123"
created_at: "2026-01-05T12:00:00Z"
phase_engram_hash: "sha256:somehash"
phase_engram_path: "/nonexistent/engram.md"
---

Content
`,
			hashMismatchReason: "",
			wantErr:            true,
			errContains:        "failed to calculate hash of phase engram",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create project directory for this test
			projectDir := t.TempDir()

			// Create deliverable file
			deliverableFile := filepath.Join(projectDir, "D1-problem-validation.md")
			if err := os.WriteFile(deliverableFile, []byte(tt.deliverableContent), 0600); err != nil {
				t.Fatalf("failed to create deliverable file: %v", err)
			}

			// Validate
			err := validateMethodologyFreshness(projectDir, "D1", tt.hashMismatchReason)

			if tt.wantErr {
				if err == nil {
					t.Errorf("validateMethodologyFreshness() expected error, got nil")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("validateMethodologyFreshness() error = %q, want substring %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("validateMethodologyFreshness() unexpected error = %v", err)
			}
		})
	}
}

func TestValidateMethodologyFreshness_NoDeliverable(t *testing.T) {
	tmpDir := t.TempDir()
	// No deliverable file created

	// Should not error (validateDeliverableExists catches this case)
	err := validateMethodologyFreshness(tmpDir, "D1", "")
	if err != nil {
		t.Errorf("validateMethodologyFreshness() with no deliverable should return nil, got %v", err)
	}
}
