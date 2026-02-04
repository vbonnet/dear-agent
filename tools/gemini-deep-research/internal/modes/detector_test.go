package modes

import (
	"testing"
)

func TestDetectMode(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		explicitMode string
		want         Mode
	}{
		// Explicit mode flag tests
		{
			name:         "explicit competitive mode overrides keywords",
			input:        "analyze Go best practices",
			explicitMode: "competitive",
			want:         ModeCompetitive,
		},
		{
			name:         "explicit general mode overrides keywords",
			input:        "competitive analysis of X vs Y",
			explicitMode: "general",
			want:         ModeGeneral,
		},

		// URL detection tests
		{
			name:         "http URL triggers general mode",
			input:        "http://example.com/docs",
			explicitMode: "",
			want:         ModeGeneral,
		},
		{
			name:         "https URL triggers general mode",
			input:        "https://github.com/user/repo",
			explicitMode: "",
			want:         ModeGeneral,
		},

		// Keyword matching tests
		{
			name:         "keyword 'competitive' triggers competitive mode",
			input:        "competitive analysis of Conductor vs Engram",
			explicitMode: "",
			want:         ModeCompetitive,
		},
		{
			name:         "keyword 'vs' triggers competitive mode",
			input:        "analyze X vs Y",
			explicitMode: "",
			want:         ModeCompetitive,
		},
		{
			name:         "keyword 'compare' triggers competitive mode",
			input:        "compare React and Vue",
			explicitMode: "",
			want:         ModeCompetitive,
		},
		{
			name:         "keyword 'what can we learn' triggers competitive mode",
			input:        "what can we learn from GitHub Copilot",
			explicitMode: "",
			want:         ModeCompetitive,
		},
		{
			name:         "keyword 'gap analysis' triggers competitive mode",
			input:        "gap analysis of our tool vs competitor",
			explicitMode: "",
			want:         ModeCompetitive,
		},
		{
			name:         "keyword 'competitor' triggers competitive mode",
			input:        "analyze competitor documentation",
			explicitMode: "",
			want:         ModeCompetitive,
		},

		// Case insensitivity tests
		{
			name:         "uppercase 'COMPETITIVE' triggers competitive mode",
			input:        "COMPETITIVE analysis of tools",
			explicitMode: "",
			want:         ModeCompetitive,
		},
		{
			name:         "mixed case 'CoMpArE' triggers competitive mode",
			input:        "CoMpArE these two frameworks",
			explicitMode: "",
			want:         ModeCompetitive,
		},

		// Default general mode tests
		{
			name:         "no keywords or URL defaults to general",
			input:        "analyze Go best practices",
			explicitMode: "",
			want:         ModeGeneral,
		},
		{
			name:         "empty input defaults to general",
			input:        "",
			explicitMode: "",
			want:         ModeGeneral,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectMode(tt.input, tt.explicitMode)
			if got != tt.want {
				t.Errorf("DetectMode(%q, %q) = %v, want %v", tt.input, tt.explicitMode, got, tt.want)
			}
		})
	}
}

func TestIsURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "valid http URL",
			input: "http://example.com",
			want:  true,
		},
		{
			name:  "valid https URL",
			input: "https://github.com/user/repo",
			want:  true,
		},
		{
			name:  "URL with path",
			input: "https://docs.example.com/guide/intro",
			want:  true,
		},
		{
			name:  "not a URL - plain text",
			input: "analyze this topic",
			want:  false,
		},
		{
			name:  "not a URL - too short",
			input: "http",
			want:  false,
		},
		{
			name:  "not a URL - missing protocol",
			input: "example.com",
			want:  false,
		},
		{
			name:  "empty string",
			input: "",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isURL(tt.input)
			if got != tt.want {
				t.Errorf("isURL(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseMode(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  Mode
	}{
		{
			name:  "parse 'competitive'",
			input: "competitive",
			want:  ModeCompetitive,
		},
		{
			name:  "parse 'general'",
			input: "general",
			want:  ModeGeneral,
		},
		{
			name:  "parse uppercase 'COMPETITIVE'",
			input: "COMPETITIVE",
			want:  ModeCompetitive,
		},
		{
			name:  "parse invalid defaults to general",
			input: "invalid",
			want:  ModeGeneral,
		},
		{
			name:  "parse empty defaults to general",
			input: "",
			want:  ModeGeneral,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseMode(tt.input)
			if got != tt.want {
				t.Errorf("ParseMode(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestMode_IsCompetitive(t *testing.T) {
	tests := []struct {
		name string
		mode Mode
		want bool
	}{
		{
			name: "competitive mode returns true",
			mode: ModeCompetitive,
			want: true,
		},
		{
			name: "general mode returns false",
			mode: ModeGeneral,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.mode.IsCompetitive()
			if got != tt.want {
				t.Errorf("Mode.IsCompetitive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMode_IsGeneral(t *testing.T) {
	tests := []struct {
		name string
		mode Mode
		want bool
	}{
		{
			name: "general mode returns true",
			mode: ModeGeneral,
			want: true,
		},
		{
			name: "competitive mode returns false",
			mode: ModeCompetitive,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.mode.IsGeneral()
			if got != tt.want {
				t.Errorf("Mode.IsGeneral() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMode_String(t *testing.T) {
	tests := []struct {
		name string
		mode Mode
		want string
	}{
		{
			name: "competitive mode string",
			mode: ModeCompetitive,
			want: "competitive",
		},
		{
			name: "general mode string",
			mode: ModeGeneral,
			want: "general",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.mode.String()
			if got != tt.want {
				t.Errorf("Mode.String() = %v, want %v", got, tt.want)
			}
		})
	}
}
