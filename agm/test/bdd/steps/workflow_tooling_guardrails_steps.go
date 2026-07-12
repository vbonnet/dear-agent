package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

type workflowToolingGuardrailState struct {
	command     string
	commandSpec string
}

type workflowToolingGuardrailStateKey struct{}

// RegisterWorkflowToolingGuardrailSteps registers BDD steps for workflow tooling commands.
func RegisterWorkflowToolingGuardrailSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, workflowToolingGuardrailStateKey{}, &workflowToolingGuardrailState{}), nil
	})

	ctx.Step(`^workflow tooling command "([^"]*)" is configured$`, workflowToolingCommandIsConfigured)
	ctx.Step(`^AGM validates workflow tooling command coverage$`, agmValidatesWorkflowToolingCommandCoverage)
	ctx.Step(`^workflow tooling command "([^"]*)" should have a co-located SPEC$`, workflowToolingCommandShouldHaveCoLocatedSPEC)
}

func workflowToolingCommandIsConfigured(ctx context.Context, command string) error {
	state, err := getWorkflowToolingGuardrailState(ctx)
	if err != nil {
		return err
	}
	state.command = command
	state.commandSpec = filepath.Join(workflowToolingBDDRepoRoot(), "cmd", command, "SPEC.md")
	return nil
}

func agmValidatesWorkflowToolingCommandCoverage(ctx context.Context) error {
	state, err := getWorkflowToolingGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.commandSpec == "" {
		return fmt.Errorf("no workflow tooling command configured")
	}
	if _, err := os.Stat(state.commandSpec); err != nil {
		return fmt.Errorf("workflow tooling command SPEC %s: %w", state.commandSpec, err)
	}
	return nil
}

func workflowToolingCommandShouldHaveCoLocatedSPEC(ctx context.Context, command string) error {
	state, err := getWorkflowToolingGuardrailState(ctx)
	if err != nil {
		return err
	}
	if command != state.command {
		return fmt.Errorf("configured workflow tooling command = %q, want %q", state.command, command)
	}
	wantSuffix := filepath.Join("cmd", command, "SPEC.md")
	if !strings.HasSuffix(state.commandSpec, wantSuffix) {
		return fmt.Errorf("workflow tooling command SPEC = %q, want suffix %q", state.commandSpec, wantSuffix)
	}
	return nil
}

func getWorkflowToolingGuardrailState(ctx context.Context) (*workflowToolingGuardrailState, error) {
	state, ok := ctx.Value(workflowToolingGuardrailStateKey{}).(*workflowToolingGuardrailState)
	if !ok || state == nil {
		return nil, fmt.Errorf("workflow tooling guardrail state not initialized")
	}
	return state, nil
}

func workflowToolingBDDRepoRoot() string {
	return packageSpecBDDRepoRoot()
}
