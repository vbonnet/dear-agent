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

type outOfOrderGeneratedSkillRequest struct {
	Second string `json:"second" ef:"second,pos=1,required" desc:"Second value"`
	First  string `json:"first" ef:"first,pos=0,required" desc:"First value"`
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
		"GeneratedSkillRequest": reflect.TypeFor[generatedSkillRequest](),
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
	if !strings.Contains(text, "Run: `agm session search <query> --output json`") {
		t.Fatalf("generated invocation omitted binary or path:\n%s", text)
	}
	if !strings.Contains(text, "allowed-tools: Bash(agm session search *)") {
		t.Fatalf("generated permission changed governed syntax:\n%s", text)
	}
	if strings.Contains(text, "Run: `session search") {
		t.Fatalf("generated invocation retained bare command path:\n%s", text)
	}
	if strings.Contains(text, "$ARGUMENTS --output") {
		t.Fatalf("generated invocation interpolates raw arguments:\n%s", text)
	}
}

func TestGenerateSkillsSortsPositionalArgumentsByDeclaredPosition(t *testing.T) {
	op := Op{
		Name:        "ordered",
		Description: "Use ordered arguments",
		RequestType: "OutOfOrderGeneratedSkillRequest",
		CLI:         &CLISurface{CommandPath: "ordered"},
		Skill: &SkillSurface{
			SlashCommand: "agm-ordered",
			AllowedTools: "Bash(agm ordered *)",
		},
	}
	ir, err := BuildOpIR(op, map[string]reflect.Type{
		"OutOfOrderGeneratedSkillRequest": reflect.TypeFor[outOfOrderGeneratedSkillRequest](),
	}, nil)
	if err != nil {
		t.Fatalf("BuildOpIR: %v", err)
	}

	outDir := t.TempDir()
	if err := GenerateSkills([]OpIR{*ir}, outDir, "agm"); err != nil {
		t.Fatalf("GenerateSkills: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(outDir, "skills", "agm-ordered.md"))
	if err != nil {
		t.Fatalf("read generated skill: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "argument-hint: \"<first> <second>\"") {
		t.Fatalf("generated hint does not follow declared positions:\n%s", text)
	}
	if !strings.Contains(text, "Run: `agm ordered <first> <second> --output json`") {
		t.Fatalf("generated invocation does not follow declared positions:\n%s", text)
	}
}

func TestGenerateSkillsRequiresBinaryAndGovernedPermissionSyntax(t *testing.T) {
	base := OpIR{Op: Op{
		Name: "list_sessions",
		CLI:  &CLISurface{CommandPath: "session list"},
		Skill: &SkillSurface{
			SlashCommand: "agm-list",
			AllowedTools: "Bash(agm session list *)",
		},
	}}
	if err := GenerateSkills([]OpIR{base}, t.TempDir(), ""); err == nil || !strings.Contains(err.Error(), "CLI binary") {
		t.Fatalf("expected missing-binary error, got %v", err)
	}
	base.Op.Skill.AllowedTools = "Bash(agm session list:*)"
	if err := GenerateSkills([]OpIR{base}, t.TempDir(), "agm"); err == nil || !strings.Contains(err.Error(), "retired colon") {
		t.Fatalf("expected retired-permission error, got %v", err)
	}
}
