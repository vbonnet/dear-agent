package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cucumber/godog"
	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

type hookParityState struct {
	harness string
	hooks   map[string][]bddHookGroup
}

type bddHookGroup struct {
	Hooks []bddHookEntry `json:"hooks"`
}

type bddHookEntry struct {
	Command string `json:"command"`
}

type bddHookSettings struct {
	Hooks map[string][]bddHookGroup `json:"hooks"`
}

type hookParityStateKey struct{}

// RegisterHookParitySteps registers BDD steps for hook harness parity.
func RegisterHookParitySteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, hookParityStateKey{}, &hookParityState{}), nil
	})

	ctx.Step(`^hook harness "([^"]*)" is configured$`, hookHarnessIsConfigured)
	ctx.Step(`^AGM validates hook parity for that harness$`, agmValidatesHookParityForThatHarness)
	ctx.Step(`^hook harness "([^"]*)" should include guardrail hook "([^"]*)"$`, hookHarnessShouldIncludeGuardrailHook)
	ctx.Step(`^hook harness "([^"]*)" should include Beads lifecycle hook "([^"]*)"$`, hookHarnessShouldIncludeBeadsLifecycleHook)
}

func hookHarnessIsConfigured(ctx context.Context, harness string) error {
	state, err := getHookParityState(ctx)
	if err != nil {
		return err
	}
	state.harness = agent.NormalizeHarnessName(harness)
	return nil
}

func agmValidatesHookParityForThatHarness(ctx context.Context) error {
	state, err := getHookParityState(ctx)
	if err != nil {
		return err
	}
	if state.harness == "" {
		return fmt.Errorf("no hook harness configured")
	}
	path, ok := hookManifestPath(state.harness)
	if !ok {
		return fmt.Errorf("harness %q has no hook manifest path", state.harness)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read hook manifest %s: %w", path, err)
	}
	var settings bddHookSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("parse hook manifest %s: %w", path, err)
	}
	state.hooks = settings.Hooks
	if len(state.hooks) == 0 {
		return fmt.Errorf("hook manifest %s has no hooks", path)
	}
	return nil
}

func hookHarnessShouldIncludeGuardrailHook(ctx context.Context, harness, guardrail string) error {
	state, err := getHookParityState(ctx)
	if err != nil {
		return err
	}
	if agent.NormalizeHarnessName(harness) != state.harness {
		return fmt.Errorf("configured hook harness = %q, want %q", state.harness, harness)
	}
	if hookCommandsContain(state.hooks, guardrail) {
		return nil
	}
	return fmt.Errorf("harness %q missing guardrail hook %q", state.harness, guardrail)
}

func hookHarnessShouldIncludeBeadsLifecycleHook(ctx context.Context, harness, event string) error {
	state, err := getHookParityState(ctx)
	if err != nil {
		return err
	}
	if agent.NormalizeHarnessName(harness) != state.harness {
		return fmt.Errorf("configured hook harness = %q, want %q", state.harness, harness)
	}
	prefix, ok := map[string]string{
		"codex-cli":    "codex",
		"agy":          "antigravity",
		"opencode-cli": "opencode",
	}[state.harness]
	if !ok {
		return fmt.Errorf("harness %q is not expected to have Beads lifecycle hooks", state.harness)
	}
	want := "bd " + prefix + "-hook " + event
	for _, group := range state.hooks[event] {
		for _, hook := range group.Hooks {
			if strings.Contains(hook.Command, want) {
				return nil
			}
		}
	}
	return fmt.Errorf("harness %q missing Beads lifecycle hook %q", state.harness, want)
}

func hookCommandsContain(hooks map[string][]bddHookGroup, substr string) bool {
	for _, groups := range hooks {
		for _, group := range groups {
			for _, hook := range group.Hooks {
				if strings.Contains(hook.Command, substr) {
					return true
				}
			}
		}
	}
	return false
}

func getHookParityState(ctx context.Context) (*hookParityState, error) {
	state, ok := ctx.Value(hookParityStateKey{}).(*hookParityState)
	if !ok || state == nil {
		return nil, fmt.Errorf("hook parity state not initialized")
	}
	return state, nil
}

func hookManifestPath(harness string) (string, bool) {
	root := hookBDDRepoRoot()
	switch harness {
	case "claude-code":
		return filepath.Join(root, ".claude", "settings.json"), true
	case "codex-cli":
		return filepath.Join(root, ".codex", "hooks.json"), true
	case "agy":
		return filepath.Join(root, ".agents", "hooks.json"), true
	case "opencode-cli":
		return filepath.Join(root, ".opencode", "hooks.json"), true
	default:
		return "", false
	}
}

func hookBDDRepoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}
