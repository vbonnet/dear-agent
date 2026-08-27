package steps

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cucumber/godog"

	workflowpkg "github.com/vbonnet/dear-agent/pkg/workflow"
)

const workflowPackageFeaturePath = "agm/test/bdd/features/workflow_package_guardrails.feature"

type workflowPackageGuardrailStateKey struct{}
type workflowConstitutionalStateKey struct{}
type workflowDefineFailureStateKey struct{}

type workflowConstitutionalState struct {
	workflow      *workflowpkg.Workflow
	loadErr       error
	runErr        error
	recorderCalls int
	defineCalls   int
	executorCalls int
}

type workflowDefineFailureState struct {
	workflow      *workflowpkg.Workflow
	runErr        error
	beginCalls    int
	finishCalls   int
	finishState   workflowpkg.RunState
	finishError   string
	nodeCalls     int
	attemptCalls  int
	enforceCalls  int
	executorCalls int
}

// RegisterWorkflowPackageGuardrailSteps registers BDD steps for workflow implementation packages.
func RegisterWorkflowPackageGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          workflowPackageGuardrailStateKey{},
		label:             "workflow package",
		featurePath:       workflowPackageFeaturePath,
		configuredPattern: `^workflow package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates workflow package coverage$`,
		colocatedPattern:  `^workflow package "([^"]*)" should have a co-located SPEC$`,
	})
	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		ctx = context.WithValue(ctx, workflowConstitutionalStateKey{}, &workflowConstitutionalState{})
		return context.WithValue(ctx, workflowDefineFailureStateKey{}, &workflowDefineFailureState{}), nil
	})
	ctx.Step(`^a workflow enables constitutional enforcement without invariants$`, enforcedWorkflowOmitsInvariants)
	ctx.Step(`^AGM validates and attempts to run the workflow$`, validateAndRunEnforcedWorkflow)
	ctx.Step(`^workflow validation should fail before run recording, lifecycle hooks, or node execution$`, workflowValidationFailsBeforeExecution)
	ctx.Step(`^a valid workflow whose definition policy rejects it$`, validWorkflowWithRejectingDefinitionPolicy)
	ctx.Step(`^AGM attempts to run the definition-rejected workflow$`, runDefinitionRejectedWorkflow)
	ctx.Step(`^the run should fail before node execution with a terminal definition error$`, definitionFailureStopsExecution)
}

func validWorkflowWithRejectingDefinitionPolicy(ctx context.Context) error {
	state, err := getWorkflowDefineFailureState(ctx)
	if err != nil {
		return err
	}
	state.workflow = &workflowpkg.Workflow{
		Name: "bdd-define-failure", Version: "1",
		Nodes: []workflowpkg.Node{{
			ID: "execute", Kind: workflowpkg.KindAI, AI: &workflowpkg.AINode{Prompt: "must not execute"},
		}},
	}
	return nil
}

func runDefinitionRejectedWorkflow(ctx context.Context) error {
	state, err := getWorkflowDefineFailureState(ctx)
	if err != nil {
		return err
	}
	if state.workflow == nil {
		return fmt.Errorf("definition-rejected workflow not configured")
	}
	runner := workflowpkg.NewRunner(workflowDefineFailureExecutor{state: state})
	runner.Recorder = workflowDefineFailureRecorder{state: state}
	runner.Hooks = &workflowpkg.Hooks{
		OnDefine: func(context.Context, workflowpkg.DefinePayload) error {
			return errors.New("policy rejected definition")
		},
		OnEnforce: func(context.Context, workflowpkg.EnforcePayload) error {
			state.enforceCalls++
			return nil
		},
	}
	_, state.runErr = runner.Run(ctx, state.workflow, nil)
	return nil
}

func definitionFailureStopsExecution(ctx context.Context) error {
	state, err := getWorkflowDefineFailureState(ctx)
	if err != nil {
		return err
	}
	const want = "hook OnDefine: policy rejected definition"
	if state.runErr == nil || !strings.Contains(state.runErr.Error(), want) {
		return fmt.Errorf("Runner.Run error: %w; want %q", state.runErr, want)
	}
	if state.beginCalls != 1 || state.finishCalls != 1 || state.finishState != workflowpkg.RunStateFailed {
		return fmt.Errorf("run record begin=%d finish=%d state=%q, want 1/1/failed",
			state.beginCalls, state.finishCalls, state.finishState)
	}
	if state.finishError != state.runErr.Error() {
		return fmt.Errorf("terminal error = %q, run error: %w", state.finishError, state.runErr)
	}
	if state.nodeCalls != 0 || state.attemptCalls != 0 || state.enforceCalls != 0 || state.executorCalls != 0 {
		return fmt.Errorf("definition failure reached execution: nodes=%d attempts=%d enforce=%d executor=%d",
			state.nodeCalls, state.attemptCalls, state.enforceCalls, state.executorCalls)
	}
	return nil
}

func enforcedWorkflowOmitsInvariants(ctx context.Context) error {
	state, err := getWorkflowConstitutionalState(ctx)
	if err != nil {
		return err
	}
	state.workflow = &workflowpkg.Workflow{
		Name: "bdd-constitutional", Version: "1",
		Constitutional: &workflowpkg.Constitutional{Enforce: true},
		Nodes: []workflowpkg.Node{{
			ID: "execute", Kind: workflowpkg.KindAI, AI: &workflowpkg.AINode{Prompt: "must not execute"},
		}},
	}
	return nil
}

func validateAndRunEnforcedWorkflow(ctx context.Context) error {
	state, err := getWorkflowConstitutionalState(ctx)
	if err != nil {
		return err
	}
	if state.workflow == nil {
		return fmt.Errorf("constitutional workflow not configured")
	}

	const raw = `name: bdd-constitutional
version: "1"
constitutional:
  enforce: true
nodes:
  - id: execute
    kind: ai
    ai:
      prompt: must not execute
`
	_, state.loadErr = workflowpkg.LoadBytes([]byte(raw))
	runner := workflowpkg.NewRunner(workflowConstitutionalExecutor{state: state})
	runner.Recorder = workflowConstitutionalRecorder{state: state}
	runner.Hooks = &workflowpkg.Hooks{OnDefine: func(context.Context, workflowpkg.DefinePayload) error {
		state.defineCalls++
		return nil
	}}
	_, state.runErr = runner.Run(ctx, state.workflow, nil)
	return nil
}

func workflowValidationFailsBeforeExecution(ctx context.Context) error {
	state, err := getWorkflowConstitutionalState(ctx)
	if err != nil {
		return err
	}
	const want = "constitutional mode is on but declares no invariants"
	if state.loadErr == nil {
		return errors.New("LoadBytes unexpectedly accepted enforced workflow without invariants")
	}
	if !strings.Contains(state.loadErr.Error(), want) {
		return fmt.Errorf("LoadBytes error: %w; want %q", state.loadErr, want)
	}
	if state.runErr == nil {
		return errors.New("Runner.Run unexpectedly accepted enforced workflow without invariants")
	}
	if !strings.Contains(state.runErr.Error(), want) {
		return fmt.Errorf("Runner.Run error: %w; want %q", state.runErr, want)
	}
	if state.recorderCalls != 0 || state.defineCalls != 0 || state.executorCalls != 0 {
		return fmt.Errorf("invalid workflow reached runtime: recorder=%d define=%d executor=%d",
			state.recorderCalls, state.defineCalls, state.executorCalls)
	}
	return nil
}

type workflowConstitutionalExecutor struct {
	state *workflowConstitutionalState
}

type workflowDefineFailureExecutor struct {
	state *workflowDefineFailureState
}

type workflowDefineFailureRecorder struct {
	state *workflowDefineFailureState
}

func (r workflowDefineFailureRecorder) BeginRun(context.Context, workflowpkg.RunRecord) error {
	r.state.beginCalls++
	return nil
}

func (r workflowDefineFailureRecorder) UpsertNode(context.Context, workflowpkg.NodeRecord) error {
	r.state.nodeCalls++
	return nil
}

func (r workflowDefineFailureRecorder) RecordAttempt(context.Context, workflowpkg.AttemptRecord) error {
	r.state.attemptCalls++
	return nil
}

func (r workflowDefineFailureRecorder) FinishRun(
	_ context.Context,
	_ string,
	state workflowpkg.RunState,
	_ time.Time,
	errMsg string,
) error {
	r.state.finishCalls++
	r.state.finishState = state
	r.state.finishError = errMsg
	return nil
}

func (e workflowDefineFailureExecutor) Generate(
	context.Context,
	*workflowpkg.AINode,
	map[string]string,
	map[string]string,
) (string, error) {
	e.state.executorCalls++
	return "unexpected", nil
}

type workflowConstitutionalRecorder struct {
	state *workflowConstitutionalState
}

func (r workflowConstitutionalRecorder) BeginRun(context.Context, workflowpkg.RunRecord) error {
	r.state.recorderCalls++
	return nil
}

func (r workflowConstitutionalRecorder) UpsertNode(context.Context, workflowpkg.NodeRecord) error {
	r.state.recorderCalls++
	return nil
}

func (r workflowConstitutionalRecorder) RecordAttempt(context.Context, workflowpkg.AttemptRecord) error {
	r.state.recorderCalls++
	return nil
}

func (r workflowConstitutionalRecorder) FinishRun(
	context.Context,
	string,
	workflowpkg.RunState,
	time.Time,
	string,
) error {
	r.state.recorderCalls++
	return nil
}

func (e workflowConstitutionalExecutor) Generate(
	context.Context,
	*workflowpkg.AINode,
	map[string]string,
	map[string]string,
) (string, error) {
	e.state.executorCalls++
	return "unexpected", nil
}

func getWorkflowConstitutionalState(ctx context.Context) (*workflowConstitutionalState, error) {
	state, ok := ctx.Value(workflowConstitutionalStateKey{}).(*workflowConstitutionalState)
	if !ok || state == nil {
		return nil, fmt.Errorf("workflow constitutional guardrail state not initialized")
	}
	return state, nil
}

func getWorkflowDefineFailureState(ctx context.Context) (*workflowDefineFailureState, error) {
	state, ok := ctx.Value(workflowDefineFailureStateKey{}).(*workflowDefineFailureState)
	if !ok || state == nil {
		return nil, fmt.Errorf("workflow definition-failure guardrail state not initialized")
	}
	return state, nil
}
