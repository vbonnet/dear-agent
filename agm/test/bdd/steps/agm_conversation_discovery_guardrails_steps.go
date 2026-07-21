package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
	"github.com/vbonnet/dear-agent/agm/internal/history"
	"github.com/vbonnet/dear-agent/agm/internal/pisession"
)

type agmConversationDiscoveryGuardrailStateKey struct{}
type agyHistoryBDDStateKey struct{}
type piImportModelBDDStateKey struct{}

type agyHistoryBDDState struct {
	conversationID string
	location       *history.HistoryLocation
}

type piImportModelBDDState struct {
	tempDir            string
	withModelPath      string
	withoutModelPath   string
	withModelResult    string
	withoutModelResult string
}

// RegisterAGMConversationDiscoveryGuardrailSteps registers conversation package coverage steps.
func RegisterAGMConversationDiscoveryGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          agmConversationDiscoveryGuardrailStateKey{},
		label:             "AGM conversation package",
		featurePath:       "agm/test/bdd/features/agm_conversation_discovery_guardrails.feature",
		configuredPattern: `^AGM conversation package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates conversation package coverage$`,
		colocatedPattern:  `^AGM conversation package "([^"]*)" should have a co-located SPEC$`,
	})
	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		ctx = context.WithValue(ctx, agyHistoryBDDStateKey{}, &agyHistoryBDDState{})
		return context.WithValue(ctx, piImportModelBDDStateKey{}, &piImportModelBDDState{}), nil
	})
	ctx.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		state, _ := ctx.Value(piImportModelBDDStateKey{}).(*piImportModelBDDState)
		if state != nil && state.tempDir != "" {
			return ctx, os.RemoveAll(state.tempDir)
		}
		return ctx, nil
	})
	ctx.Step(`^an AGY native conversation ID$`, anAgyNativeConversationID)
	ctx.Step(`^AGM resolves AGY conversation history paths$`, agmResolvesAgyConversationHistoryPaths)
	ctx.Step(`^AGY history should include the native conversation database$`, agyHistoryShouldIncludeNativeConversationDatabase)
	ctx.Step(`^AGY history should include compact and full transcripts$`, agyHistoryShouldIncludeCompactAndFullTranscripts)
	ctx.Step(`^Pi transcripts with and without native model provenance$`, piTranscriptsWithAndWithoutNativeModelProvenance)
	ctx.Step(`^AGM reads Pi import model provenance$`, agmReadsPiImportModelProvenance)
	ctx.Step(`^AGM should preserve the provider-qualified Pi model$`, agmShouldPreserveTheProviderQualifiedPiModel)
	ctx.Step(`^AGM should leave the Pi model empty when provenance is absent$`, agmShouldLeaveThePiModelEmptyWhenProvenanceIsAbsent)
}

func piTranscriptsWithAndWithoutNativeModelProvenance(ctx context.Context) error {
	state, err := getPiImportModelBDDState(ctx)
	if err != nil {
		return err
	}
	state.tempDir, err = os.MkdirTemp("", "agm-pi-model-bdd-")
	if err != nil {
		return err
	}
	state.withModelPath = filepath.Join(state.tempDir, "with-model.jsonl")
	state.withoutModelPath = filepath.Join(state.tempDir, "without-model.jsonl")
	withModel := `{"type":"model_change","provider":"openai","modelId":"gpt-5.6-terra"}` + "\n"
	withoutModel := `{"type":"message","message":{"role":"user","content":"hello"}}` + "\n"
	if err := os.WriteFile(state.withModelPath, []byte(withModel), 0o600); err != nil {
		return err
	}
	return os.WriteFile(state.withoutModelPath, []byte(withoutModel), 0o600)
}

func agmReadsPiImportModelProvenance(ctx context.Context) error {
	state, err := getPiImportModelBDDState(ctx)
	if err != nil {
		return err
	}
	state.withModelResult, err = pisession.ReadModel(state.withModelPath)
	if err != nil {
		return err
	}
	state.withoutModelResult, err = pisession.ReadModel(state.withoutModelPath)
	return err
}

func agmShouldPreserveTheProviderQualifiedPiModel(ctx context.Context) error {
	state, err := getPiImportModelBDDState(ctx)
	if err != nil {
		return err
	}
	if state.withModelResult != "openai/gpt-5.6-terra" {
		return fmt.Errorf("pi native model = %q, want provider-qualified model", state.withModelResult)
	}
	return nil
}

func agmShouldLeaveThePiModelEmptyWhenProvenanceIsAbsent(ctx context.Context) error {
	state, err := getPiImportModelBDDState(ctx)
	if err != nil {
		return err
	}
	if state.withoutModelResult != "" {
		return fmt.Errorf("pi model without provenance = %q, want empty", state.withoutModelResult)
	}
	return nil
}

func anAgyNativeConversationID(ctx context.Context) error {
	state, err := getAgyHistoryBDDState(ctx)
	if err != nil {
		return err
	}
	state.conversationID = "117ff898-a964-4a9f-b460-1be4a8a49b17"
	return nil
}

func agmResolvesAgyConversationHistoryPaths(ctx context.Context) error {
	state, err := getAgyHistoryBDDState(ctx)
	if err != nil {
		return err
	}
	location, err := history.GetHistoryPaths("agy", state.conversationID, "", false)
	if err != nil {
		return err
	}
	state.location = location
	return nil
}

func agyHistoryShouldIncludeNativeConversationDatabase(ctx context.Context) error {
	state, err := getAgyHistoryBDDState(ctx)
	if err != nil {
		return err
	}
	if state.location == nil || len(state.location.Paths) != 3 {
		return fmt.Errorf("AGY history paths = %+v, want three native locations", state.location)
	}
	wantSuffix := filepath.Join(".gemini", "antigravity-cli", "conversations", state.conversationID+".db")
	if !strings.HasSuffix(state.location.Paths[0], wantSuffix) {
		return fmt.Errorf("AGY conversation DB = %q, want suffix %q", state.location.Paths[0], wantSuffix)
	}
	return nil
}

func agyHistoryShouldIncludeCompactAndFullTranscripts(ctx context.Context) error {
	state, err := getAgyHistoryBDDState(ctx)
	if err != nil {
		return err
	}
	if state.location == nil || len(state.location.Paths) != 3 {
		return fmt.Errorf("AGY history paths = %+v, want three native locations", state.location)
	}
	for i, name := range []string{"transcript.jsonl", "transcript_full.jsonl"} {
		wantSuffix := filepath.Join("brain", state.conversationID, ".system_generated", "logs", name)
		if !strings.HasSuffix(state.location.Paths[i+1], wantSuffix) {
			return fmt.Errorf("AGY transcript path = %q, want suffix %q", state.location.Paths[i+1], wantSuffix)
		}
	}
	return nil
}

func getAgyHistoryBDDState(ctx context.Context) (*agyHistoryBDDState, error) {
	state, ok := ctx.Value(agyHistoryBDDStateKey{}).(*agyHistoryBDDState)
	if !ok || state == nil {
		return nil, fmt.Errorf("AGY history BDD state not initialized")
	}
	return state, nil
}

func getPiImportModelBDDState(ctx context.Context) (*piImportModelBDDState, error) {
	state, ok := ctx.Value(piImportModelBDDStateKey{}).(*piImportModelBDDState)
	if !ok || state == nil {
		return nil, fmt.Errorf("pi import model BDD state not initialized")
	}
	return state, nil
}
