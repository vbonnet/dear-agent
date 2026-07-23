package steps

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cucumber/godog"
	"github.com/vbonnet/dear-agent/internal/earsbdd"
)

type harnessSpecGuardrailState struct {
	duplicateIDs  []string
	ownershipErr  error
	resumeOwnerOK bool
}

type harnessSpecGuardrailStateKey struct{}

// RegisterHarnessSpecGuardrailSteps registers structural checks for the
// harness specification and its production lifecycle owners.
func RegisterHarnessSpecGuardrailSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, harnessSpecGuardrailStateKey{}, &harnessSpecGuardrailState{}), nil
	})

	ctx.Step(`^AGM harness parity specification and lifecycle surfaces$`, agmHarnessParitySpecificationAndLifecycleSurfaces)
	ctx.Step(`^AGM validates harness requirement identifiers and lifecycle ownership$`, agmValidatesHarnessRequirementIdentifiersAndLifecycleOwnership)
	ctx.Step(`^harness requirement identifiers should be unique$`, harnessRequirementIdentifiersShouldBeUnique)
	ctx.Step(`^CLI and MCP lifecycle surfaces should delegate to shared operations$`, cliAndMCPLifecycleSurfacesShouldDelegateToSharedOperations)
	ctx.Step(`^CLI resume should retain its focused transactional owner$`, cliResumeShouldRetainItsFocusedTransactionalOwner)
}

func agmHarnessParitySpecificationAndLifecycleSurfaces(context.Context) error {
	return nil
}

func agmValidatesHarnessRequirementIdentifiersAndLifecycleOwnership(ctx context.Context) error {
	state, err := requireHarnessSpecGuardrailState(ctx)
	if err != nil {
		return err
	}
	root := packageSpecBDDRepoRoot()

	requirements, err := earsbdd.ExtractFile(filepath.Join(root, "agm", "internal", "agent", "SPEC.md"))
	if err != nil {
		return fmt.Errorf("extract harness parity requirements: %w", err)
	}
	state.duplicateIDs = duplicateHarnessRequirementIDs(requirements)

	state.ownershipErr = requireLifecycleDelegation(root)
	resumeSource, err := os.ReadFile(filepath.Join(root, "agm", "cmd", "agm", "resume.go"))
	if err != nil {
		return fmt.Errorf("read CLI resume owner: %w", err)
	}
	state.resumeOwnerOK = strings.Contains(string(resumeSource), "resumeSessionTransactionWithRuntime(")
	return nil
}

func duplicateHarnessRequirementIDs(requirements []earsbdd.Requirement) []string {
	counts := make(map[string]int)
	for _, requirement := range requirements {
		if strings.HasPrefix(requirement.ID, "AGP-") {
			counts[requirement.ID]++
		}
	}
	var duplicates []string
	for id, count := range counts {
		if count > 1 {
			duplicates = append(duplicates, id)
		}
	}
	slices.Sort(duplicates)
	return duplicates
}

func requireLifecycleDelegation(root string) error {
	checks := map[string][]string{
		"agm/cmd/agm/create_child.go": {
			"ops.CreateSessionWithContext(",
		},
		"agm/cmd/agm/new_session.go": {
			"ops.CreateSessionWithContext(",
		},
		"agm/cmd/agm/new_currenttmux.go": {
			"ops.CreateSessionWithContext(",
		},
		"agm/cmd/agm/kill.go": {
			"ops.KillSession(",
		},
		"agm/cmd/agm/archive.go": {
			"ops.ArchiveSession(",
		},
		"agm/cmd/agm/send_msg.go": {
			"ops.SendMessage(",
		},
		"agm/cmd/agm-mcp-server/tools.go": {
			"ops.CreateSessionWithContext(",
			"ops.KillSession(",
			"ops.ArchiveSession(",
			"ops.SendMessage(",
		},
	}
	for path, required := range checks {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return fmt.Errorf("read lifecycle surface %s: %w", path, err)
		}
		for _, call := range required {
			if !strings.Contains(string(data), call) {
				return fmt.Errorf("%s does not delegate through %s", path, call)
			}
		}
	}
	return nil
}

func harnessRequirementIdentifiersShouldBeUnique(ctx context.Context) error {
	state, err := requireHarnessSpecGuardrailState(ctx)
	if err != nil {
		return err
	}
	if len(state.duplicateIDs) != 0 {
		return fmt.Errorf("duplicate harness requirement identifiers: %s", strings.Join(state.duplicateIDs, ", "))
	}
	return nil
}

func cliAndMCPLifecycleSurfacesShouldDelegateToSharedOperations(ctx context.Context) error {
	state, err := requireHarnessSpecGuardrailState(ctx)
	if err != nil {
		return err
	}
	return state.ownershipErr
}

func cliResumeShouldRetainItsFocusedTransactionalOwner(ctx context.Context) error {
	state, err := requireHarnessSpecGuardrailState(ctx)
	if err != nil {
		return err
	}
	if !state.resumeOwnerOK {
		return fmt.Errorf("CLI resume no longer exposes its focused transaction owner")
	}
	return nil
}

func requireHarnessSpecGuardrailState(ctx context.Context) (*harnessSpecGuardrailState, error) {
	state, ok := ctx.Value(harnessSpecGuardrailStateKey{}).(*harnessSpecGuardrailState)
	if !ok || state == nil {
		return nil, fmt.Errorf("harness SPEC guardrail state is not initialized")
	}
	return state, nil
}
