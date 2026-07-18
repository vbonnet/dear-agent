package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cucumber/godog"
)

const wayfinderV2CommandFeaturePath = "agm/test/bdd/features/wayfinder_v2_command_guardrails.feature"

type wayfinderV2CommandPackageStateKey struct{}
type wayfinderV2CommandStateKey struct{}

type wayfinderV2CommandState struct {
	repoRoot string
	help     string
}

// RegisterWayfinderV2CommandGuardrailSteps registers canonical command checks.
func RegisterWayfinderV2CommandGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          wayfinderV2CommandPackageStateKey{},
		label:             "Wayfinder V2 command package",
		featurePath:       wayfinderV2CommandFeaturePath,
		configuredPattern: `^Wayfinder V2 command package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates Wayfinder V2 command package coverage$`,
		colocatedPattern:  `^Wayfinder V2 command package "([^"]*)" should have a co-located SPEC$`,
	})

	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, wayfinderV2CommandStateKey{}, &wayfinderV2CommandState{
			repoRoot: packageSpecBDDRepoRoot(),
		}), nil
	})

	ctx.Step(`^AGM inspects the Wayfinder root help contract$`, agmInspectsWayfinderRootHelp)
	ctx.Step(`^Wayfinder help should name all nine canonical phases$`, wayfinderHelpNamesCanonicalPhases)
	ctx.Step(`^Wayfinder help should expose the V2 session command$`, wayfinderHelpExposesV2Session)
	ctx.Step(`^Wayfinder help should not expose retired V1 executors$`, wayfinderHelpOmitsRetiredExecutors)
	ctx.Step(`^Wayfinder help should not expose legacy migration commands$`, wayfinderHelpOmitsLegacyMigrationCommands)
	ctx.Step(`^AGM audits Wayfinder command source policy$`, agmAuditsWayfinderCommandPolicy)
	ctx.Step(`^retired V1 root and feature executors should be absent$`, retiredWayfinderExecutorsAreAbsent)
	ctx.Step(`^all Wayfinder session commands should parse only schema 2.0 status$`, normalWayfinderCommandsParseOnlyV2)
	ctx.Step(`^Wayfinder runtime source should omit retired phase identifiers$`, nonMigrationRuntimeOmitsRetiredPhases)
	ctx.Step(`^Wayfinder phase enumeration should expose the nine named phases$`, unversionedPhasesDefaultToV2)
}

func agmInspectsWayfinderRootHelp(ctx context.Context) error {
	state, err := getWayfinderV2CommandState(ctx)
	if err != nil {
		return err
	}
	path := filepath.Join(state.repoRoot, "wayfinder/cmd/wayfinder/cmd/root.go")
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		return fmt.Errorf("read Wayfinder root help contract: %w", readErr)
	}
	state.help = string(data)
	return nil
}

func wayfinderHelpNamesCanonicalPhases(ctx context.Context) error {
	state, err := getWayfinderV2CommandState(ctx)
	if err != nil {
		return err
	}
	for _, phase := range []string{"CHARTER", "PROBLEM", "RESEARCH", "DESIGN", "SPEC", "PLAN", "SETUP", "BUILD", "RETRO"} {
		if !strings.Contains(state.help, phase) {
			return fmt.Errorf("wayfinder help does not name canonical phase %s", phase)
		}
	}
	return nil
}

func wayfinderHelpExposesV2Session(ctx context.Context) error {
	state, err := getWayfinderV2CommandState(ctx)
	if err != nil {
		return err
	}
	if !strings.Contains(state.help, "session") || !strings.Contains(state.help, "canonical 9-phase") {
		return fmt.Errorf("wayfinder help does not expose the canonical session surface: %s", state.help)
	}
	return nil
}

func wayfinderHelpOmitsRetiredExecutors(ctx context.Context) error {
	state, err := getWayfinderV2CommandState(ctx)
	if err != nil {
		return err
	}
	for _, command := range []string{"start", "autopilot", "features", "abort"} {
		if strings.Contains(state.help, "  "+command+" ") {
			return fmt.Errorf("wayfinder help exposes retired direct command %q", command)
		}
	}
	return nil
}

func wayfinderHelpOmitsLegacyMigrationCommands(ctx context.Context) error {
	state, err := getWayfinderV2CommandState(ctx)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(state.repoRoot, "wayfinder/cmd/wayfinder/cmd/session.go"))
	if err != nil {
		return err
	}
	for _, command := range []string{"MigrateCmd", "MigrateAllCmd"} {
		if strings.Contains(string(data), command) {
			return fmt.Errorf("Wayfinder session command still registers %s", command)
		}
	}
	return nil
}

func agmAuditsWayfinderCommandPolicy(ctx context.Context) error {
	_, err := getWayfinderV2CommandState(ctx)
	return err
}

func retiredWayfinderExecutorsAreAbsent(ctx context.Context) error {
	state, err := getWayfinderV2CommandState(ctx)
	if err != nil {
		return err
	}
	for _, path := range []string{
		"wayfinder/cmd/wayfinder/cmd/start.go",
		"wayfinder/cmd/wayfinder/cmd/autopilot.go",
		"wayfinder/cmd/wayfinder/cmd/features.go",
		"wayfinder/cmd/wayfinder/cmd/abort.go",
	} {
		if _, statErr := os.Stat(filepath.Join(state.repoRoot, path)); statErr == nil {
			return fmt.Errorf("retired Wayfinder executor still exists: %s", path)
		}
	}
	featureRoot := filepath.Join(state.repoRoot, "wayfinder/cmd/wayfinder-features")
	if _, statErr := os.Stat(featureRoot); os.IsNotExist(statErr) {
		return nil
	}
	return filepath.WalkDir(featureRoot, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".go") {
			return fmt.Errorf("retired Wayfinder feature executor still exists: %s", path)
		}
		return nil
	})
}

func normalWayfinderCommandsParseOnlyV2(ctx context.Context) error {
	state, err := getWayfinderV2CommandState(ctx)
	if err != nil {
		return err
	}
	for _, path := range []string{
		"wayfinder/cmd/wayfinder-session/commands/start_phase.go",
		"wayfinder/cmd/wayfinder-session/commands/complete_phase.go",
		"wayfinder/cmd/wayfinder-session/commands/next_phase.go",
		"wayfinder/cmd/wayfinder-session/commands/end.go",
		"wayfinder/cmd/wayfinder-session/commands/status.go",
		"wayfinder/cmd/wayfinder-session/commands/set_lifecycle_state.go",
	} {
		data, readErr := os.ReadFile(filepath.Join(state.repoRoot, path))
		if readErr != nil {
			return readErr
		}
		text := string(data)
		for _, forbidden := range []string{"status.ReadFrom(", "status.LoadAnyVersion(", "status.DetectFromFilesystem(", "runEndV1"} {
			if strings.Contains(text, forbidden) {
				return fmt.Errorf("normal command %s retains legacy path %q", path, forbidden)
			}
		}
		if !strings.Contains(text, "ParseV2FromDir") && !strings.Contains(text, "runEndV2") {
			return fmt.Errorf("normal command %s has no canonical V2 parser path", path)
		}
	}
	return nil
}

func nonMigrationRuntimeOmitsRetiredPhases(ctx context.Context) (resultErr error) {
	state, err := getWayfinderV2CommandState(ctx)
	if err != nil {
		return err
	}
	root := filepath.Join(state.repoRoot, "wayfinder")
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		return fmt.Errorf("open Wayfinder source root: %w", err)
	}
	defer preserveRootCloseError(rootFS, &resultErr)
	retired := regexp.MustCompile(`\b(W0|D1|D2|D3|D4|S4|S5|S6|S7|S8|S9|S10|S11)\b|S11-retrospective|discovery\.(problem|solutions|approach|requirements)|design\.(tech-lead|security|qa)|roadmap\.(planning|breakdown|dependencies)`)
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if strings.Contains(rel, string(filepath.Separator)+"migrate"+string(filepath.Separator)) ||
			strings.Contains(rel, string(filepath.Separator)+"migration"+string(filepath.Separator)) ||
			strings.Contains(rel, string(filepath.Separator)+"converter"+string(filepath.Separator)) ||
			strings.Contains(rel, filepath.Join("internal", "status")+string(filepath.Separator)) ||
			rel == filepath.Join("cmd", "wayfinder-session", "commands", "migrate.go") {
			return nil
		}
		data, readErr := rootFS.ReadFile(filepath.ToSlash(rel))
		if readErr != nil {
			return readErr
		}
		if token := retired.FindString(string(data)); token != "" {
			return fmt.Errorf("non-migration Wayfinder runtime %s contains retired phase token %s", rel, token)
		}
		return nil
	})
}

func preserveRootCloseError(rootFS *os.Root, resultErr *error) {
	if closeErr := rootFS.Close(); *resultErr == nil && closeErr != nil {
		*resultErr = fmt.Errorf("close Wayfinder source root: %w", closeErr)
	}
}

func unversionedPhasesDefaultToV2(ctx context.Context) error {
	state, err := getWayfinderV2CommandState(ctx)
	if err != nil {
		return err
	}
	path := filepath.Join(state.repoRoot, "wayfinder/cmd/wayfinder-session/internal/status/types.go")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !strings.Contains(string(data), "v := WayfinderV2") {
		return fmt.Errorf("unversioned AllPhases does not default to WayfinderV2")
	}
	return nil
}

func getWayfinderV2CommandState(ctx context.Context) (*wayfinderV2CommandState, error) {
	state, ok := ctx.Value(wayfinderV2CommandStateKey{}).(*wayfinderV2CommandState)
	if !ok || state == nil {
		return nil, fmt.Errorf("wayfinder V2 command state not initialized")
	}
	return state, nil
}
