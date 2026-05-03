package validator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectProjectLanguage(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string // filename -> content
		wantLang string
		wantErr  bool
	}{
		{
			name: "go project with go.mod",
			files: map[string]string{
				"go.mod": "module example.com/test",
			},
			wantLang: "go",
			wantErr:  false,
		},
		{
			name: "python project with requirements.txt",
			files: map[string]string{
				"requirements.txt": "pytest==7.0.0",
			},
			wantLang: "python",
			wantErr:  false,
		},
		{
			name: "javascript project with package.json",
			files: map[string]string{
				"package.json": `{"name": "test"}`,
			},
			wantLang: "javascript",
			wantErr:  false,
		},
		{
			name: "typescript project with tsconfig.json",
			files: map[string]string{
				"tsconfig.json": `{"compilerOptions": {}}`,
			},
			wantLang: "typescript",
			wantErr:  false,
		},
		{
			name: "rust project with Cargo.toml",
			files: map[string]string{
				"Cargo.toml": "[package]\nname = \"test\"",
			},
			wantLang: "rust",
			wantErr:  false,
		},
		{
			name: "java project with pom.xml",
			files: map[string]string{
				"pom.xml": "<project></project>",
			},
			wantLang: "java",
			wantErr:  false,
		},
		{
			name: "go project by file extension",
			files: map[string]string{
				"main.go": "package main",
				"util.go": "package main",
			},
			wantLang: "go",
			wantErr:  false,
		},
		{
			name: "python project by file extension",
			files: map[string]string{
				"app.py":   "print('hello')",
				"utils.py": "def foo(): pass",
			},
			wantLang: "python",
			wantErr:  false,
		},
		{
			name:     "no recognized language",
			files:    map[string]string{},
			wantLang: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory
			tmpDir := t.TempDir()

			// Create test files
			for filename, content := range tt.files {
				path := filepath.Join(tmpDir, filename)
				if err := os.WriteFile(path, []byte(content), 0644); err != nil {
					t.Fatalf("failed to write file %s: %v", filename, err)
				}
			}

			// Test language detection
			gotLang, err := detectProjectLanguage(tmpDir)
			if (err != nil) != tt.wantErr {
				t.Errorf("detectProjectLanguage() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotLang != tt.wantLang {
				t.Errorf("detectProjectLanguage() = %v, want %v", gotLang, tt.wantLang)
			}
		})
	}
}

func TestParseTestOutput(t *testing.T) {
	tests := []struct {
		name             string
		output           string
		lang             string
		wantTestCount    int
		wantFailureCount int
	}{
		{
			name: "go tests all passing",
			output: `=== RUN   TestFoo
--- PASS: TestFoo (0.00s)
=== RUN   TestBar
--- PASS: TestBar (0.00s)
=== RUN   TestBaz
--- PASS: TestBaz (0.00s)
PASS
ok      example.com/test        0.001s`,
			lang:             "go",
			wantTestCount:    3,
			wantFailureCount: 0,
		},
		{
			name: "go tests with failures",
			output: `=== RUN   TestFoo
--- PASS: TestFoo (0.00s)
=== RUN   TestBar
--- FAIL: TestBar (0.00s)
=== RUN   TestBaz
--- PASS: TestBaz (0.00s)
FAIL
exit status 1`,
			lang:             "go",
			wantTestCount:    3,
			wantFailureCount: 1,
		},
		{
			name: "python pytest all passing",
			output: `test_app.py::test_foo PASSED
test_app.py::test_bar PASSED
test_app.py::test_baz PASSED
======================== 3 passed in 0.01s ========================`,
			lang:             "python",
			wantTestCount:    3,
			wantFailureCount: 0,
		},
		{
			name: "python pytest with failures",
			output: `test_app.py::test_foo PASSED
test_app.py::test_bar FAILED
test_app.py::test_baz PASSED
======================== 1 failed, 2 passed in 0.01s ========================`,
			lang:             "python",
			wantTestCount:    3,
			wantFailureCount: 1,
		},
		{
			name: "rust tests",
			output: `running 3 tests
test test_foo ... ok
test test_bar ... ok
test test_baz ... ok

test result: ok. 3 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out`,
			lang:             "rust",
			wantTestCount:    3,
			wantFailureCount: 0,
		},
		{
			name:             "no tests",
			output:           `No tests found`,
			lang:             "go",
			wantTestCount:    0,
			wantFailureCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTestCount, gotFailureCount := parseTestOutput(tt.output, tt.lang)
			if gotTestCount != tt.wantTestCount {
				t.Errorf("parseTestOutput() testCount = %v, want %v", gotTestCount, tt.wantTestCount)
			}
			if gotFailureCount != tt.wantFailureCount {
				t.Errorf("parseTestOutput() failureCount = %v, want %v", gotFailureCount, tt.wantFailureCount)
			}
		})
	}
}

func TestValidateCompilation_NonS8Phase(t *testing.T) {
	// validateCompilation should skip validation for non-S8 phases
	tmpDir := t.TempDir()

	// Test with different phases
	phases := []string{"W0", "D1", "D2", "D3", "D4", "S4", "S5", "S6", "S7", "S9", "S10", "S11"}
	for _, phase := range phases {
		err := validateCompilation(tmpDir, phase)
		if err != nil {
			t.Errorf("validateCompilation(%s) should skip validation for non-S8 phases, got error: %v", phase, err)
		}
	}
}

func TestValidateCompilation_NoLanguageDetected(t *testing.T) {
	// validateCompilation should skip validation if no language detected
	tmpDir := t.TempDir()

	// Create a README.md (non-code file)
	readmePath := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test"), 0644); err != nil {
		t.Fatalf("failed to write README.md: %v", err)
	}

	// Should not error because no language is detected (allows docs-only projects)
	err := validateCompilation(tmpDir, "S8")
	if err != nil {
		t.Errorf("validateCompilation() should skip validation for non-code projects, got error: %v", err)
	}
}
