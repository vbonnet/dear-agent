package validator

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestCountClarificationMarkers(t *testing.T) {
	tests := []struct {
		name          string
		files         map[string]string
		expectedCount int
		expectedFiles []string
		expectError   bool
	}{
		{
			name: "no markers",
			files: map[string]string{
				"D1.md": "# Document\nNo markers here",
			},
			expectedCount: 0,
			expectedFiles: nil,
		},
		{
			name: "single marker in one file",
			files: map[string]string{
				"D2.md": "[NEEDS_CLARIFICATION_BY_PHASE: How should we handle edge case?]",
			},
			expectedCount: 1,
			expectedFiles: []string{"D2.md"},
		},
		{
			name: "multiple markers across files",
			files: map[string]string{
				"D3.md": "[NEEDS_CLARIFICATION_BY_PHASE: Q1]\n[NEEDS_CLARIFICATION_BY_PHASE: Q2]",
				"D4.md": "[NEEDS_CLARIFICATION_BY_PHASE: Q3]",
			},
			expectedCount: 3,
			expectedFiles: []string{"D3.md", "D4.md"},
		},
		{
			name: "strikethrough marker (false positive - acceptable)",
			files: map[string]string{
				"D5.md": "~~[NEEDS_CLARIFICATION_BY_PHASE: resolved]~~",
			},
			expectedCount: 1,
			expectedFiles: []string{"D5.md"},
		},
		{
			name: "marker in code block (false positive - acceptable)",
			files: map[string]string{
				"README.md": "```\n[NEEDS_CLARIFICATION_BY_PHASE: example]\n```",
			},
			expectedCount: 1,
			expectedFiles: []string{"README.md"},
		},
		{
			name: "no markdown files",
			files: map[string]string{
				"file.txt": "Not a markdown file",
			},
			expectedCount: 0,
			expectedFiles: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := createTempProject(t, tt.files)
			defer os.RemoveAll(dir)

			count, files, err := countClarificationMarkers(dir)

			if tt.expectError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if count != tt.expectedCount {
				t.Errorf("expected count %d, got %d", tt.expectedCount, count)
			}
			if !slicesEqual(files, tt.expectedFiles) {
				t.Errorf("expected files %v, got %v", tt.expectedFiles, files)
			}
		})
	}
}

func TestCountUncheckedAssumptions(t *testing.T) {
	tests := []struct {
		name          string
		phase         string
		files         map[string]string
		expectedCount int
		expectedFile  string
	}{
		{
			name:          "non-D4-S7 phase",
			phase:         "D2",
			files:         map[string]string{},
			expectedCount: 0,
			expectedFile:  "",
		},
		{
			name:  "D4 with all checked",
			phase: "D4",
			files: map[string]string{
				"D4-solution-requirements.md": `
## Assumption Verification Checklist
- [x] Assumption 1 verified
- [x] Assumption 2 verified
`,
			},
			expectedCount: 0,
			expectedFile:  "",
		},
		{
			name:  "D4 with unchecked items",
			phase: "D4",
			files: map[string]string{
				"D4-solution-requirements.md": `
## Assumption Verification Checklist
- [x] Assumption 1 verified
- [ ] Assumption 2 NOT verified
- [ ] Assumption 3 NOT verified
`,
			},
			expectedCount: 2,
			expectedFile:  "D4-solution-requirements.md",
		},
		{
			name:  "S7 with unchecked items",
			phase: "S7",
			files: map[string]string{
				"S7-plan.md": `
## Assumption Verification Checklist
- [ ] Verify database schema
- [x] Verify API endpoint
`,
			},
			expectedCount: 1,
			expectedFile:  "S7-plan.md",
		},
		{
			name:  "D4 file missing (graceful degradation)",
			phase: "D4",
			files: map[string]string{
				"other.md": "Some content",
			},
			expectedCount: 0,
			expectedFile:  "",
		},
		{
			name:  "D4 with no assumption section",
			phase: "D4",
			files: map[string]string{
				"D4-solution-requirements.md": `
# Requirements
No assumption section here
`,
			},
			expectedCount: 0,
			expectedFile:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := createTempProject(t, tt.files)
			defer os.RemoveAll(dir)

			count, file, err := countUncheckedAssumptions(tt.phase, dir)

			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if count != tt.expectedCount {
				t.Errorf("expected count %d, got %d", tt.expectedCount, count)
			}
			if file != tt.expectedFile {
				t.Errorf("expected file %s, got %s", tt.expectedFile, file)
			}
		})
	}
}

func TestGetPendingQuestions(t *testing.T) {
	tests := []struct {
		name          string
		stateFile     string // JSON content
		expectedCount int
		expectedPhase string
		expectError   bool
	}{
		{
			name:          "no state file (graceful degradation)",
			stateFile:     "", // no file created
			expectedCount: 0,
			expectedPhase: "",
			expectError:   false,
		},
		{
			name: "all questions answered",
			stateFile: `{
				"questions": [
					{"phase": "D2", "question": "Q1", "status": "answered"},
					{"phase": "D3", "question": "Q2", "status": "answered"}
				]
			}`,
			expectedCount: 0,
			expectedPhase: "",
		},
		{
			name: "pending questions",
			stateFile: `{
				"questions": [
					{"phase": "D2", "question": "Q1", "status": "pending"},
					{"phase": "D3", "question": "Q2", "status": "answered"},
					{"phase": "D4", "question": "Q3", "status": "pending"}
				]
			}`,
			expectedCount: 2,
			expectedPhase: "D2",
		},
		{
			name:        "malformed JSON",
			stateFile:   `{invalid json`,
			expectError: true,
		},
		{
			name: "empty questions array",
			stateFile: `{
				"questions": []
			}`,
			expectedCount: 0,
			expectedPhase: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			if tt.stateFile != "" {
				err := os.WriteFile(
					filepath.Join(dir, ".wayfinder-questions.json"),
					[]byte(tt.stateFile),
					0644,
				)
				if err != nil {
					t.Fatalf("failed to create state file: %v", err)
				}
			}

			count, phase, err := getPendingQuestions(dir)

			if tt.expectError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if count != tt.expectedCount {
				t.Errorf("expected count %d, got %d", tt.expectedCount, count)
			}
			if phase != tt.expectedPhase {
				t.Errorf("expected phase %s, got %s", tt.expectedPhase, phase)
			}
		})
	}
}

func TestValidatePhaseQuestions(t *testing.T) {
	tests := []struct {
		name        string
		phase       string
		files       map[string]string
		expectError bool
		errorReason string
	}{
		{
			name:        "validation passes - no issues",
			phase:       "D2",
			files:       map[string]string{"D2.md": "# Document"},
			expectError: false,
		},
		{
			name:  "validation fails - clarification markers",
			phase: "D3",
			files: map[string]string{
				"D3.md": "[NEEDS_CLARIFICATION_BY_PHASE: Unresolved question]",
			},
			expectError: true,
			errorReason: "unresolved clarification marker",
		},
		{
			name:  "validation fails - unchecked D4 assumptions",
			phase: "D4",
			files: map[string]string{
				"D4-solution-requirements.md": `
## Assumption Verification Checklist
- [ ] Unchecked assumption
`,
			},
			expectError: true,
			errorReason: "unchecked assumption",
		},
		{
			name:  "validation fails - pending hook questions",
			phase: "S5",
			files: map[string]string{
				".wayfinder-questions.json": `{
					"questions": [
						{"phase": "S5", "question": "Q1", "status": "pending"}
					]
				}`,
			},
			expectError: true,
			errorReason: "pending question",
		},
		{
			name:  "multiple issues - returns first (markers)",
			phase: "D4",
			files: map[string]string{
				"D4-solution-requirements.md": `
## Assumption Verification Checklist
- [ ] Unchecked
`,
				"notes.md": "[NEEDS_CLARIFICATION_BY_PHASE: Question]",
			},
			expectError: true,
			errorReason: "unresolved clarification marker",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := createTempProject(t, tt.files)
			defer os.RemoveAll(dir)

			err := validatePhaseQuestions(dir, tt.phase)

			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
					return
				}
				valErr, ok := err.(*ValidationError)
				if !ok {
					t.Errorf("expected ValidationError, got %T", err)
					return
				}
				if !strings.Contains(strings.ToLower(valErr.Reason), tt.errorReason) {
					t.Errorf("expected reason to contain %q, got %q", tt.errorReason, valErr.Reason)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

// Helper functions

// createTempProject creates a temporary directory with specified files
func createTempProject(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()

	for filename, content := range files {
		path := filepath.Join(dir, filename)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("failed to create test file %s: %v", filename, err)
		}
	}

	return dir
}

// slicesEqual compares two string slices (order-independent)
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	aCopy := make([]string, len(a))
	bCopy := make([]string, len(b))
	copy(aCopy, a)
	copy(bCopy, b)
	sort.Strings(aCopy)
	sort.Strings(bCopy)
	for i := range aCopy {
		if aCopy[i] != bCopy[i] {
			return false
		}
	}
	return true
}
