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

type packageSpecGuardrailConfig struct {
	stateKey          any
	label             string
	featurePath       string
	configuredPattern string
	validatePattern   string
	colocatedPattern  string
}

type packageSpecGuardrailState struct {
	pkg      string
	spec     string
	repoRoot string
}

func registerPackageSpecGuardrailSteps(ctx *godog.ScenarioContext, cfg packageSpecGuardrailConfig) {
	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, cfg.stateKey, &packageSpecGuardrailState{
			repoRoot: packageSpecBDDRepoRoot(),
		}), nil
	})

	ctx.Step(cfg.configuredPattern, func(ctx context.Context, pkg string) error {
		state, err := getPackageSpecGuardrailState(ctx, cfg)
		if err != nil {
			return err
		}
		state.pkg = pkg
		state.spec = filepath.Join(state.repoRoot, filepath.FromSlash(pkg), "SPEC.md")
		return nil
	})
	ctx.Step(cfg.validatePattern, func(ctx context.Context) error {
		state, err := getPackageSpecGuardrailState(ctx, cfg)
		if err != nil {
			return err
		}
		if state.spec == "" {
			return fmt.Errorf("no %s configured", cfg.label)
		}
		data, err := os.ReadFile(state.spec)
		if err != nil {
			return fmt.Errorf("%s SPEC %s: %w", cfg.label, state.spec, err)
		}
		if !strings.Contains(string(data), cfg.featurePath) {
			return fmt.Errorf("%s SPEC %s does not reference %s", cfg.label, state.spec, cfg.featurePath)
		}
		return nil
	})
	ctx.Step(cfg.colocatedPattern, func(ctx context.Context, pkg string) error {
		state, err := getPackageSpecGuardrailState(ctx, cfg)
		if err != nil {
			return err
		}
		if pkg != state.pkg {
			return fmt.Errorf("configured %s = %q, want %q", cfg.label, state.pkg, pkg)
		}
		wantSuffix := filepath.Join(filepath.FromSlash(pkg), "SPEC.md")
		if !strings.HasSuffix(state.spec, wantSuffix) {
			return fmt.Errorf("%s SPEC = %q, want suffix %q", cfg.label, state.spec, wantSuffix)
		}
		return nil
	})
}

func getPackageSpecGuardrailState(ctx context.Context, cfg packageSpecGuardrailConfig) (*packageSpecGuardrailState, error) {
	state, ok := ctx.Value(cfg.stateKey).(*packageSpecGuardrailState)
	if !ok || state == nil {
		return nil, fmt.Errorf("%s guardrail state not initialized", cfg.label)
	}
	return state, nil
}

func packageSpecBDDRepoRoot() string {
	if dir, err := os.Getwd(); err == nil {
		for {
			if _, err := os.Stat(filepath.Join(dir, "engram")); err == nil {
				if _, err := os.Stat(filepath.Join(dir, "agm")); err == nil {
					return dir
				}
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}
