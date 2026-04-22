package surface

import (
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/codegen"
)

func TestOpDefinitions_FieldValues(t *testing.T) {
	tests := []struct {
		op           codegen.Op
		wantCategory codegen.Category
		wantReqType  string
		wantRespType string
		wantHandler  string
	}{
		{
			op:           ListSessions,
			wantCategory: codegen.CategoryRead,
			wantReqType:  "ListSessionsRequest",
			wantRespType: "ListSessionsResult",
			wantHandler:  "ListSessions",
		},
		{
			op:           GetSession,
			wantCategory: codegen.CategoryRead,
			wantReqType:  "GetSessionRequest",
			wantRespType: "GetSessionResult",
			wantHandler:  "GetSession",
		},
		{
			op:           SearchSessions,
			wantCategory: codegen.CategoryRead,
			wantReqType:  "SearchSessionsRequest",
			wantRespType: "SearchSessionsResult",
			wantHandler:  "SearchSessions",
		},
		{
			op:           GetStatus,
			wantCategory: codegen.CategoryRead,
			wantReqType:  "GetStatusRequest",
			wantRespType: "GetStatusResult",
			wantHandler:  "GetStatus",
		},
		{
			op:           ArchiveSession,
			wantCategory: codegen.CategoryMutation,
			wantReqType:  "ArchiveSessionRequest",
			wantRespType: "ArchiveSessionResult",
			wantHandler:  "ArchiveSession",
		},
		{
			op:           KillSession,
			wantCategory: codegen.CategoryMutation,
			wantReqType:  "KillSessionRequest",
			wantRespType: "KillSessionResult",
			wantHandler:  "KillSession",
		},
		{
			op:           ListOps,
			wantCategory: codegen.CategoryMeta,
			wantReqType:  "ListOpsRequest",
			wantRespType: "ListOpsResult",
			wantHandler:  "ListOps",
		},
	}

	for _, tt := range tests {
		t.Run(tt.op.Name, func(t *testing.T) {
			if tt.op.Category != tt.wantCategory {
				t.Errorf("Category = %q, want %q", tt.op.Category, tt.wantCategory)
			}
			if tt.op.RequestType != tt.wantReqType {
				t.Errorf("RequestType = %q, want %q", tt.op.RequestType, tt.wantReqType)
			}
			if tt.op.ResponseType != tt.wantRespType {
				t.Errorf("ResponseType = %q, want %q", tt.op.ResponseType, tt.wantRespType)
			}
			if tt.op.HandlerFunc != tt.wantHandler {
				t.Errorf("HandlerFunc = %q, want %q", tt.op.HandlerFunc, tt.wantHandler)
			}
		})
	}
}

func TestOpDefinitions_MCPToolNameConvention(t *testing.T) {
	for _, op := range Registry {
		t.Run(op.Name, func(t *testing.T) {
			if op.MCP == nil {
				return
			}
			if !strings.HasPrefix(op.MCP.ToolName, "agm_") {
				t.Errorf("MCP tool name %q must start with 'agm_'", op.MCP.ToolName)
			}
			if op.MCP.Description == "" {
				t.Errorf("MCP description must not be empty for op %q", op.Name)
			}
			// Tool name should be agm_ + op name
			wantToolName := "agm_" + op.Name
			if op.MCP.ToolName != wantToolName {
				t.Errorf("MCP tool name = %q, want %q", op.MCP.ToolName, wantToolName)
			}
		})
	}
}

func TestOpDefinitions_CLICommandPathConvention(t *testing.T) {
	for _, op := range Registry {
		t.Run(op.Name, func(t *testing.T) {
			if op.CLI == nil {
				return
			}
			if !strings.HasPrefix(op.CLI.CommandPath, "session ") {
				t.Errorf("CLI CommandPath %q must start with 'session '", op.CLI.CommandPath)
			}
			if op.CLI.Use == "" {
				t.Errorf("CLI Use must not be empty for op %q", op.Name)
			}
			if len(op.CLI.OutputFormats) == 0 {
				t.Errorf("CLI OutputFormats must not be empty for op %q", op.Name)
			}
		})
	}
}

func TestOpDefinitions_SkillSurfaceConsistency(t *testing.T) {
	for _, op := range Registry {
		t.Run(op.Name, func(t *testing.T) {
			if op.Skill == nil {
				return
			}
			if op.Skill.SlashCommand == "" {
				t.Errorf("Skill SlashCommand must not be empty for op %q", op.Name)
			}
			if !strings.HasPrefix(op.Skill.SlashCommand, "agm-") {
				t.Errorf("Skill SlashCommand %q must start with 'agm-'", op.Skill.SlashCommand)
			}
			if op.Skill.ActionVerb == "" {
				t.Errorf("Skill ActionVerb must not be empty for op %q", op.Name)
			}
			if len(op.Skill.OutputTable) == 0 {
				t.Errorf("Skill OutputTable must not be empty for op %q", op.Name)
			}
		})
	}
}

func TestOpDefinitions_ManualSkillOps(t *testing.T) {
	// Ops with ManualSkill=true should have a Skill surface but no auto-generated skill file
	manualSkillOps := []string{"get_session", "archive_session"}
	nameSet := make(map[string]bool)
	for _, name := range manualSkillOps {
		nameSet[name] = true
	}

	for _, op := range Registry {
		t.Run(op.Name, func(t *testing.T) {
			if nameSet[op.Name] && !op.ManualSkill {
				t.Errorf("op %q should have ManualSkill=true", op.Name)
			}
			if !nameSet[op.Name] && op.ManualSkill {
				t.Errorf("op %q has unexpected ManualSkill=true", op.Name)
			}
		})
	}
}

func TestOpDefinitions_MutationOpsHaveNoDeprecation(t *testing.T) {
	for _, op := range Registry {
		t.Run(op.Name, func(t *testing.T) {
			if op.Deprecated {
				t.Errorf("op %q is deprecated but still in Registry", op.Name)
			}
			if op.DeprecatedMsg != "" {
				t.Errorf("op %q has DeprecatedMsg but Deprecated=false", op.Name)
			}
		})
	}
}

func TestOpDefinitions_ListOpsIsMetaOnly(t *testing.T) {
	if ListOps.CLI != nil {
		t.Error("ListOps should not have a CLI surface")
	}
	if ListOps.Skill != nil {
		t.Error("ListOps should not have a Skill surface")
	}
	if ListOps.MCP == nil {
		t.Error("ListOps must have an MCP surface")
	}
}

func TestOpDefinitions_KillSessionNoSkill(t *testing.T) {
	if KillSession.Skill != nil {
		t.Error("KillSession should not have a Skill surface (too destructive)")
	}
}

func TestOpDefinitions_ListSessionsAliases(t *testing.T) {
	if ListSessions.CLI == nil {
		t.Fatal("ListSessions must have a CLI surface")
	}
	found := false
	for _, alias := range ListSessions.CLI.Aliases {
		if alias == "ls" {
			found = true
			break
		}
	}
	if !found {
		t.Error("ListSessions CLI should have 'ls' alias")
	}
}
