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

type localDevGuardrailState struct {
	command     string
	commandSpec string
	library     string
	librarySpec string
}

type localDevGuardrailStateKey struct{}

// RegisterLocalDevelopmentGuardrailSteps registers BDD steps for audited local development wrappers.
func RegisterLocalDevelopmentGuardrailSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, localDevGuardrailStateKey{}, &localDevGuardrailState{}), nil
	})

	ctx.Step(`^safe local development command "([^"]*)" is configured$`, safeLocalDevelopmentCommandIsConfigured)
	ctx.Step(`^AGM validates safe local development command coverage$`, agmValidatesSafeLocalDevelopmentCommandCoverage)
	ctx.Step(`^safe local development command "([^"]*)" should have a co-located SPEC$`, safeLocalDevelopmentCommandShouldHaveCoLocatedSPEC)
	ctx.Step(`^safe local development library "([^"]*)" is configured$`, safeLocalDevelopmentLibraryIsConfigured)
	ctx.Step(`^AGM validates safe local development library coverage$`, agmValidatesSafeLocalDevelopmentLibraryCoverage)
	ctx.Step(`^safe local development library "([^"]*)" should have a co-located SPEC$`, safeLocalDevelopmentLibraryShouldHaveCoLocatedSPEC)
}

func safeLocalDevelopmentCommandIsConfigured(ctx context.Context, command string) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	state.command = command
	state.commandSpec = filepath.Join(localDevBDDRepoRoot(), "cmd", command, "SPEC.md")
	return nil
}

func agmValidatesSafeLocalDevelopmentCommandCoverage(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.commandSpec == "" {
		return fmt.Errorf("no safe local development command configured")
	}
	if _, err := os.Stat(state.commandSpec); err != nil {
		return fmt.Errorf("safe local development command SPEC %s: %w", state.commandSpec, err)
	}
	return nil
}

func safeLocalDevelopmentCommandShouldHaveCoLocatedSPEC(ctx context.Context, command string) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if command != state.command {
		return fmt.Errorf("configured safe local development command = %q, want %q", state.command, command)
	}
	wantSuffix := filepath.Join("cmd", command, "SPEC.md")
	if !strings.HasSuffix(state.commandSpec, wantSuffix) {
		return fmt.Errorf("safe local development command SPEC = %q, want suffix %q", state.commandSpec, wantSuffix)
	}
	return nil
}

func safeLocalDevelopmentLibraryIsConfigured(ctx context.Context, pkg string) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	state.library = pkg
	state.librarySpec = filepath.Join(localDevBDDRepoRoot(), "internal", pkg, "SPEC.md")
	return nil
}

func agmValidatesSafeLocalDevelopmentLibraryCoverage(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.librarySpec == "" {
		return fmt.Errorf("no safe local development library configured")
	}
	if _, err := os.Stat(state.librarySpec); err != nil {
		return fmt.Errorf("safe local development library SPEC %s: %w", state.librarySpec, err)
	}
	return nil
}

func safeLocalDevelopmentLibraryShouldHaveCoLocatedSPEC(ctx context.Context, pkg string) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if pkg != state.library {
		return fmt.Errorf("configured safe local development library = %q, want %q", state.library, pkg)
	}
	wantSuffix := filepath.Join("internal", pkg, "SPEC.md")
	if !strings.HasSuffix(state.librarySpec, wantSuffix) {
		return fmt.Errorf("safe local development library SPEC = %q, want suffix %q", state.librarySpec, wantSuffix)
	}
	return nil
}

func getLocalDevGuardrailState(ctx context.Context) (*localDevGuardrailState, error) {
	state, ok := ctx.Value(localDevGuardrailStateKey{}).(*localDevGuardrailState)
	if !ok || state == nil {
		return nil, fmt.Errorf("local development guardrail state not initialized")
	}
	return state, nil
}

func localDevBDDRepoRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}
