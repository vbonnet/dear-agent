package steps

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
	"github.com/vbonnet/dear-agent/agm/internal/history"
)

type agmConversationDiscoveryGuardrailStateKey struct{}
type agyHistoryBDDStateKey struct{}

type agyHistoryBDDState struct {
	conversationID string
	location       *history.HistoryLocation
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
		return context.WithValue(ctx, agyHistoryBDDStateKey{}, &agyHistoryBDDState{}), nil
	})
	ctx.Step(`^an AGY native conversation ID$`, anAgyNativeConversationID)
	ctx.Step(`^AGM resolves AGY conversation history paths$`, agmResolvesAgyConversationHistoryPaths)
	ctx.Step(`^AGY history should include the native conversation database$`, agyHistoryShouldIncludeNativeConversationDatabase)
	ctx.Step(`^AGY history should include compact and full transcripts$`, agyHistoryShouldIncludeCompactAndFullTranscripts)
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
