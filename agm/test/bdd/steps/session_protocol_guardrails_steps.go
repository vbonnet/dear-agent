package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	upstream "github.com/a2aproject/a2a-go/a2a"
	"github.com/cucumber/godog"

	deara2a "github.com/vbonnet/dear-agent/pkg/a2a"
)

const sessionProtocolFeaturePath = "agm/test/bdd/features/session_protocol_guardrails.feature"

type sessionProtocolGuardrailStateKey struct{}
type a2aCardParityStateKey struct{}

type a2aCardParityState struct {
	harness string
	family  string
	card    *upstream.AgentCard
}

// RegisterSessionProtocolGuardrailSteps registers shared package coverage and
// cross-route A2A card parity steps.
func RegisterSessionProtocolGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          sessionProtocolGuardrailStateKey{},
		label:             "session protocol package",
		featurePath:       sessionProtocolFeaturePath,
		configuredPattern: `^session protocol package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates session protocol package coverage$`,
		colocatedPattern:  `^session protocol package "([^"]*)" should have a co-located SPEC$`,
	})

	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, a2aCardParityStateKey{}, &a2aCardParityState{}), nil
	})

	ctx.Step(`^harness "([^"]*)" and model family "([^"]*)" expose an A2A session$`,
		func(ctx context.Context, harness, family string) error {
			state, err := getA2ACardParityState(ctx)
			if err != nil {
				return err
			}
			state.harness = harness
			state.family = family
			return nil
		})

	ctx.Step(`^the shared A2A session card is built$`, func(ctx context.Context) error {
		state, err := getA2ACardParityState(ctx)
		if err != nil {
			return err
		}
		state.card = deara2a.SessionCard{Harness: state.harness, SessionID: "bdd-session"}.Build()
		return nil
	})

	ctx.Step(`^the A2A card should advertise only harness "([^"]*)"$`,
		func(ctx context.Context, harness string) error {
			state, err := getA2ACardParityState(ctx)
			if err != nil {
				return err
			}
			if state.card == nil || len(state.card.Skills) != 1 {
				return fmt.Errorf("A2A card does not have one default skill")
			}
			tags := state.card.Skills[0].Tags
			if !slices.Contains(tags, harness) {
				return fmt.Errorf("A2A card tags %v omit harness %q", tags, harness)
			}
			for _, active := range []string{"claude-code", "codex-cli", "agy", "opencode-cli", "pi-cli"} {
				if active != harness && slices.Contains(tags, active) {
					return fmt.Errorf("A2A card for %q also advertises %q", harness, active)
				}
			}
			return nil
		})

	ctx.Step(`^the A2A card presentation should not encode model family "([^"]*)"$`,
		func(ctx context.Context, family string) error {
			state, err := getA2ACardParityState(ctx)
			if err != nil {
				return err
			}
			var presentation strings.Builder
			presentation.WriteString(state.card.Name)
			presentation.WriteByte(' ')
			presentation.WriteString(state.card.Description)
			for _, skill := range state.card.Skills {
				fmt.Fprintf(&presentation, " %s %s %s", skill.Name, skill.Description, strings.Join(skill.Tags, " "))
			}
			text := presentation.String()
			if strings.Contains(strings.ToLower(text), strings.ToLower(family)) {
				return fmt.Errorf("A2A card presentation encodes model family %q: %q", family, text)
			}
			return nil
		})

	ctx.Step(`^agent trace redaction policy is configured$`, agentTraceRedactionPolicyIsConfigured)
	ctx.Step(`^AGM validates nested trace redaction traversal$`, agmValidatesNestedTraceRedactionTraversal)
	ctx.Step(`^every nested trace value should be inspected without per-key normalizer allocation$`, nestedTraceValuesUseStableRedactionTraversal)
}

func agentTraceRedactionPolicyIsConfigured() error {
	_, err := os.Stat(filepath.Join(packageSpecBDDRepoRoot(), "pkg", "agenttrace", "redact.go"))
	return err
}

func agmValidatesNestedTraceRedactionTraversal() error {
	return nil
}

func nestedTraceValuesUseStableRedactionTraversal() error {
	path := filepath.Join(packageSpecBDDRepoRoot(), "pkg", "agenttrace", "redact.go")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	source := string(data)
	for _, required := range []string{"keyNormalizer", "if redactJSONValue(child)"} {
		if !strings.Contains(source, required) {
			return fmt.Errorf("trace redaction policy lacks %q", required)
		}
	}
	if strings.Contains(source, "redactJSONValue(child) || changed") {
		return fmt.Errorf("trace redaction traversal relies on short-circuit expression")
	}
	return nil
}

func getA2ACardParityState(ctx context.Context) (*a2aCardParityState, error) {
	state, ok := ctx.Value(a2aCardParityStateKey{}).(*a2aCardParityState)
	if !ok || state == nil {
		return nil, fmt.Errorf("A2A card parity state not initialized")
	}
	return state, nil
}
