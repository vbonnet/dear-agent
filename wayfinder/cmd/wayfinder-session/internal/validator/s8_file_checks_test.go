package validator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanForRedFlags(t *testing.T) {
	tests := []struct {
		name        string
		files       map[string]string // filename -> content
		wantErr     bool
		errContains string
	}{
		{
			name: "no red flags",
			files: map[string]string{
				"README.md":       "# Project\n\nThis is implementation.",
				"ARCHITECTURE.md": "# Architecture\n\nWe implemented the feature.",
			},
			wantErr: false,
		},
		{
			name: "red flag - would implement",
			files: map[string]string{
				"S8-implementation.md": "# Implementation\n\nThis would implement the feature.",
			},
			wantErr:     true,
			errContains: "Design document detected",
		},
		{
			name: "red flag - demonstration",
			files: map[string]string{
				"DEMO.md": "This is a demonstration of the concept.",
			},
			wantErr:     true,
			errContains: "Design document detected",
		},
		{
			name: "red flag - blueprint",
			files: map[string]string{
				"PLAN.md": "This blueprint shows the design.",
			},
			wantErr:     true,
			errContains: "Design document detected",
		},
		{
			name: "red flag - conceptual",
			files: map[string]string{
				"DESIGN.md": "This is a conceptual design.",
			},
			wantErr:     true,
			errContains: "Design document detected",
		},
		{
			name: "red flag - ready for implementation",
			files: map[string]string{
				"TODO.md": "The design is ready for implementation.",
			},
			wantErr:     true,
			errContains: "Design document detected",
		},
		{
			name: "red flag - what would be",
			files: map[string]string{
				"SPEC.md": "This shows what would be built.",
			},
			wantErr:     true,
			errContains: "Design document detected",
		},
		{
			name: "red flag - example implementation",
			files: map[string]string{
				"EXAMPLE.md": "Here's an example implementation.",
			},
			wantErr:     true,
			errContains: "Design document detected",
		},
		{
			name: "red flag - design phase",
			files: map[string]string{
				"STATUS.md": "We're in the design phase.",
			},
			wantErr:     true,
			errContains: "Design document detected",
		},
		{
			name: "red flag - placeholder",
			files: map[string]string{
				"CODE.md": "This is a placeholder for the code.",
			},
			wantErr:     true,
			errContains: "Design document detected",
		},
		{
			name: "red flag - to be implemented",
			files: map[string]string{
				"FUTURE.md": "This feature is to be implemented later.",
			},
			wantErr:     true,
			errContains: "Design document detected",
		},
		{
			name: "red flag - case insensitive",
			files: map[string]string{
				"NOTES.md": "This WOULD IMPLEMENT the feature.",
			},
			wantErr:     true,
			errContains: "Design document detected",
		},
		{
			name: "no markdown files",
			files: map[string]string{
				"main.go": "package main\n\nfunc main() {}",
			},
			wantErr: false,
		},
		{
			name:    "empty project",
			files:   map[string]string{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory with test files
			tmpDir := t.TempDir()
			for filename, content := range tt.files {
				path := filepath.Join(tmpDir, filename)
				if err := os.WriteFile(path, []byte(content), 0600); err != nil {
					t.Fatalf("failed to create test file %s: %v", filename, err)
				}
			}

			err := scanForRedFlags(tmpDir)

			if tt.wantErr {
				if err == nil {
					t.Errorf("scanForRedFlags() expected error, got nil")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("scanForRedFlags() error = %q, want substring %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("scanForRedFlags() unexpected error = %v", err)
			}
		})
	}
}

func TestCheckArtifact(t *testing.T) {
	tests := []struct {
		name       string
		artifact   string
		createFile bool
		wantExists bool
		wantErr    bool
	}{
		{
			name:       "relative path - exists",
			artifact:   "bin/app",
			createFile: true,
			wantExists: true,
			wantErr:    false,
		},
		{
			name:       "relative path - missing",
			artifact:   "bin/missing",
			createFile: false,
			wantExists: false,
			wantErr:    false,
		},
		{
			name:       "absolute path - exists",
			artifact:   "", // Will be set to tmpDir/bin/app in test
			createFile: true,
			wantExists: true,
			wantErr:    false,
		},
		{
			name:       "nested path - exists",
			artifact:   "dist/output/final.bin",
			createFile: true,
			wantExists: true,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			artifact := tt.artifact
			var filePath string

			// Handle absolute path test case
			if artifact == "" {
				artifact = filepath.Join(tmpDir, "bin", "app")
				filePath = artifact
			} else {
				filePath = filepath.Join(tmpDir, artifact)
			}

			// Create file if needed
			if tt.createFile {
				dir := filepath.Dir(filePath)
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatalf("failed to create directory: %v", err)
				}
				if err := os.WriteFile(filePath, []byte("content"), 0600); err != nil {
					t.Fatalf("failed to create test file: %v", err)
				}
			}

			exists, err := checkArtifact(tmpDir, artifact)

			if tt.wantErr {
				if err == nil {
					t.Errorf("checkArtifact() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("checkArtifact() unexpected error = %v", err)
				return
			}

			if exists != tt.wantExists {
				t.Errorf("checkArtifact() exists = %v, want %v", exists, tt.wantExists)
			}
		})
	}
}

func TestValidateArtifactPath(t *testing.T) {
	tests := []struct {
		name        string
		artifact    string
		wantErr     bool
		errContains string
	}{
		{
			name:     "relative path - safe",
			artifact: "bin/app",
			wantErr:  false,
		},
		{
			name:     "relative nested path - safe",
			artifact: "dist/output/final.bin",
			wantErr:  false,
		},
		{
			name:     "absolute path - allowed",
			artifact: "/tmp/build/app",
			wantErr:  false,
		},
		{
			name:        "path traversal - parent directory",
			artifact:    "../etc/passwd",
			wantErr:     true,
			errContains: "path traversal detected",
		},
		{
			name:        "path traversal - multiple levels",
			artifact:    "../../etc/passwd",
			wantErr:     true,
			errContains: "path traversal detected",
		},
		{
			name:        "path traversal - mixed with valid path",
			artifact:    "bin/../../etc/passwd",
			wantErr:     true,
			errContains: "path traversal detected",
		},
		{
			name:     "dot in filename - safe",
			artifact: "bin/app.exe",
			wantErr:  false,
		},
		{
			name:     "current directory - safe",
			artifact: "./bin/app",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			err := validateArtifactPath(tmpDir, tt.artifact)

			if tt.wantErr {
				if err == nil {
					t.Errorf("validateArtifactPath() expected error, got nil")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("validateArtifactPath() error = %q, want substring %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("validateArtifactPath() unexpected error = %v", err)
			}
		})
	}
}

func TestVerifyBuildArtifacts(t *testing.T) {
	tests := []struct {
		name        string
		artifacts   []string
		createFiles []string
		wantErr     bool
		errContains string
	}{
		{
			name:        "all artifacts exist",
			artifacts:   []string{"bin/app", "dist/config.yaml"},
			createFiles: []string{"bin/app", "dist/config.yaml"},
			wantErr:     false,
		},
		{
			name:        "one artifact missing",
			artifacts:   []string{"bin/app", "dist/config.yaml"},
			createFiles: []string{"bin/app"},
			wantErr:     true,
			errContains: "Build artifacts missing",
		},
		{
			name:        "all artifacts missing",
			artifacts:   []string{"bin/app", "dist/config.yaml"},
			createFiles: []string{},
			wantErr:     true,
			errContains: "Build artifacts missing",
		},
		{
			name:        "empty artifact list",
			artifacts:   []string{},
			createFiles: []string{},
			wantErr:     false,
		},
		{
			name:        "nil artifact list",
			artifacts:   nil,
			createFiles: []string{},
			wantErr:     false,
		},
		{
			name:        "path traversal in artifacts",
			artifacts:   []string{"../../etc/passwd"},
			createFiles: []string{},
			wantErr:     true,
			errContains: "Path traversal detected",
		},
		{
			name:        "mixed - some exist, some missing",
			artifacts:   []string{"bin/app", "dist/missing.yaml", "lib/util.so"},
			createFiles: []string{"bin/app", "lib/util.so"},
			wantErr:     true,
			errContains: "Build artifacts missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// Create specified files
			for _, file := range tt.createFiles {
				path := filepath.Join(tmpDir, file)
				dir := filepath.Dir(path)
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatalf("failed to create directory: %v", err)
				}
				if err := os.WriteFile(path, []byte("content"), 0600); err != nil {
					t.Fatalf("failed to create test file: %v", err)
				}
			}

			err := verifyBuildArtifacts(tmpDir, tt.artifacts)

			if tt.wantErr {
				if err == nil {
					t.Errorf("verifyBuildArtifacts() expected error, got nil")
				} else if tt.errContains != "" && !contains(err.Error(), tt.errContains) {
					t.Errorf("verifyBuildArtifacts() error = %q, want substring %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("verifyBuildArtifacts() unexpected error = %v", err)
			}
		})
	}
}
