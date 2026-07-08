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

const engramKnowledgeFeaturePath = "agm/test/bdd/features/engram_knowledge_guardrails.feature"

type engramKnowledgeGuardrailState struct {
	pkg      string
	spec     string
	repoRoot string
}

type engramKnowledgeGuardrailStateKey struct{}

// RegisterEngramKnowledgeGuardrailSteps registers BDD steps for Engram knowledge packages.
func RegisterEngramKnowledgeGuardrailSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, engramKnowledgeGuardrailStateKey{}, &engramKnowledgeGuardrailState{
			repoRoot: engramKnowledgeBDDRepoRoot(),
		}), nil
	})

	ctx.Step(`^Engram knowledge package "([^"]*)" is configured$`, engramKnowledgePackageIsConfigured)
	ctx.Step(`^AGM validates Engram knowledge package coverage$`, agmValidatesEngramKnowledgePackageCoverage)
	ctx.Step(`^Engram knowledge package "([^"]*)" should have a co-located SPEC$`, engramKnowledgePackageShouldHaveCoLocatedSPEC)
}

func engramKnowledgePackageIsConfigured(ctx context.Context, pkg string) error {
	state, err := getEngramKnowledgeGuardrailState(ctx)
	if err != nil {
		return err
	}
	state.pkg = pkg
	state.spec = filepath.Join(state.repoRoot, filepath.FromSlash(pkg), "SPEC.md")
	return nil
}

func agmValidatesEngramKnowledgePackageCoverage(ctx context.Context) error {
	state, err := getEngramKnowledgeGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.spec == "" {
		return fmt.Errorf("no engram knowledge package configured")
	}
	data, err := os.ReadFile(state.spec)
	if err != nil {
		return fmt.Errorf("engram knowledge package SPEC %s: %w", state.spec, err)
	}
	if !strings.Contains(string(data), engramKnowledgeFeaturePath) {
		return fmt.Errorf("engram knowledge package SPEC %s does not reference %s", state.spec, engramKnowledgeFeaturePath)
	}
	return nil
}

func engramKnowledgePackageShouldHaveCoLocatedSPEC(ctx context.Context, pkg string) error {
	state, err := getEngramKnowledgeGuardrailState(ctx)
	if err != nil {
		return err
	}
	if pkg != state.pkg {
		return fmt.Errorf("configured engram knowledge package = %q, want %q", state.pkg, pkg)
	}
	wantSuffix := filepath.Join(filepath.FromSlash(pkg), "SPEC.md")
	if !strings.HasSuffix(state.spec, wantSuffix) {
		return fmt.Errorf("engram knowledge package SPEC = %q, want suffix %q", state.spec, wantSuffix)
	}
	return nil
}

func getEngramKnowledgeGuardrailState(ctx context.Context) (*engramKnowledgeGuardrailState, error) {
	state, ok := ctx.Value(engramKnowledgeGuardrailStateKey{}).(*engramKnowledgeGuardrailState)
	if !ok || state == nil {
		return nil, fmt.Errorf("engram knowledge guardrail state not initialized")
	}
	return state, nil
}

func engramKnowledgeBDDRepoRoot() string {
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
