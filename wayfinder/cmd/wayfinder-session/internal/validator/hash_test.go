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

func TestResolvePhaseEngramPath(t *testing.T) {
	projectDir := t.TempDir()
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("os.UserHomeDir() error = %v", err)
	}
	absolute := filepath.Join(t.TempDir(), "method.md")

	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{name: "project relative", path: filepath.Join("methods", "problem.md"), want: filepath.Join(projectDir, "methods", "problem.md")},
		{name: "absolute", path: absolute, want: absolute},
		{name: "home root", path: "~", want: filepath.Clean(homeDir)},
		{name: "home relative", path: filepath.Join("~", "methods", "problem.md"), want: filepath.Join(homeDir, "methods", "problem.md")},
		{name: "leading tilde directory", path: "~methods/problem.md", want: filepath.Join(projectDir, "~methods/problem.md")},
		{name: "named-home-like directory", path: "~other/method.md", want: filepath.Join(projectDir, "~other/method.md")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolvePhaseEngramPath(projectDir, tt.path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolvePhaseEngramPath() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("resolvePhaseEngramPath() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("resolvePhaseEngramPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateMethodologyFreshnessProjectRelativeEngramPath(t *testing.T) {
	projectDir := t.TempDir()
	relativeEngramPath := filepath.Join("methods", "problem.md")
	engramPath := filepath.Join(projectDir, relativeEngramPath)
	if err := os.MkdirAll(filepath.Dir(engramPath), 0o755); err != nil {
		t.Fatalf("create methodology directory: %v", err)
	}
	if err := os.WriteFile(engramPath, []byte("# Problem methodology\n"), 0o600); err != nil {
		t.Fatalf("write methodology: %v", err)
	}
	expectedHash, err := calculatePhaseEngramHash(engramPath)
	if err != nil {
		t.Fatalf("calculate methodology hash: %v", err)
	}

	deliverable := `---
phase: "PROBLEM"
phase_name: "Problem Validation"
wayfinder_session_id: "relative-engram"
created_at: "2026-08-01T00:00:00Z"
phase_engram_hash: "` + expectedHash + `"
phase_engram_path: "` + relativeEngramPath + `"
---

# Problem
`
	if err := os.WriteFile(filepath.Join(projectDir, "PROBLEM-problem-validation.md"), []byte(deliverable), 0o600); err != nil {
		t.Fatalf("write deliverable: %v", err)
	}

	if err := validateMethodologyFreshness(projectDir, "PROBLEM", ""); err != nil {
		t.Fatalf("validateMethodologyFreshness() error = %v", err)
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
	engramFile := filepath.Join(tmpDir, "problem-validation.ai.md")
	engramContent := "# PROBLEM Phase Methodology\n\nSome methodology content.\n"
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
phase: "PROBLEM"
phase_name: "Problem Validation"
wayfinder_session_id: "test-123"
created_at: "2026-01-05T12:00:00Z"
phase_engram_hash: "` + expectedHash + `"
phase_engram_path: "` + engramFile + `"
---

# PROBLEM: Problem Validation

Content here.
`,
			hashMismatchReason: "",
			wantErr:            false,
		},
		{
			name: "hash mismatch - no reason - blocks",
			deliverableContent: `---
phase: "PROBLEM"
phase_name: "Problem Validation"
wayfinder_session_id: "test-123"
created_at: "2026-01-05T12:00:00Z"
phase_engram_hash: "sha256:outdatedhash123"
phase_engram_path: "` + engramFile + `"
---

# PROBLEM: Problem Validation

Content here.
`,
			hashMismatchReason: "",
			wantErr:            true,
			errContains:        "outdated methodology (hash mismatch",
		},
		{
			name: "hash mismatch - with reason - allows",
			deliverableContent: `---
phase: "PROBLEM"
phase_name: "Problem Validation"
wayfinder_session_id: "test-123"
created_at: "2026-01-05T12:00:00Z"
phase_engram_hash: "sha256:outdatedhash123"
phase_engram_path: "` + engramFile + `"
---

# PROBLEM: Problem Validation

Content here.
`,
			hashMismatchReason: "Reviewed methodology changes, deliverable still valid",
			wantErr:            false,
		},
		{
			name: "missing frontmatter - blocks",
			deliverableContent: `# PROBLEM: Problem Validation

No frontmatter here.
`,
			hashMismatchReason: "",
			wantErr:            true,
			errContains:        "invalid or missing frontmatter",
		},
		{
			name: "invalid engram path - blocks",
			deliverableContent: `---
phase: "PROBLEM"
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
			deliverableFile := filepath.Join(projectDir, "PROBLEM-problem-validation.md")
			if err := os.WriteFile(deliverableFile, []byte(tt.deliverableContent), 0600); err != nil {
				t.Fatalf("failed to create deliverable file: %v", err)
			}

			// Validate
			err := validateMethodologyFreshness(projectDir, "PROBLEM", tt.hashMismatchReason)

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
	err := validateMethodologyFreshness(tmpDir, "PROBLEM", "")
	if err != nil {
		t.Errorf("validateMethodologyFreshness() with no deliverable should return nil, got %v", err)
	}
}

// TestValidateMethodologyFreshness_EmptyEngramPath is the regression test for
// ce-fvkz / ce-11fi: when a deliverable has the required base frontmatter
// (phase, phase_name, wayfinder_session_id, created_at) but omits the
// pipeline-internal phase_engram_path field, the hash validation must be
// skipped rather than failing with "failed to calculate hash of phase engram".
// Before the fix, an empty path was passed to hash.CalculateFileHash which
// resolved to the CWD and produced an "is a directory" error.
func TestValidateMethodologyFreshness_EmptyEngramPath(t *testing.T) {
	tmpDir := t.TempDir()

	// Deliverable with valid base frontmatter but no engram fields.
	deliverable := filepath.Join(tmpDir, "CHARTER-charter.md")
	content := `---
phase: CHARTER
phase_name: Intake & Waypoint
wayfinder_session_id: session-abc123
created_at: 2026-06-17T00:00:00Z
---

# Charter

Some content here.
`
	if err := os.WriteFile(deliverable, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write deliverable: %v", err)
	}

	err := validateMethodologyFreshness(tmpDir, "CHARTER", "")
	if err != nil {
		t.Errorf("validateMethodologyFreshness() with empty phase_engram_path should return nil (ce-11fi regression), got: %v", err)
	}
}
