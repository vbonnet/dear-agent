package validator

import (
	"testing"
)

func TestIsPhaseForbiddenForCode(t *testing.T) {
	tests := []struct {
		name      string
		phaseName string
		want      bool
	}{
		// Forbidden phases (planning)
		{"PROBLEM is forbidden", "PROBLEM", true},
		{"RESEARCH is forbidden", "RESEARCH", true},
		{"DESIGN is forbidden", "DESIGN", true},
		{"SPEC is forbidden", "SPEC", true},
		{"PLAN is forbidden", "PLAN", true},
		{"SETUP is forbidden", "SETUP", true},

		// Allowed phases (implementation/deployment)
		{"CHARTER is allowed", "CHARTER", false},
		{"BUILD is allowed", "BUILD", false},
		{"RETRO is allowed", "RETRO", false},

		// Unknown phase (treat as allowed)
		{"Unknown phase is allowed", "X1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPhaseForbiddenForCode(tt.phaseName)
			if got != tt.want {
				t.Errorf("isPhaseForbiddenForCode(%q) = %v, want %v", tt.phaseName, got, tt.want)
			}
		})
	}
}

func TestFormatViolatingFiles(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		want  string
	}{
		{
			name:  "Empty list",
			files: []string{},
			want:  "",
		},
		{
			name:  "Single file",
			files: []string{"file1.go"},
			want:  "file1.go",
		},
		{
			name:  "Three files",
			files: []string{"file1.go", "file2.py", "file3.js"},
			want:  "file1.go, file2.py, file3.js",
		},
		{
			name:  "Five files (at threshold)",
			files: []string{"file1.go", "file2.py", "file3.js", "file4.ts", "file5.rb"},
			want:  "file1.go, file2.py, file3.js, file4.ts, file5.rb",
		},
		{
			name:  "Six files (truncated)",
			files: []string{"file1.go", "file2.py", "file3.js", "file4.ts", "file5.rb", "file6.php"},
			want:  "file1.go, file2.py, file3.js, file4.ts, file5.rb ...and 1 more",
		},
		{
			name:  "Ten files (truncated)",
			files: []string{"f1.go", "f2.py", "f3.js", "f4.ts", "f5.rb", "f6.php", "f7.c", "f8.cpp", "f9.java", "f10.rs"},
			want:  "f1.go, f2.py, f3.js, f4.ts, f5.rb ...and 5 more",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatViolatingFiles(tt.files)
			if got != tt.want {
				t.Errorf("formatViolatingFiles() = %q, want %q", got, tt.want)
			}
		})
	}
}
