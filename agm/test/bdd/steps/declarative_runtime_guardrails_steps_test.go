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
			name:    "eval cases",
			asset:   "evals.json",
			content: `{"cases":[{"id":"trigger"}]}`,
		},
		{
			name:    "eval cases empty",
			asset:   "evals.json",
			content: `{"cases":[]}`,
			wantErr: "has no eval cases",
		},
		{
			name:  "OpenAI interface",
			asset: "openai.yaml",
			content: "interface:\n" +
				"  display_name: Research Pipeline\n" +
				"  short_description: Research and plan\n" +
				"  default_prompt: Use the research pipeline\n",
		},
		{
			name:    "OpenAI interface missing prompt",
			asset:   "openai.yaml",
			content: "interface:\n  display_name: Research Pipeline\n  short_description: Research and plan\n",
			wantErr: "lacks its published interface fields",
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
			if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
				t.Fatal(err)
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
