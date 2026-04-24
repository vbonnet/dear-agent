package worktree

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectStructure(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) string
		expected RepoStructure
	}{
		{
			name: "standard git repo",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				// Standard repo has .git directory
				gitDir := filepath.Join(dir, ".git")
				if err := os.Mkdir(gitDir, 0755); err != nil {
					t.Fatalf("Failed to create .git dir: %v", err)
				}
				return dir
			},
			expected: StructureStandard,
		},
		{
			name: "bare repo structure",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				// Bare repo has .bare directory
				bareDir := filepath.Join(dir, ".bare")
				if err := os.Mkdir(bareDir, 0755); err != nil {
					t.Fatalf("Failed to create .bare dir: %v", err)
				}
				return dir
			},
			expected: StructureBare,
		},
		{
			name: "empty directory defaults to standard",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			expected: StructureStandard,
		},
		{
			name: "directory with both .git and .bare prefers bare",
			setup: func(t *testing.T) string {
				dir := t.TempDir()
				os.Mkdir(filepath.Join(dir, ".git"), 0755)
				os.Mkdir(filepath.Join(dir, ".bare"), 0755)
				return dir
			},
			expected: StructureBare,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repoPath := tt.setup(t)
			got, err := DetectStructure(repoPath)
			if err != nil {
				t.Errorf("DetectStructure() unexpected error: %v", err)
			}
			if got != tt.expected {
				t.Errorf("DetectStructure() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetWorktreeBase(t *testing.T) {
	tests := []struct {
		name      string
		structure RepoStructure
		repoPath  string
		expected  string
	}{
		{
			name:      "standard structure uses global worktree base",
			structure: StructureStandard,
			repoPath:  "/tmp/test/src/myrepo",
			expected:  expandHome("~/worktrees"),
		},
		{
			name:      "bare structure uses .bare/worktrees",
			structure: StructureBare,
			repoPath:  "/tmp/test/src/myrepo",
			expected:  "/tmp/test/src/myrepo/.bare/worktrees",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetWorktreeBase(tt.structure, tt.repoPath)
			if got != tt.expected {
				t.Errorf("GetWorktreeBase() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestExpandHome(t *testing.T) {
	home := os.Getenv("HOME")
	if home == "" {
		t.Skip("HOME environment variable not set")
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "tilde at start expands",
			input:    "~/worktrees",
			expected: filepath.Join(home, "worktrees"),
		},
		{
			name:     "lone tilde expands to home",
			input:    "~",
			expected: home,
		},
		{
			name:     "no tilde returns unchanged",
			input:    "/absolute/path",
			expected: "/absolute/path",
		},
		{
			name:     "tilde in middle returns unchanged",
			input:    "/path/~/middle",
			expected: "/path/~/middle",
		},
		{
			name:     "empty string returns unchanged",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandHome(tt.input)
			if got != tt.expected {
				t.Errorf("expandHome(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// Test detection with real filesystem setup
func TestDetectStructure_Integration(t *testing.T) {
	tmpDir := t.TempDir()

	// Test 1: Standard repo
	standardRepo := filepath.Join(tmpDir, "standard")
	if err := os.Mkdir(standardRepo, 0755); err != nil {
		t.Fatalf("Failed to create standard repo: %v", err)
	}
	os.Mkdir(filepath.Join(standardRepo, ".git"), 0755)

	structure, err := DetectStructure(standardRepo)
	if err != nil {
		t.Errorf("DetectStructure(standard) error: %v", err)
	}
	if structure != StructureStandard {
		t.Errorf("Expected StructureStandard, got %v", structure)
	}

	// Test 2: Bare repo
	bareRepo := filepath.Join(tmpDir, "bare")
	if err := os.Mkdir(bareRepo, 0755); err != nil {
		t.Fatalf("Failed to create bare repo: %v", err)
	}
	os.Mkdir(filepath.Join(bareRepo, ".bare"), 0755)

	structure, err = DetectStructure(bareRepo)
	if err != nil {
		t.Errorf("DetectStructure(bare) error: %v", err)
	}
	if structure != StructureBare {
		t.Errorf("Expected StructureBare, got %v", structure)
	}
}

// Benchmark structure detection
func BenchmarkDetectStructure(b *testing.B) {
	tmpDir := b.TempDir()
	os.Mkdir(filepath.Join(tmpDir, ".git"), 0755)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		DetectStructure(tmpDir)
	}
}

// Benchmark worktree base calculation
func BenchmarkGetWorktreeBase(b *testing.B) {
	repoPath := "/tmp/test/src/myrepo"

	b.Run("standard", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			GetWorktreeBase(StructureStandard, repoPath)
		}
	})

	b.Run("bare", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			GetWorktreeBase(StructureBare, repoPath)
		}
	})
}
