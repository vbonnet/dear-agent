package steps

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

const agmControlSurfaceFeaturePath = "agm/test/bdd/features/agm_control_surface_guardrails.feature"

type agmControlSurfaceGuardrailStateKey struct{}

// RegisterAGMControlSurfaceGuardrailSteps registers BDD steps for AGM control-plane packages.
func RegisterAGMControlSurfaceGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          agmControlSurfaceGuardrailStateKey{},
		label:             "AGM control surface package",
		featurePath:       agmControlSurfaceFeaturePath,
		configuredPattern: `^AGM control surface package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates control surface package coverage$`,
		colocatedPattern:  `^AGM control surface package "([^"]*)" should have a co-located SPEC$`,
	})
	ctx.Step(`^AGM Cobra commands with mutable execution state$`, agmCobraCommandsWithMutableExecutionState)
	ctx.Step(`^AGM audits Cobra command test isolation$`, agmAuditsCobraCommandTestIsolation)
	ctx.Step(`^mutable command flags should belong to fresh command instances$`, mutableCommandFlagsShouldBelongToFreshCommandInstances)
	ctx.Step(`^command validation tests should exercise repeatable execution orders$`, commandValidationTestsShouldExerciseRepeatableExecutionOrders)
}

func agmCobraCommandsWithMutableExecutionState() error {
	root := packageSpecBDDRepoRoot()
	for _, name := range []string{
		"agm/cmd/agm/admin_install_harness.go",
		"agm/cmd/agm/session_search.go",
		"agm/cmd/agm/session_tag.go",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); err != nil {
			return fmt.Errorf("locate Cobra command source %s: %w", name, err)
		}
	}
	return nil
}

func agmAuditsCobraCommandTestIsolation() error {
	root := packageSpecBDDRepoRoot()
	data, err := os.ReadFile(filepath.Join(root, "agm", "cmd", "agm", "command_state_regression_test.go"))
	if err != nil {
		return fmt.Errorf("read Cobra command-state regression: %w", err)
	}
	if !strings.Contains(string(data), "TestCobraCommandValidationIsOrderIndependent") {
		return fmt.Errorf("Cobra command-state regression does not declare its order-independence contract")
	}
	return nil
}

func mutableCommandFlagsShouldBelongToFreshCommandInstances() error {
	root := packageSpecBDDRepoRoot()
	requirements := map[string][]string{
		"agm/cmd/agm/admin_install_harness.go": {"func newInstallHarnessCommand()", "func newInstallCodexCommand()"},
		"agm/cmd/agm/session_search.go":        {"func newSessionSearchCommand()"},
		"agm/cmd/agm/session_tag.go":           {"func newSessionTagCommand()"},
	}
	for name, markers := range requirements {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		text := string(data)
		for _, marker := range markers {
			if !strings.Contains(text, marker) {
				return fmt.Errorf("%s does not define fresh command factory %q", name, marker)
			}
		}
		if strings.Contains(text, ".BoolVar(") || strings.Contains(text, ".StringVar(") {
			return fmt.Errorf("%s binds mutable Cobra flags to package globals", name)
		}
	}
	return nil
}

func commandValidationTestsShouldExerciseRepeatableExecutionOrders() error {
	root := packageSpecBDDRepoRoot()
	data, err := os.ReadFile(filepath.Join(root, "agm", "cmd", "agm", "command_state_regression_test.go"))
	if err != nil {
		return fmt.Errorf("read Cobra command-state regression: %w", err)
	}
	text := string(data)
	for _, marker := range []string{"newInstallHarnessCommand", "newSessionSearchCommand", "newSessionTagCommand", `name: "forward"`, `name: "reverse"`, "repeat < 3"} {
		if !strings.Contains(text, marker) {
			return fmt.Errorf("Cobra command-state regression is missing %q", marker)
		}
	}

	spec, err := os.ReadFile(filepath.Join(root, "agm", "cmd", "agm", "SPEC.md"))
	if err != nil {
		return fmt.Errorf("read AGM CLI SPEC: %w", err)
	}
	if !strings.Contains(string(spec), "**CLI-21**") || !strings.Contains(string(spec), "**CLI-22**") {
		return fmt.Errorf("AGM CLI SPEC does not require command-state isolation and pre-storage regex validation")
	}
	return nil
}
