package codegen

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type generatedSkillRequest struct {
	Query string `json:"query" ef:"query,pos=0,required" desc:"Search query"`
}

func TestGenerateSkillsPreservesCLIBinaryAndCommandPath(t *testing.T) {
	op := Op{
		Name:        "search_sessions",
		Description: "Search sessions",
		RequestType: "GeneratedSkillRequest",
		CLI: &CLISurface{
			CommandPath: "session search",
		},
		Skill: &SkillSurface{
			SlashCommand: "agm-search",
			AllowedTools: "Bash(agm session search *)",
			ActionVerb:   "search sessions",
		},
	}
	ir, err := BuildOpIR(op, map[string]reflect.Type{
		"GeneratedSkillRequest": reflect.TypeOf(generatedSkillRequest{}),
	}, nil)
	if err != nil {
		t.Fatalf("BuildOpIR: %v", err)
	}

	outDir := t.TempDir()
	if err := GenerateSkills([]OpIR{*ir}, outDir, "agm"); err != nil {
		t.Fatalf("GenerateSkills: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(outDir, "skills", "agm-search.md"))
	if err != nil {
		t.Fatalf("read generated skill: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "Run: `agm session search --output json`") {
		t.Fatalf("generated invocation omitted binary or path:\n%s", text)
	}
	if !strings.Contains(text, "allowed-tools: Bash(agm session search *)") {
		t.Fatalf("generated permission changed governed syntax:\n%s", text)
	}
	if strings.Contains(text, "Run: `session search") {
		t.Fatalf("generated invocation retained bare command path:\n%s", text)
	}
}
