package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/pkg/vroom/escalation"
	"github.com/vbonnet/dear-agent/pkg/vroom/supervisor"
)

const vroomRuntimeFeaturePath = "agm/test/bdd/features/vroom_runtime_guardrails.feature"

type vroomRuntimePackageStateKey struct{}
type vroomRuntimeParityStateKey struct{}

type vroomRuntimeParityState struct {
	harness        string
	family         string
	model          string
	adjudicator    *escalation.ModelAdjudicator
	adjudication   escalation.Adjudication
	dispatchArgs   []string
	supervisorSpec string
}

// RegisterVROOMRuntimeGuardrailSteps registers VROOM package and parity steps.
func RegisterVROOMRuntimeGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          vroomRuntimePackageStateKey{},
		label:             "VROOM runtime package",
		featurePath:       vroomRuntimeFeaturePath,
		configuredPattern: `^VROOM runtime package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates VROOM runtime package coverage$`,
		colocatedPattern:  `^VROOM runtime package "([^"]*)" should have a co-located SPEC$`,
	})

	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, vroomRuntimeParityStateKey{}, &vroomRuntimeParityState{}), nil
	})

	ctx.Step(`^VROOM harness "([^"]*)" uses model family "([^"]*)" route "([^"]*)"$`, vroomHarnessUsesModelFamily)
	ctx.Step(`^VROOM builds the shared adjudication and worker dispatch contracts$`, vroomBuildsSharedContracts)
	ctx.Step(`^VROOM adjudication should be attributed to model family "([^"]*)"$`, vroomAdjudicationAttributedToFamily)
	ctx.Step(`^VROOM worker dispatch should preserve model route "([^"]*)"$`, vroomDispatchPreservesModelRoute)
	ctx.Step(`^VROOM contracts should remain independent of harness "([^"]*)"$`, vroomContractsRemainHarnessIndependent)
	ctx.Step(`^VROOM harness "([^"]*)" has no explicit model route$`, vroomHarnessHasNoModelRoute)
	ctx.Step(`^VROOM builds default worker dispatch arguments$`, vroomBuildsDefaultDispatchArguments)
	ctx.Step(`^VROOM worker dispatch should omit a fixed model route$`, vroomDispatchOmitsFixedModelRoute)
	ctx.Step(`^AGM validates VROOM queue storage hygiene$`, agmValidatesVROOMQueueStorageHygiene)
	ctx.Step(`^the VROOM supervisor specification should require cleared backing storage$`, vroomSupervisorSpecificationRequiresClearedBackingStorage)
}

func agmValidatesVROOMQueueStorageHygiene(ctx context.Context) error {
	state, err := getVROOMRuntimeParityState(ctx)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(packageSpecBDDRepoRoot(), "pkg", "vroom", "supervisor", "SPEC.md"))
	if err != nil {
		return fmt.Errorf("read VROOM supervisor SPEC: %w", err)
	}
	state.supervisorSpec = string(data)
	return nil
}

func vroomSupervisorSpecificationRequiresClearedBackingStorage(ctx context.Context) error {
	state, err := getVROOMRuntimeParityState(ctx)
	if err != nil {
		return err
	}
	if !strings.Contains(state.supervisorSpec, "**VROOM-SUP-27**") || !strings.Contains(state.supervisorSpec, "clear the vacated backing-array slot") {
		return fmt.Errorf("VROOM supervisor SPEC does not enforce queue storage release")
	}
	return nil
}

func vroomHarnessUsesModelFamily(ctx context.Context, harness, family, model string) error {
	state, err := getVROOMRuntimeParityState(ctx)
	if err != nil {
		return err
	}
	if !slices.Contains(agent.ActiveHarnesses(), harness) {
		return fmt.Errorf("VROOM harness %q is not in the active parity registry", harness)
	}
	if !agent.IsSupportedModelFamily(family) {
		return fmt.Errorf("VROOM model family %q is not in the supported parity registry", family)
	}
	if got := agent.ModelFamilyForName(model); got != family {
		return fmt.Errorf("VROOM model route %q resolves to family %q, want %q", model, got, family)
	}
	state.harness = harness
	state.family = family
	state.model = model
	return nil
}

func vroomBuildsSharedContracts(ctx context.Context) error {
	state, err := getVROOMRuntimeParityState(ctx)
	if err != nil {
		return err
	}
	if state.harness == "" || state.family == "" || state.model == "" {
		return fmt.Errorf("VROOM parity route is incomplete")
	}
	fake := &escalation.FakeAdjudicator{
		Verdict: escalation.Adjudication{Outcome: escalation.OutcomeCorrect},
	}
	state.adjudicator = escalation.NewModelAdjudicator(state.family, fake)
	state.adjudication, err = state.adjudicator.Adjudicate(ctx, escalation.AdjudicationRequest{
		Answer: "a substantive answer from the configured model family",
	})
	if err != nil {
		return err
	}
	state.dispatchArgs = supervisor.AGMDispatchArgs("bdd-parity", state.model, "worker")
	return nil
}

func vroomAdjudicationAttributedToFamily(ctx context.Context, family string) error {
	state, err := getVROOMRuntimeParityState(ctx)
	if err != nil {
		return err
	}
	if state.adjudicator == nil {
		return fmt.Errorf("VROOM adjudicator was not built")
	}
	if got := state.adjudicator.Name(); got != family {
		return fmt.Errorf("adjudicator family = %q, want %q", got, family)
	}
	if state.adjudication.Outcome != escalation.OutcomeCorrect {
		return fmt.Errorf("adjudication outcome = %q, want correct", state.adjudication.Outcome)
	}
	return nil
}

func vroomDispatchPreservesModelRoute(ctx context.Context, model string) error {
	state, err := getVROOMRuntimeParityState(ctx)
	if err != nil {
		return err
	}
	for i, arg := range state.dispatchArgs {
		if arg == "--model" && i+1 < len(state.dispatchArgs) && state.dispatchArgs[i+1] == model {
			return nil
		}
	}
	return fmt.Errorf("dispatch args %v do not preserve model route %q", state.dispatchArgs, model)
}

func vroomContractsRemainHarnessIndependent(ctx context.Context, harness string) error {
	state, err := getVROOMRuntimeParityState(ctx)
	if err != nil {
		return err
	}
	if state.harness != harness {
		return fmt.Errorf("configured harness = %q, want %q", state.harness, harness)
	}
	if slices.Contains(state.dispatchArgs, "--harness") {
		return fmt.Errorf("shared VROOM dispatch unexpectedly pins a harness: %v", state.dispatchArgs)
	}
	return nil
}

func vroomHarnessHasNoModelRoute(ctx context.Context, harness string) error {
	state, err := getVROOMRuntimeParityState(ctx)
	if err != nil {
		return err
	}
	if !slices.Contains(agent.ActiveHarnesses(), harness) {
		return fmt.Errorf("VROOM harness %q is not in the active parity registry", harness)
	}
	state.harness = harness
	state.model = ""
	return nil
}

func vroomBuildsDefaultDispatchArguments(ctx context.Context) error {
	state, err := getVROOMRuntimeParityState(ctx)
	if err != nil {
		return err
	}
	if state.harness == "" {
		return fmt.Errorf("no VROOM harness configured")
	}
	state.dispatchArgs = supervisor.AGMDispatchArgs("bdd-default", "", "worker")
	return nil
}

func vroomDispatchOmitsFixedModelRoute(ctx context.Context) error {
	state, err := getVROOMRuntimeParityState(ctx)
	if err != nil {
		return err
	}
	if slices.Contains(state.dispatchArgs, "--model") {
		return fmt.Errorf("default dispatch pins a model route: %v", state.dispatchArgs)
	}
	return nil
}

func getVROOMRuntimeParityState(ctx context.Context) (*vroomRuntimeParityState, error) {
	state, ok := ctx.Value(vroomRuntimeParityStateKey{}).(*vroomRuntimeParityState)
	if !ok || state == nil {
		return nil, fmt.Errorf("VROOM runtime parity state not initialized")
	}
	return state, nil
}
