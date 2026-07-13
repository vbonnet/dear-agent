package mcpparity

import (
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

func TestValidateActiveCreateSessionSurfaces(t *testing.T) {
	if err := ValidateActiveCreateSessionSurfaces(); err != nil {
		t.Fatal(err)
	}
	got := ActiveCreateSessionSurfaces()
	if len(got) != len(agent.ActiveHarnesses()) {
		t.Fatalf("active MCP create-session surfaces = %d, want %d: %+v", len(got), len(agent.ActiveHarnesses()), got)
	}
}

func TestActiveHarnessesHaveCreateSessionSurface(t *testing.T) {
	for _, harness := range agent.ActiveHarnesses() {
		surface, ok := CreateSessionSurfaceFor(harness)
		if !ok {
			t.Fatalf("CreateSessionSurfaceFor(%q) not found", harness)
		}
		if surface.Harness != harness {
			t.Errorf("surface.Harness = %q, want %q", surface.Harness, harness)
		}
		if surface.Deprecated {
			t.Errorf("active harness %q marked deprecated", harness)
		}
		if surface.DefaultModel == "" {
			t.Errorf("active harness %q has empty default/fallback model", harness)
		}
	}
}

func TestDeprecatedGeminiHasCompatibilitySurface(t *testing.T) {
	surface, ok := CreateSessionSurfaceFor("gemini-cli")
	if !ok {
		t.Fatal("gemini-cli should retain deprecated MCP compatibility")
	}
	if !surface.Deprecated {
		t.Errorf("gemini-cli Deprecated = false, want true")
	}
}

func TestValidateModelIdentifierAcceptsOpenRouterIDs(t *testing.T) {
	for _, model := range []string{"z-ai/glm-5.2", "deepseek/deepseek-v4-pro", "nvidia/nemotron-3-ultra-550b-a55b", "qwen/qwen3.6-max-preview"} {
		if err := ValidateModelIdentifier("opencode-cli", model); err != nil {
			t.Errorf("ValidateModelIdentifier(opencode-cli, %q): %v", model, err)
		}
	}
}

func TestValidateModelIdentifierRejectsShellMetacharacters(t *testing.T) {
	if err := ValidateModelIdentifier("claude-code", "sonnet; rm -rf /"); err == nil {
		t.Fatal("expected unsafe model identifier to be rejected")
	}
}

func TestValidateLifecycleOperations(t *testing.T) {
	if err := ValidateLifecycleOperations(); err != nil {
		t.Fatal(err)
	}
}
