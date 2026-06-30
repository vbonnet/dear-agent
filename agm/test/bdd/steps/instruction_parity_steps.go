package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cucumber/godog"
)

type instructionParityState struct {
	file    string
	content string
	lines   []string
}

type instructionParityStateKey struct{}

// RegisterInstructionParitySteps registers BDD steps for instruction import parity.
func RegisterInstructionParitySteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, instructionParityStateKey{}, &instructionParityState{}), nil
	})

	ctx.Step(`^instruction entrypoint "([^"]*)" is configured$`, instructionEntrypointIsConfigured)
	ctx.Step(`^AGM validates instruction entrypoint parity$`, agmValidatesInstructionEntrypointParity)
	ctx.Step(`^instruction entrypoint "([^"]*)" should import "([^"]*)"$`, instructionEntrypointShouldImport)
	ctx.Step(`^instruction entrypoint "([^"]*)" should not duplicate shared policy$`, instructionEntrypointShouldNotDuplicateSharedPolicy)
}

func instructionEntrypointIsConfigured(ctx context.Context, file string) error {
	state, err := getInstructionParityState(ctx)
	if err != nil {
		return err
	}
	state.file = file
	return nil
}

func agmValidatesInstructionEntrypointParity(ctx context.Context) error {
	state, err := getInstructionParityState(ctx)
	if err != nil {
		return err
	}
	if state.file == "" {
		return fmt.Errorf("no instruction entrypoint configured")
	}
	data, err := os.ReadFile(filepath.Join(bddRepoRoot(), filepath.FromSlash(state.file)))
	if err != nil {
		return fmt.Errorf("read instruction entrypoint %q: %w", state.file, err)
	}
	state.content = string(data)
	state.lines = nonEmptyInstructionLines(state.content)
	if len(state.lines) == 0 {
		return fmt.Errorf("instruction entrypoint %q is empty", state.file)
	}
	return nil
}

func instructionEntrypointShouldImport(ctx context.Context, file, expectedImport string) error {
	state, err := getInstructionParityState(ctx)
	if err != nil {
		return err
	}
	if file != state.file {
		return fmt.Errorf("configured instruction file = %q, want %q", state.file, file)
	}
	if state.lines[0] != expectedImport {
		return fmt.Errorf("%s first non-empty line = %q, want %q", file, state.lines[0], expectedImport)
	}
	return nil
}

func instructionEntrypointShouldNotDuplicateSharedPolicy(ctx context.Context, file string) error {
	state, err := getInstructionParityState(ctx)
	if err != nil {
		return err
	}
	if file != state.file {
		return fmt.Errorf("configured instruction file = %q, want %q", state.file, file)
	}
	for _, marker := range []string{
		"Core Engineering Principles",
		"Living Documentation Policy",
		"Agent Delegation Enforcement",
		"Source Repository Policy",
	} {
		if strings.Contains(state.content, marker) {
			return fmt.Errorf("%s duplicates shared policy section %q instead of importing AGENTS.md", file, marker)
		}
	}
	return nil
}

func getInstructionParityState(ctx context.Context) (*instructionParityState, error) {
	state, ok := ctx.Value(instructionParityStateKey{}).(*instructionParityState)
	if !ok || state == nil {
		return nil, fmt.Errorf("instruction parity state not initialized")
	}
	return state, nil
}

func nonEmptyInstructionLines(content string) []string {
	var lines []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func bddRepoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}
