package steps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateDeclarativeRuntimeAsset(t *testing.T) {
	tests := []struct {
		name    string
		asset   string
		content string
		wantErr string
	}{
		{
			name:    "plugin manifest",
			asset:   "plugin.json",
			content: `{"name":"research-pipeline","version":"0.1.0","skills":["./skills/"]}`,
		},
		{
			name:    "plugin manifest missing contract",
			asset:   "plugin.json",
			content: `{"name":"research-pipeline"}`,
			wantErr: "lacks name, version, or skills",
		},
		{
			name:    "plugin manifest missing canonical skill",
			asset:   "plugin.json",
			content: `{"name":"research-pipeline","version":"0.1.0","skills":["./missing/"]}`,
			wantErr: "does not contain canonical research-pipeline/SKILL.md",
		},
		{
			name:    "eval cases",
			asset:   "evals.json",
			content: `{"cases":[{"id":"trigger","prompt":"research this","harness":["codex"],"should_trigger":true,"trials":1,"expected_checks":[{"type":"regex","target":"trace","pattern":"research"}]}]}`,
		},
		{
			name:    "eval cases empty",
			asset:   "evals.json",
			content: `{"cases":[]}`,
			wantErr: "has no eval cases",
		},
		{
			name:    "eval case missing schema",
			asset:   "evals.json",
			content: `{"cases":[{}]}`,
			wantErr: "lacks required fields",
		},
		{
			name:    "eval check missing schema",
			asset:   "evals.json",
			content: `{"cases":[{"id":"trigger","prompt":"research this","harness":["codex"],"should_trigger":true,"trials":1,"expected_checks":[{}]}]}`,
			wantErr: "check 0 lacks required fields",
		},
		{
			name:  "OpenAI interface",
			asset: "openai.yaml",
			content: "interface:\n" +
				"  display_name: Research Pipeline\n" +
				"  short_description: Research, verify, plan, and decompose\n" +
				"  default_prompt: Use the research pipeline\n",
		},
		{
			name:    "OpenAI interface missing prompt",
			asset:   "openai.yaml",
			content: "interface:\n  display_name: Research Pipeline\n  short_description: Research and plan\n",
			wantErr: "lacks its published interface fields",
		},
		{
			name:  "OpenAI interface short description too short",
			asset: "openai.yaml",
			content: "interface:\n" +
				"  display_name: Research Pipeline\n" +
				"  short_description: Research and plan\n" +
				"  default_prompt: Use the research pipeline\n",
			wantErr: "must contain 25-64 characters",
		},
		{
			name:  "OpenAI interface short description too long",
			asset: "openai.yaml",
			content: "interface:\n" +
				"  display_name: Research Pipeline\n" +
				"  short_description: " + strings.Repeat("x", 65) + "\n" +
				"  default_prompt: Use the research pipeline\n",
			wantErr: "must contain 25-64 characters",
		},
		{
			name:    "skill",
			asset:   "SKILL.md",
			content: "# Research Pipeline\n",
		},
		{
			name:    "empty skill",
			asset:   "SKILL.md",
			content: " \n",
			wantErr: "is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			dir := "published"
			if tt.asset == "plugin.json" {
				dir = "published/.claude-plugin"
			}
			if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
				t.Fatal(err)
			}
			if tt.name == "plugin manifest" {
				skillDir := filepath.Join(root, "published", "skills", "research-pipeline")
				if err := os.MkdirAll(skillDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Research Pipeline\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(root, dir, tt.asset), []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}

			err := validateDeclarativeRuntimeAsset(root, dir, tt.asset)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateDeclarativeRuntimeAsset() error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateDeclarativeRuntimeAsset() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}
