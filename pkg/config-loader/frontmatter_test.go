package configloader

import (
	"strings"
	"testing"
)

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		wantFrontmatter string
		wantBody        string
		wantErr         bool
		errContains     string
	}{
		{
			name: "valid frontmatter",
			input: `---
name: test
version: 1.0.0
---
Body content here.`,
			wantFrontmatter: "name: test\nversion: 1.0.0",
			wantBody:        "Body content here.",
			wantErr:         false,
		},
		{
			name: "multiline body",
			input: `---
key: value
---
Line 1
Line 2
Line 3`,
			wantFrontmatter: "key: value",
			wantBody:        "Line 1\nLine 2\nLine 3",
			wantErr:         false,
		},
		{
			name: "empty body",
			input: `---
key: value
---
`,
			wantFrontmatter: "key: value",
			wantBody:        "",
			wantErr:         false,
		},
		{
			name: "complex yaml",
			input: `---
name: complex
items:
  - one
  - two
nested:
  key: value
---
Body`,
			wantFrontmatter: "name: complex\nitems:\n  - one\n  - two\nnested:\n  key: value",
			wantBody:        "Body",
			wantErr:         false,
		},
		{
			name:        "missing frontmatter",
			input:       "Just body content",
			wantErr:     true,
			errContains: "missing frontmatter",
		},
		{
			name: "missing closing delimiter",
			input: `---
key: value
Body without closing delimiter`,
			wantErr:     true,
			errContains: "invalid frontmatter format",
		},
		{
			name:        "empty input",
			input:       "",
			wantErr:     true,
			errContains: "missing frontmatter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frontmatter, body, err := ParseFrontmatter(tt.input)

			// Check error expectation
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseFrontmatter() error = nil, want error containing %q", tt.errContains)
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ParseFrontmatter() error = %q, want error containing %q", err.Error(), tt.errContains)
				}
				return
			}

			if err != nil {
				t.Fatalf("ParseFrontmatter() unexpected error: %v", err)
			}

			// Check frontmatter
			if frontmatter != tt.wantFrontmatter {
				t.Errorf("ParseFrontmatter() frontmatter = %q, want %q", frontmatter, tt.wantFrontmatter)
			}

			// Check body
			if body != tt.wantBody {
				t.Errorf("ParseFrontmatter() body = %q, want %q", body, tt.wantBody)
			}
		})
	}
}

func TestParseFrontmatterStrict(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantErr     bool
		errContains string
	}{
		{
			name: "valid yaml",
			input: `---
name: test
version: 1.0.0
---
Body`,
			wantErr: false,
		},
		{
			name: "invalid yaml",
			input: `---
name: test
  bad indentation: true
---
Body`,
			wantErr:     true,
			errContains: "invalid YAML",
		},
		{
			name:        "missing frontmatter",
			input:       "No frontmatter",
			wantErr:     true,
			errContains: "missing frontmatter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := ParseFrontmatterStrict(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseFrontmatterStrict() error = nil, want error containing %q", tt.errContains)
					return
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ParseFrontmatterStrict() error = %q, want error containing %q", err.Error(), tt.errContains)
				}
			} else if err != nil {
				t.Errorf("ParseFrontmatterStrict() unexpected error: %v", err)
			}
		})
	}
}

func TestHasFrontmatter(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name: "has frontmatter",
			input: `---
key: value
---
Body`,
			want: true,
		},
		{
			name:  "no frontmatter",
			input: "Just content",
			want:  false,
		},
		{
			name:  "empty string",
			input: "",
			want:  false,
		},
		{
			name:  "starts with --- but no newline",
			input: "---key: value",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasFrontmatter(tt.input)
			if got != tt.want {
				t.Errorf("HasFrontmatter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseFrontmatter_Unicode(t *testing.T) {
	input := `---
name: тест
emoji: 🚀
chinese: 你好
---
Body with Unicode: 日本語`

	frontmatter, body, err := ParseFrontmatter(input)
	if err != nil {
		t.Fatalf("ParseFrontmatter() unexpected error: %v", err)
	}

	if !strings.Contains(frontmatter, "тест") {
		t.Error("Frontmatter should contain Cyrillic text")
	}
	if !strings.Contains(frontmatter, "🚀") {
		t.Error("Frontmatter should contain emoji")
	}
	if !strings.Contains(body, "日本語") {
		t.Error("Body should contain Japanese text")
	}
}

func TestParseFrontmatter_SpecialCharacters(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		wantFrontmatter string
		wantBody        string
	}{
		{
			name: "quotes and apostrophes",
			input: `---
title: "Test's \"quoted\" value"
---
Body`,
			wantFrontmatter: `title: "Test's \"quoted\" value"`,
			wantBody:        "Body",
		},
		{
			name: "special YAML characters",
			input: `---
key: "value: with: colons"
another: value|with|pipes
---
Body`,
			wantFrontmatter: `key: "value: with: colons"
another: value|with|pipes`,
			wantBody: "Body",
		},
		{
			name: "body with triple dashes",
			input: `---
key: value
---
Body content
---
More content with --- in middle`,
			wantFrontmatter: "key: value",
			wantBody:        "Body content\n---\nMore content with --- in middle",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frontmatter, body, err := ParseFrontmatter(tt.input)
			if err != nil {
				t.Fatalf("ParseFrontmatter() unexpected error: %v", err)
			}
			if frontmatter != tt.wantFrontmatter {
				t.Errorf("ParseFrontmatter() frontmatter = %q, want %q", frontmatter, tt.wantFrontmatter)
			}
			if body != tt.wantBody {
				t.Errorf("ParseFrontmatter() body = %q, want %q", body, tt.wantBody)
			}
		})
	}
}
