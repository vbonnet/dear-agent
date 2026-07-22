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

const sharedRuntimePolicyFeaturePath = "agm/test/bdd/features/shared_runtime_policy_guardrails.feature"

type sharedRuntimePolicyGuardrailStateKey struct{}

type sharedRuntimePolicyRouteStateKey struct{}

type sharedRuntimePolicyRouteState struct {
	harness   string
	family    string
	validated bool
}

func sharedRuntimePolicyConfig() packageSpecGuardrailConfig {
	return packageSpecGuardrailConfig{
		stateKey:          sharedRuntimePolicyGuardrailStateKey{},
		label:             "shared runtime policy package",
		featurePath:       sharedRuntimePolicyFeaturePath,
		configuredPattern: `^shared runtime policy package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates shared runtime policy package coverage$`,
		colocatedPattern:  `^shared runtime policy package "([^"]*)" should have a co-located SPEC$`,
	}
}

// RegisterSharedRuntimePolicyGuardrailSteps verifies package traceability and
// prevents shared policy code from acquiring route-specific defaults.
func RegisterSharedRuntimePolicyGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, sharedRuntimePolicyConfig())
	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, sharedRuntimePolicyRouteStateKey{}, &sharedRuntimePolicyRouteState{}), nil
	})
	ctx.Step(
		`^shared runtime policy package "([^"]*)" should cover every supported harness and model family$`,
		sharedRuntimePolicyPackageShouldCoverEveryRoute,
	)
	ctx.Step(
		`^shared runtime policy package "([^"]*)" should not embed a harness or model-family route$`,
		sharedRuntimePolicyPackageShouldNotEmbedRoute,
	)
	ctx.Step(`^shared runtime route harness "([^"]*)" uses model family "([^"]*)"$`, sharedRuntimeRouteUsesModelFamily)
	ctx.Step(`^AGM validates every shared runtime policy package for that route$`, agmValidatesEverySharedRuntimePolicyPackage)
	ctx.Step(`^no shared runtime policy package should embed route-specific behavior$`, noSharedRuntimePolicyPackageShouldEmbedRoute)
}

func sharedRuntimeRouteUsesModelFamily(ctx context.Context, harness, family string) error {
	state, err := getSharedRuntimePolicyRouteState(ctx)
	if err != nil {
		return err
	}
	if !slices.Contains([]string{"claude-code", "codex-cli", "agy", "opencode-cli", "pi-cli"}, harness) {
		return fmt.Errorf("unsupported shared runtime harness %q", harness)
	}
	if !slices.Contains([]string{"anthropic", "openai", "gemini", "glm", "deepseek", "nemotron", "qwen"}, family) {
		return fmt.Errorf("unsupported shared runtime model family %q", family)
	}
	state.harness, state.family = harness, family
	return nil
}

func agmValidatesEverySharedRuntimePolicyPackage(ctx context.Context) error {
	state, err := getSharedRuntimePolicyRouteState(ctx)
	if err != nil {
		return err
	}
	if state.harness == "" || state.family == "" {
		return fmt.Errorf("shared runtime route is not configured")
	}
	root := packageSpecBDDRepoRoot()
	for _, pkg := range sharedRuntimePolicyPackages() {
		if err := rejectRouteSpecificPackageLiterals(root, pkg); err != nil {
			return fmt.Errorf("route %s/%s: %w", state.harness, state.family, err)
		}
	}
	state.validated = true
	return nil
}

func noSharedRuntimePolicyPackageShouldEmbedRoute(ctx context.Context) error {
	state, err := getSharedRuntimePolicyRouteState(ctx)
	if err != nil {
		return err
	}
	if !state.validated {
		return fmt.Errorf("shared runtime route %s/%s was not validated", state.harness, state.family)
	}
	return nil
}

func sharedRuntimePolicyPackageShouldCoverEveryRoute(ctx context.Context, pkg string) error {
	state, err := getPackageSpecGuardrailState(ctx, sharedRuntimePolicyConfig())
	if err != nil {
		return err
	}
	if pkg != state.pkg {
		return fmt.Errorf("configured shared runtime policy package = %q, want %q", state.pkg, pkg)
	}
	data, err := os.ReadFile(state.spec)
	if err != nil {
		return fmt.Errorf("read shared runtime policy SPEC %s: %w", state.spec, err)
	}
	spec := strings.ToLower(string(data))
	for _, harness := range []string{"claude", "codex", "antigravity", "opencode"} {
		if !strings.Contains(spec, harness) {
			return fmt.Errorf("%s does not cover harness %s", state.spec, harness)
		}
	}
	for _, family := range []string{"anthropic", "openai", "gemini", "glm", "deepseek", "nemotron", "qwen"} {
		if !strings.Contains(spec, family) {
			return fmt.Errorf("%s does not cover model family %s", state.spec, family)
		}
	}
	return nil
}

func sharedRuntimePolicyPackageShouldNotEmbedRoute(ctx context.Context, pkg string) error {
	state, err := getPackageSpecGuardrailState(ctx, sharedRuntimePolicyConfig())
	if err != nil {
		return err
	}
	if pkg != state.pkg {
		return fmt.Errorf("configured shared runtime policy package = %q, want %q", state.pkg, pkg)
	}
	return rejectRouteSpecificPackageLiterals(state.repoRoot, pkg)
}

func rejectRouteSpecificPackageLiterals(repoRoot, pkg string) error {
	entries, err := os.ReadDir(filepath.Join(repoRoot, filepath.FromSlash(pkg)))
	if err != nil {
		return fmt.Errorf("read shared runtime policy package %s: %w", pkg, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(repoRoot, filepath.FromSlash(pkg), entry.Name())
		if err := rejectRouteSpecificStringLiterals(path); err != nil {
			return err
		}
	}
	return nil
}

func sharedRuntimePolicyPackages() []string {
	return []string{
		"pkg/autoconfig", "pkg/backlog", "pkg/codegen", "pkg/codeintel", "pkg/config-loader",
		"pkg/enforcement", "pkg/evalcase", "pkg/eventbus", "pkg/frontmatter", "pkg/gracefulexit",
		"pkg/health-checker",
	}
}

func getSharedRuntimePolicyRouteState(ctx context.Context) (*sharedRuntimePolicyRouteState, error) {
	state, ok := ctx.Value(sharedRuntimePolicyRouteStateKey{}).(*sharedRuntimePolicyRouteState)
	if !ok || state == nil {
		return nil, fmt.Errorf("shared runtime policy route state not initialized")
	}
	return state, nil
}

func rejectRouteSpecificStringLiterals(path string) error {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	pattern := regexp.MustCompile(`(?i)(claude|codex|antigravity|opencode|anthropic|openai|gemini|deepseek|nemotron|qwen|(^|[^a-z0-9])glm([^a-z0-9]|$))`)
	var violation error
	ast.Inspect(file, func(node ast.Node) bool {
		if violation != nil {
			return false
		}
		literal, ok := node.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(literal.Value)
		if err != nil {
			violation = fmt.Errorf("unquote string literal in %s: %w", path, err)
			return false
		}
		if pattern.MatchString(value) {
			violation = fmt.Errorf("shared package %s embeds route-specific string literal %q", path, value)
			return false
		}
		return true
	})
	return violation
}
