package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
	"github.com/vbonnet/dear-agent/agm/internal/session"
)

type piCustomContextStateKey struct{}

type piCustomContextState struct {
	root       string
	markerPath string
	manifest   *manifest.Manifest
	usage      *manifest.ContextUsage
	priorDir   string
	hadPrior   bool
}

// RegisterPiCustomContextSteps registers Pi native-context regression steps.
func RegisterPiCustomContextSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, piCustomContextStateKey{}, &piCustomContextState{}), nil
	})
	ctx.After(func(ctx context.Context, _ *godog.Scenario, scenarioErr error) (context.Context, error) {
		state, ok := ctx.Value(piCustomContextStateKey{}).(*piCustomContextState)
		if !ok || state == nil {
			return ctx, scenarioErr
		}
		if state.root == "" {
			return ctx, scenarioErr
		}
		if state.hadPrior {
			_ = os.Setenv("PI_CODING_AGENT_DIR", state.priorDir)
		} else {
			_ = os.Unsetenv("PI_CODING_AGENT_DIR")
		}
		if err := os.RemoveAll(state.root); scenarioErr == nil && err != nil {
			return ctx, fmt.Errorf("remove Pi context fixture: %w", err)
		}
		return ctx, scenarioErr
	})

	ctx.Step(`^a managed Pi transcript uses provider "([^"]*)" model "([^"]*)"$`, managedPiTranscriptUsesModel)
	ctx.Step(`^a managed Pi transcript omits its provider for model "([^"]*)"$`, managedPiTranscriptOmitsProvider)
	ctx.Step(`^the Pi custom model catalog declares an (\d+) token window with an inert credential command$`, piCatalogDeclaresInertWindow)
	ctx.Step(`^the Pi custom model catalog declares an integral exponent context window$`, piCatalogDeclaresIntegralExponentWindow)
	ctx.Step(`^the status caller Pi catalog declares a (\d+) token window$`, statusCallerPiCatalogDeclaresWindow)
	ctx.Step(`^the Pi session persists native default configuration$`, piSessionPersistsNativeDefaultConfiguration)
	ctx.Step(`^the Pi session predates configuration presence metadata$`, piSessionPredatesConfigurationPresenceMetadata)
	ctx.Step(`^the Pi custom model catalog for provider "([^"]*)" declares model "([^"]*)" with an (\d+) token window$`, piCatalogDeclaresModelWindow)
	ctx.Step(`^the Pi custom model catalog declares a null context window$`, piCatalogDeclaresNullWindow)
	ctx.Step(`^two Pi catalog providers declare "([^"]*)" with the same (\d+) token window$`, piCatalogDeclaresEqualProviderDuplicates)
	ctx.Step(`^the Pi model catalog overrides "([^"]*)" to (\d+) tokens$`, piCatalogOverridesWindow)
	ctx.Step(`^AGM detects the managed Pi context$`, agmDetectsManagedPiContext)
	ctx.Step(`^the Pi context should report (\d+) of (\d+) tokens used$`, piContextShouldReportUsage)
	ctx.Step(`^the Pi context model should be "([^"]*)"$`, piContextModelShouldBe)
	ctx.Step(`^the Pi catalog credential command should remain inert$`, piCatalogCredentialCommandShouldRemainInert)
}

func managedPiTranscriptUsesModel(ctx context.Context, provider, model string) error {
	state, err := getPiCustomContextState(ctx)
	if err != nil {
		return err
	}
	if err := initializePiCustomContextFixture(state); err != nil {
		return err
	}
	sessionDir := filepath.Join(state.root, "sessions")
	if err := os.Mkdir(sessionDir, 0o700); err != nil {
		return err
	}
	transcriptPath := filepath.Join(sessionDir, "pi.jsonl")
	content := fmt.Sprintf("{\"type\":\"session\",\"id\":\"pi-bdd-context\",\"cwd\":\"/work\"}\n"+
		"{\"type\":\"message\",\"timestamp\":\"2026-07-22T10:11:52Z\",\"message\":{\"role\":\"assistant\",\"provider\":%q,\"model\":%q,\"usage\":{\"input\":3539,\"output\":4,\"cacheRead\":23}}}\n", provider, model)
	if err := os.WriteFile(transcriptPath, []byte(content), 0o600); err != nil {
		return err
	}
	state.manifest = &manifest.Manifest{Pi: &manifest.Pi{
		SessionID: "pi-bdd-context", SessionDir: sessionDir, TranscriptPath: transcriptPath,
		CodingAgentDir: state.root, CodingAgentDirSet: true,
	}}
	return nil
}

func managedPiTranscriptOmitsProvider(ctx context.Context, model string) error {
	state, err := getPiCustomContextState(ctx)
	if err != nil {
		return err
	}
	if err := initializePiCustomContextFixture(state); err != nil {
		return err
	}
	sessionDir := filepath.Join(state.root, "sessions")
	if err := os.Mkdir(sessionDir, 0o700); err != nil {
		return err
	}
	transcriptPath := filepath.Join(sessionDir, "pi.jsonl")
	content := fmt.Sprintf("{\"type\":\"session\",\"id\":\"pi-bdd-context\",\"cwd\":\"/work\"}\n"+
		"{\"type\":\"message\",\"timestamp\":\"2026-07-22T10:11:52Z\",\"message\":{\"role\":\"assistant\",\"model\":%q,\"usage\":{\"input\":3539,\"output\":4,\"cacheRead\":23}}}\n", model)
	if err := os.WriteFile(transcriptPath, []byte(content), 0o600); err != nil {
		return err
	}
	state.manifest = &manifest.Manifest{Pi: &manifest.Pi{
		SessionID: "pi-bdd-context", SessionDir: sessionDir, TranscriptPath: transcriptPath,
		CodingAgentDir: state.root, CodingAgentDirSet: true,
	}}
	return nil
}

func initializePiCustomContextFixture(state *piCustomContextState) error {
	root, err := os.MkdirTemp("", "agm-bdd-pi-context-")
	if err != nil {
		return fmt.Errorf("create Pi context fixture: %w", err)
	}
	priorDir, hadPrior := os.LookupEnv("PI_CODING_AGENT_DIR")
	if err := os.Setenv("PI_CODING_AGENT_DIR", root); err != nil {
		_ = os.RemoveAll(root)
		return fmt.Errorf("set Pi fixture directory: %w", err)
	}
	state.root, state.priorDir, state.hadPrior = root, priorDir, hadPrior
	return nil
}

func piCatalogDeclaresInertWindow(ctx context.Context, window int) error {
	state, err := getPiCustomContextState(ctx)
	if err != nil {
		return err
	}
	state.markerPath = filepath.Join(state.root, "credential-command-ran")
	catalog := fmt.Sprintf(`{"providers":{"ollama":{"apiKey":"!touch %s","models":[{"id":"qwen2.5-coder:7b","contextWindow":%d}]}}}`, state.markerPath, window)
	return os.WriteFile(filepath.Join(state.root, "models.json"), []byte(catalog), 0o600)
}

func piCatalogDeclaresIntegralExponentWindow(ctx context.Context) error {
	state, err := getPiCustomContextState(ctx)
	if err != nil {
		return err
	}
	catalog := `{"providers":{"ollama":{"models":[{"id":"qwen2.5-coder:7b","contextWindow":8.192e3}]}}}`
	return os.WriteFile(filepath.Join(state.root, "models.json"), []byte(catalog), 0o600)
}

func statusCallerPiCatalogDeclaresWindow(ctx context.Context, window int) error {
	state, err := getPiCustomContextState(ctx)
	if err != nil {
		return err
	}
	callerDir := filepath.Join(state.root, "status-caller")
	if err := os.Mkdir(callerDir, 0o700); err != nil {
		return err
	}
	catalog := fmt.Sprintf(`{"providers":{"ollama":{"models":[{"id":"qwen2.5-coder:7b","contextWindow":%d}]}}}`, window)
	if err := os.WriteFile(filepath.Join(callerDir, "models.json"), []byte(catalog), 0o600); err != nil {
		return err
	}
	return os.Setenv("PI_CODING_AGENT_DIR", callerDir)
}

func piSessionPersistsNativeDefaultConfiguration(ctx context.Context) error {
	state, err := getPiCustomContextState(ctx)
	if err != nil {
		return err
	}
	state.manifest.Pi.CodingAgentDir = ""
	state.manifest.Pi.CodingAgentDirSet = true
	return nil
}

func piSessionPredatesConfigurationPresenceMetadata(ctx context.Context) error {
	state, err := getPiCustomContextState(ctx)
	if err != nil {
		return err
	}
	state.manifest.Pi.CodingAgentDir = ""
	state.manifest.Pi.CodingAgentDirSet = false
	return nil
}

func piCatalogDeclaresModelWindow(ctx context.Context, provider, model string, window int) error {
	state, err := getPiCustomContextState(ctx)
	if err != nil {
		return err
	}
	catalog := fmt.Sprintf(`{"providers":{%q:{"models":[{"id":%q,"contextWindow":%d}]}}}`, provider, model, window)
	return os.WriteFile(filepath.Join(state.root, "models.json"), []byte(catalog), 0o600)
}

func piCatalogDeclaresNullWindow(ctx context.Context) error {
	state, err := getPiCustomContextState(ctx)
	if err != nil {
		return err
	}
	catalog := `{"providers":{"ollama":{"models":[{"id":"qwen2.5-coder:7b","contextWindow":null}]}}}`
	return os.WriteFile(filepath.Join(state.root, "models.json"), []byte(catalog), 0o600)
}

func piCatalogDeclaresEqualProviderDuplicates(ctx context.Context, model string, window int) error {
	state, err := getPiCustomContextState(ctx)
	if err != nil {
		return err
	}
	catalog := fmt.Sprintf(`{"providers":{"one":{"models":[{"id":%q,"contextWindow":%d}]},"two":{"models":[{"id":%q,"contextWindow":%d}]}}}`, model, window, model, window)
	return os.WriteFile(filepath.Join(state.root, "models.json"), []byte(catalog), 0o600)
}

func piCatalogOverridesWindow(ctx context.Context, qualifiedModel string, window int) error {
	state, err := getPiCustomContextState(ctx)
	if err != nil {
		return err
	}
	provider, model, ok := strings.Cut(qualifiedModel, "/")
	if !ok || provider == "" || model == "" {
		return fmt.Errorf("pi model %q is not provider-qualified", qualifiedModel)
	}
	catalog := fmt.Sprintf(`{"providers":{%q:{"modelOverrides":{%q:{"contextWindow":%d}}}}}`, provider, model, window)
	return os.WriteFile(filepath.Join(state.root, "models.json"), []byte(catalog), 0o600)
}

func agmDetectsManagedPiContext(ctx context.Context) error {
	state, err := getPiCustomContextState(ctx)
	if err != nil {
		return err
	}
	if state.manifest == nil {
		return fmt.Errorf("managed Pi transcript was not initialized")
	}
	state.usage, err = session.DetectContextFromManifestOrLog(state.manifest)
	return err
}

func piContextShouldReportUsage(ctx context.Context, used, total int) error {
	state, err := getPiCustomContextState(ctx)
	if err != nil {
		return err
	}
	if state.usage == nil {
		return fmt.Errorf("pi context usage was not detected")
	}
	if state.usage.UsedTokens != used || state.usage.TotalTokens != total {
		return fmt.Errorf("pi context usage = %d/%d, want %d/%d", state.usage.UsedTokens, state.usage.TotalTokens, used, total)
	}
	return nil
}

func piContextModelShouldBe(ctx context.Context, model string) error {
	state, err := getPiCustomContextState(ctx)
	if err != nil {
		return err
	}
	if state.usage == nil || state.usage.ModelID != model {
		return fmt.Errorf("pi context usage = %#v, want model %q", state.usage, model)
	}
	return nil
}

func piCatalogCredentialCommandShouldRemainInert(ctx context.Context) error {
	state, err := getPiCustomContextState(ctx)
	if err != nil {
		return err
	}
	if _, statErr := os.Stat(state.markerPath); statErr == nil {
		return fmt.Errorf("pi catalog credential command created marker %q", state.markerPath)
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("stat pi catalog credential command marker: %w", statErr)
	}
	return nil
}

func getPiCustomContextState(ctx context.Context) (*piCustomContextState, error) {
	state, ok := ctx.Value(piCustomContextStateKey{}).(*piCustomContextState)
	if !ok || state == nil {
		return nil, fmt.Errorf("pi custom context state not initialized")
	}
	return state, nil
}
