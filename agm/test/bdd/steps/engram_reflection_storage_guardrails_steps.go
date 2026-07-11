package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

const engramReflectionStorageFeaturePath = "agm/test/bdd/features/engram_reflection_storage_guardrails.feature"

type engramReflectionStorageGuardrailStateKey struct{}
type engramReflectionStorageSpecStateKey struct{}

type engramReflectionStorageSpecState struct {
	content  string
	path     string
	repoRoot string
}

// RegisterEngramReflectionStorageGuardrailSteps registers BDD steps for Engram learning packages.
func RegisterEngramReflectionStorageGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          engramReflectionStorageGuardrailStateKey{},
		label:             "Engram reflection storage package",
		featurePath:       engramReflectionStorageFeaturePath,
		configuredPattern: `^Engram reflection storage package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates Engram reflection storage package coverage$`,
		colocatedPattern:  `^Engram reflection storage package "([^"]*)" should have a co-located SPEC$`,
	})

	ctx.Before(func(ctx context.Context, sc *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, engramReflectionStorageSpecStateKey{}, &engramReflectionStorageSpecState{
			repoRoot: packageSpecBDDRepoRoot(),
		}), nil
	})

	ctx.Step(`^Engram reflection storage SPEC "([^"]*)" is loaded$`, loadEngramReflectionStorageSPEC)
	ctx.Step(`^the SPEC should require reflection session IDs to use only ASCII letters, ASCII digits, hyphen, or underscore$`, requireReflectionSessionIDAllowlist)
	ctx.Step(`^the SPEC should reject unsafe artifact identifiers before constructing artifact paths$`, requireArtifactIdentifierHardening)
	ctx.Step(`^the SPEC should require temporary files to be flushed before atomic replacement$`, requireTemporaryFileFlush)
	ctx.Step(`^the SPEC should require concurrent filesystem operations to be serialized$`, requireFilesystemSerialization)
}

func loadEngramReflectionStorageSPEC(ctx context.Context, specPath string) error {
	state, err := getEngramReflectionStorageSpecState(ctx)
	if err != nil {
		return err
	}
	if filepath.IsAbs(specPath) || strings.Contains(specPath, "..") {
		return fmt.Errorf("SPEC path %q must be repository-relative", specPath)
	}
	path := filepath.Join(state.repoRoot, filepath.FromSlash(specPath))
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read Engram reflection storage SPEC %s: %w", path, err)
	}
	state.path = specPath
	state.content = string(data)
	return nil
}

func requireReflectionSessionIDAllowlist(ctx context.Context) error {
	state, err := getLoadedEngramReflectionStorageSpecState(ctx)
	if err != nil {
		return err
	}
	return requireSPECText(state, "shorter than eight characters", "ASCII letters", "ASCII digits", "hyphen", "underscore")
}

func requireArtifactIdentifierHardening(ctx context.Context) error {
	state, err := getLoadedEngramReflectionStorageSpecState(ctx)
	if err != nil {
		return err
	}
	return requireSPECText(state, "empty", "relative", "traversing", "slash-delimited", "backslash-delimited", "null-containing", "_artifacts")
}

func requireTemporaryFileFlush(ctx context.Context) error {
	state, err := getLoadedEngramReflectionStorageSpecState(ctx)
	if err != nil {
		return err
	}
	return requireSPECText(state, "flush", "temporary file", "atomically renaming")
}

func requireFilesystemSerialization(ctx context.Context) error {
	state, err := getLoadedEngramReflectionStorageSpecState(ctx)
	if err != nil {
		return err
	}
	return requireSPECText(state, "concurrent", "artifact operations", "serialize access", "persisted filesystem state")
}

func requireSPECText(state *engramReflectionStorageSpecState, required ...string) error {
	for _, text := range required {
		if !strings.Contains(state.content, text) {
			return fmt.Errorf("engram reflection storage SPEC %s does not contain %q", state.path, text)
		}
	}
	return nil
}

func getLoadedEngramReflectionStorageSpecState(ctx context.Context) (*engramReflectionStorageSpecState, error) {
	state, err := getEngramReflectionStorageSpecState(ctx)
	if err != nil {
		return nil, err
	}
	if state.content == "" {
		return nil, fmt.Errorf("engram reflection storage SPEC was not loaded")
	}
	return state, nil
}

func getEngramReflectionStorageSpecState(ctx context.Context) (*engramReflectionStorageSpecState, error) {
	state, ok := ctx.Value(engramReflectionStorageSpecStateKey{}).(*engramReflectionStorageSpecState)
	if !ok || state == nil {
		return nil, fmt.Errorf("engram reflection storage SPEC state not initialized")
	}
	return state, nil
}
