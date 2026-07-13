package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
	"github.com/vbonnet/dear-agent/internal/safepr"
)

type localDevGuardrailState struct {
	command            string
	commandSpec        string
	library            string
	librarySpec        string
	traceDir           string
	trace              safepr.Session
	harness            string
	family             string
	preflightMinutes   int
	localVulnAllowlist []string
	ciVulnAllowlist    []string
}

type localDevGuardrailStateKey struct{}

// RegisterLocalDevelopmentGuardrailSteps registers BDD steps for audited local development wrappers.
func RegisterLocalDevelopmentGuardrailSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, localDevGuardrailStateKey{}, &localDevGuardrailState{}), nil
	})
	ctx.After(func(ctx context.Context, _ *godog.Scenario, scenarioErr error) (context.Context, error) {
		state, err := getLocalDevGuardrailState(ctx)
		if err == nil && state.traceDir != "" {
			if removeErr := os.RemoveAll(state.traceDir); removeErr != nil && scenarioErr == nil {
				return ctx, fmt.Errorf("remove canonical trace directory: %w", removeErr)
			}
		}
		return ctx, nil
	})

	ctx.Step(`^safe local development command "([^"]*)" is configured$`, safeLocalDevelopmentCommandIsConfigured)
	ctx.Step(`^AGM validates safe local development command coverage$`, agmValidatesSafeLocalDevelopmentCommandCoverage)
	ctx.Step(`^safe local development command "([^"]*)" should have a co-located SPEC$`, safeLocalDevelopmentCommandShouldHaveCoLocatedSPEC)
	ctx.Step(`^safe local development library "([^"]*)" is configured$`, safeLocalDevelopmentLibraryIsConfigured)
	ctx.Step(`^AGM validates safe local development library coverage$`, agmValidatesSafeLocalDevelopmentLibraryCoverage)
	ctx.Step(`^safe local development library "([^"]*)" should have a co-located SPEC$`, safeLocalDevelopmentLibraryShouldHaveCoLocatedSPEC)
	ctx.Step(`^canonical Wayfinder V2 status for harness "([^"]*)" and model family "([^"]*)"$`, canonicalWayfinderV2StatusForRoute)
	ctx.Step(`^safe-pr loads the canonical planning trace$`, safePRLoadsCanonicalPlanningTrace)
	ctx.Step(`^safe-pr should attribute the trace to project "([^"]*)"$`, safePRShouldAttributeTraceToProject)
	ctx.Step(`^the safe-pr full preflight timeout is configured$`, safePRFullPreflightTimeoutIsConfigured)
	ctx.Step(`^AGM validates the safe-pr preflight budget$`, agmValidatesSafePRPreflightBudget)
	ctx.Step(`^safe-pr should allow at least (\d+) minutes for preflight-full$`, safePRShouldAllowAtLeastMinutesForPreflightFull)
	ctx.Step(`^local and required CI govulncheck allowlists are configured$`, localAndRequiredCIGovulncheckAllowlistsAreConfigured)
	ctx.Step(`^AGM validates govulncheck policy parity$`, agmValidatesGovulncheckPolicyParity)
	ctx.Step(`^the local and required CI govulncheck allowlists should match$`, localAndRequiredCIGovulncheckAllowlistsShouldMatch)
}

func localAndRequiredCIGovulncheckAllowlistsAreConfigured(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	root := localDevBDDRepoRoot()
	state.localVulnAllowlist, err = readGovulncheckAllowlist(filepath.Join(root, "scripts", "preflight.sh"))
	if err != nil {
		return fmt.Errorf("local govulncheck allowlist: %w", err)
	}
	state.ciVulnAllowlist, err = readGovulncheckAllowlist(filepath.Join(root, ".github", "workflows", "ci.yml"))
	if err != nil {
		return fmt.Errorf("required CI govulncheck allowlist: %w", err)
	}
	return nil
}

func readGovulncheckAllowlist(path string) ([]string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	match := regexp.MustCompile(`--argjson allow '(\[[^']+\])'`).FindSubmatch(content)
	if len(match) != 2 {
		return nil, fmt.Errorf("allowlist declaration not found in %s", path)
	}
	var ids []string
	if err := json.Unmarshal(match[1], &ids); err != nil {
		return nil, fmt.Errorf("parse allowlist in %s: %w", path, err)
	}
	return ids, nil
}

func agmValidatesGovulncheckPolicyParity(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if len(state.localVulnAllowlist) == 0 || len(state.ciVulnAllowlist) == 0 {
		return fmt.Errorf("govulncheck allowlists are not configured")
	}
	return nil
}

func localAndRequiredCIGovulncheckAllowlistsShouldMatch(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if !slices.Equal(state.localVulnAllowlist, state.ciVulnAllowlist) {
		return fmt.Errorf("local govulncheck allowlist %v does not match required CI %v", state.localVulnAllowlist, state.ciVulnAllowlist)
	}
	return nil
}

func safePRFullPreflightTimeoutIsConfigured(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	sourcePath := filepath.Join(localDevBDDRepoRoot(), "cmd", "safe-pr", "main.go")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("read safe-pr source: %w", err)
	}
	match := regexp.MustCompile(`preflightTimeout\s*=\s*(\d+)\s*\*\s*time\.Minute`).FindSubmatch(source)
	if len(match) != 2 {
		return fmt.Errorf("safe-pr preflight timeout declaration not found")
	}
	state.preflightMinutes, err = strconv.Atoi(string(match[1]))
	if err != nil {
		return fmt.Errorf("parse safe-pr preflight timeout: %w", err)
	}
	return nil
}

func agmValidatesSafePRPreflightBudget(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.preflightMinutes <= 0 {
		return fmt.Errorf("safe-pr preflight timeout is not configured")
	}
	return nil
}

func safePRShouldAllowAtLeastMinutesForPreflightFull(ctx context.Context, minimum int) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.preflightMinutes < minimum {
		return fmt.Errorf("safe-pr preflight timeout = %dm, want at least %dm", state.preflightMinutes, minimum)
	}
	return nil
}

func canonicalWayfinderV2StatusForRoute(ctx context.Context, harness, family string) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp("", "safe-pr-v2-trace-*")
	if err != nil {
		return fmt.Errorf("create canonical trace directory: %w", err)
	}
	state.traceDir = dir
	state.harness = harness
	state.family = family
	content := fmt.Sprintf("---\nschema_version: \"2.0\"\nproject_name: parity-audit\nproject_type: infrastructure\nrisk_level: M\ncurrent_waypoint: CHARTER\nstatus: planning\nharness: %s\nmodel_family: %s\n---\n", harness, family)
	if err := os.WriteFile(filepath.Join(dir, "WAYFINDER-STATUS.md"), []byte(content), 0o600); err != nil {
		return fmt.Errorf("write canonical trace: %w", err)
	}
	return nil
}

func safePRLoadsCanonicalPlanningTrace(ctx context.Context) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.traceDir == "" {
		return fmt.Errorf("trace directory is not initialized in scenario state")
	}
	trace, err := safepr.LoadSession(state.traceDir)
	if err != nil {
		return fmt.Errorf("load canonical %s/%s trace: %w", state.harness, state.family, err)
	}
	state.trace = trace
	return nil
}

func safePRShouldAttributeTraceToProject(ctx context.Context, project string) error {
	state, err := getLocalDevGuardrailState(ctx)
	if err != nil {
		return err
	}
	if state.trace.ID != project {
		return fmt.Errorf("safe-pr trace attribution = %q, want %q", state.trace.ID, project)
	}
	return nil
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
	return packageSpecBDDRepoRoot()
}
