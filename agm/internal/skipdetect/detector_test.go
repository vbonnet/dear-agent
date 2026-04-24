package skipdetect

import (
	"strings"
	"testing"
)

func TestScanLines(t *testing.T) {
	tests := []struct {
		name         string
		filename     string
		content      string
		wantFindings int
		wantPatterns []string
	}{
		{
			name:         "t.Skip in test file is flagged",
			filename:     "foo_test.go",
			content:      "func TestFoo(t *testing.T) {\n\tt.Skip(\"not implemented\")\n}",
			wantFindings: 1,
			wantPatterns: []string{"t.Skip call"},
		},
		{
			name:         "t.Skip in non-test file is not flagged",
			filename:     "foo.go",
			content:      "// t.Skip(\"example\")",
			wantFindings: 0,
		},
		{
			name:         "AGM_SKIP_TEST_GATE in any file is flagged",
			filename:     "main.go",
			content:      "os.Getenv(\"AGM_SKIP_TEST_GATE\")",
			wantFindings: 1,
			wantPatterns: []string{"AGM_SKIP_TEST_GATE reference"},
		},
		{
			name:         "--force bypass suggestion is flagged",
			filename:     "guards.go",
			content:      "Suggestion: \"use --force to bypass safety guards\"",
			wantFindings: 1,
			wantPatterns: []string{"--force bypass suggestion"},
		},
		{
			name:         "legitimate skip variable not flagged",
			filename:     "main.go",
			content:      "skipCount := 0\nresult.Skipped = true",
			wantFindings: 0,
		},
		{
			name:         "--force without bypass verb not flagged",
			filename:     "cmd.go",
			content:      "fmt.Println(\"--force flag available\")",
			wantFindings: 0,
		},
		{
			name:         "multiple findings in single file",
			filename:     "bad_test.go",
			content:      "t.Skip(\"wip\")\nos.Getenv(\"AGM_SKIP_TEST_GATE\")\nuse --force to override",
			wantFindings: 3,
			wantPatterns: []string{"t.Skip call", "AGM_SKIP_TEST_GATE reference", "--force bypass suggestion"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := strings.Split(tt.content, "\n")
			findings := ScanLines(tt.filename, lines)
			if len(findings) != tt.wantFindings {
				t.Errorf("got %d findings, want %d", len(findings), tt.wantFindings)
				for _, f := range findings {
					t.Logf("  finding: %s at line %d: %s", f.Pattern, f.Line, f.Match)
				}
			}
			if tt.wantPatterns != nil {
				for i, wantP := range tt.wantPatterns {
					if i < len(findings) && findings[i].Pattern != wantP {
						t.Errorf("finding[%d] pattern = %q, want %q", i, findings[i].Pattern, wantP)
					}
				}
			}
		})
	}
}
