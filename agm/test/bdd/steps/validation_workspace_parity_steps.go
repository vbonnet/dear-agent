package steps

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
)

const validationWorkspaceParityFeaturePath = "agm/test/bdd/features/validation_workspace_parity.feature"

type validationWorkspacePackageStateKey struct{}
type validationWorkspaceRouteStateKey struct{}

type validationWorkspaceRouteState struct {
	harness   string
	family    string
	validated bool
}

// RegisterValidationWorkspaceParitySteps verifies shared validation and
// workspace package coverage on every supported route.
func RegisterValidationWorkspaceParitySteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          validationWorkspacePackageStateKey{},
		label:             "validation workspace package",
		featurePath:       validationWorkspaceParityFeaturePath,
		configuredPattern: `^validation workspace package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates validation workspace package coverage$`,
		colocatedPattern:  `^validation workspace package "([^"]*)" should have a co-located SPEC$`,
	})
	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, validationWorkspaceRouteStateKey{}, &validationWorkspaceRouteState{}), nil
	})
	ctx.Step(`^validation workspace harness "([^"]*)" uses model family "([^"]*)"$`, validationWorkspaceHarnessUsesFamily)
	ctx.Step(`^AGM scans validation workspace packages for route defaults$`, agmScansValidationWorkspacePackages)
	ctx.Step(`^validation workspace behavior should remain route neutral$`, validationWorkspaceBehaviorShouldRemainNeutral)
}

func validationWorkspaceHarnessUsesFamily(ctx context.Context, harness, family string) error {
	state, err := getValidationWorkspaceRouteState(ctx)
	if err != nil {
		return err
	}
	if !slices.Contains([]string{"claude-code", "codex-cli", "agy", "opencode-cli", "pi-cli"}, harness) {
		return fmt.Errorf("unsupported validation workspace harness %q", harness)
	}
	if !slices.Contains([]string{"anthropic", "openai", "gemini", "glm", "deepseek", "nemotron", "qwen"}, family) {
		return fmt.Errorf("unsupported validation workspace model family %q", family)
	}
	state.harness, state.family = harness, family
	return nil
}

func agmScansValidationWorkspacePackages(ctx context.Context) error {
	state, err := getValidationWorkspaceRouteState(ctx)
	if err != nil {
		return err
	}
	root := packageSpecBDDRepoRoot()
	for _, pkg := range validationWorkspacePackages() {
		if err := scanValidationWorkspacePackage(root, pkg); err != nil {
			return fmt.Errorf("route %s/%s: %w", state.harness, state.family, err)
		}
	}
	state.validated = true
	return nil
}

func validationWorkspaceBehaviorShouldRemainNeutral(ctx context.Context) error {
	state, err := getValidationWorkspaceRouteState(ctx)
	if err != nil {
		return err
	}
	if !state.validated {
		return fmt.Errorf("validation workspace route %s/%s was not validated", state.harness, state.family)
	}
	return nil
}

func validationWorkspacePackages() []string {
	return []string{
		"pkg/security", "pkg/validation", "pkg/validation/engram", "pkg/validation/scope", "pkg/validator",
		"pkg/vcs", "pkg/version", "pkg/workspace", "pkg/workspace/dolt",
	}
}

func scanValidationWorkspacePackage(root, pkg string) error {
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(pkg)))
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(pkg), entry.Name())
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		pattern := regexp.MustCompile(`(?i)(claude|codex|antigravity|opencode|anthropic|openai|gemini|deepseek|nemotron|qwen|(^|[^a-z0-9])glm([^a-z0-9]|$))`)
		var routeLiteral string
		ast.Inspect(file, func(node ast.Node) bool {
			if routeLiteral != "" {
				return false
			}
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			value, unquoteErr := strconv.Unquote(literal.Value)
			if unquoteErr == nil && pattern.MatchString(value) {
				routeLiteral = value
				return false
			}
			return true
		})
		if routeLiteral != "" {
			return fmt.Errorf("shared package %s embeds route-specific string literal %q", path, routeLiteral)
		}
	}
	return nil
}

func getValidationWorkspaceRouteState(ctx context.Context) (*validationWorkspaceRouteState, error) {
	state, ok := ctx.Value(validationWorkspaceRouteStateKey{}).(*validationWorkspaceRouteState)
	if !ok || state == nil {
		return nil, fmt.Errorf("validation workspace route state not initialized")
	}
	return state, nil
}
