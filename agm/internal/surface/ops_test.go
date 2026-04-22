package surface

import (
	"testing"

	"github.com/vbonnet/dear-agent/pkg/codegen"
)

func TestRegistry_NotEmpty(t *testing.T) {
	if len(Registry) == 0 {
		t.Fatal("Registry must not be empty")
	}
}

func TestRegistry_ExpectedCount(t *testing.T) {
	const want = 7
	if got := len(Registry); got != want {
		t.Errorf("Registry length = %d, want %d", got, want)
	}
}

func TestRegistry_UniqueNames(t *testing.T) {
	seen := make(map[string]bool, len(Registry))
	for _, op := range Registry {
		if seen[op.Name] {
			t.Errorf("duplicate op name in Registry: %q", op.Name)
		}
		seen[op.Name] = true
	}
}

func TestRegistry_AllHaveRequiredFields(t *testing.T) {
	for _, op := range Registry {
		t.Run(op.Name, func(t *testing.T) {
			if op.Name == "" {
				t.Error("Name must not be empty")
			}
			if op.Description == "" {
				t.Errorf("op %q: Description must not be empty", op.Name)
			}
			if op.RequestType == "" {
				t.Errorf("op %q: RequestType must not be empty", op.Name)
			}
			if op.ResponseType == "" {
				t.Errorf("op %q: ResponseType must not be empty", op.Name)
			}
			if op.HandlerFunc == "" {
				t.Errorf("op %q: HandlerFunc must not be empty", op.Name)
			}
		})
	}
}

func TestRegistry_AllHaveValidCategory(t *testing.T) {
	for _, op := range Registry {
		t.Run(op.Name, func(t *testing.T) {
			if !op.Category.Valid() {
				t.Errorf("op %q: invalid category %q", op.Name, op.Category)
			}
		})
	}
}

func TestRegistry_AllHaveMCP(t *testing.T) {
	for _, op := range Registry {
		t.Run(op.Name, func(t *testing.T) {
			if op.MCP == nil {
				t.Errorf("op %q: MCP surface must not be nil (all ops require MCP)", op.Name)
			}
		})
	}
}

func TestRegistry_MCPToolNamesUnique(t *testing.T) {
	seen := make(map[string]bool, len(Registry))
	for _, op := range Registry {
		if op.MCP == nil {
			continue
		}
		if seen[op.MCP.ToolName] {
			t.Errorf("duplicate MCP tool name: %q (op %q)", op.MCP.ToolName, op.Name)
		}
		seen[op.MCP.ToolName] = true
	}
}

func TestRegistry_CategoryDistribution(t *testing.T) {
	counts := make(map[codegen.Category]int)
	for _, op := range Registry {
		counts[op.Category]++
	}

	// Verify we have at least one op in each expected category
	for _, cat := range []codegen.Category{codegen.CategoryRead, codegen.CategoryMutation, codegen.CategoryMeta} {
		if counts[cat] == 0 {
			t.Errorf("no ops with category %q", cat)
		}
	}
}

func TestRegistry_ContainsExpectedOps(t *testing.T) {
	expected := []string{
		"list_sessions",
		"get_session",
		"search_sessions",
		"get_status",
		"archive_session",
		"kill_session",
		"list_ops",
	}

	names := make(map[string]bool, len(Registry))
	for _, op := range Registry {
		names[op.Name] = true
	}

	for _, name := range expected {
		if !names[name] {
			t.Errorf("Registry missing expected op: %q", name)
		}
	}
}

func TestRegistry_OrderReadMutationMeta(t *testing.T) {
	// Verify registry ordering: reads first, then mutations, then meta
	var lastCategory codegen.Category
	categoryOrder := map[codegen.Category]int{
		codegen.CategoryRead:     0,
		codegen.CategoryMutation: 1,
		codegen.CategoryMeta:     2,
	}

	for i, op := range Registry {
		if i > 0 && categoryOrder[op.Category] < categoryOrder[lastCategory] {
			t.Errorf("op %q (category %q) at index %d breaks ordering after %q",
				op.Name, op.Category, i, lastCategory)
		}
		lastCategory = op.Category
	}
}
