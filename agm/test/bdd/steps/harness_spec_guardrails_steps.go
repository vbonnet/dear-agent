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
	ctx.Step(`^CLI, MCP, and daemon lifecycle surfaces should delegate to shared operations$`, lifecycleSurfacesShouldDelegateToSharedOperations)
	ctx.Step(`^CLI resume should delegate its transaction to shared operations$`, cliResumeShouldDelegateItsTransactionToSharedOperations)
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
		return fmt.Errorf("read CLI resume adapter: %w", err)
	}
	operationSource, err := os.ReadFile(filepath.Join(root, "agm", "internal", "ops", "session_resume.go"))
	if err != nil {
		return fmt.Errorf("read shared resume owner: %w", err)
	}
	state.resumeOwnerOK =
		strings.Count(string(resumeSource), "ops.ResumeSession(") == 1 &&
			!strings.Contains(string(resumeSource), "resumeSessionTransactionWithRuntime(") &&
			strings.Contains(string(operationSource), "func ResumeSession(") &&
			strings.Contains(string(operationSource), "WithSessionLockContext(ctx, req.SessionID")
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
		"agm/internal/daemon/daemon.go": {
			"deliverDirect: ops.SendMessage",
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
	for path, forbidden := range map[string][]string{
		"agm/cmd/agm/send_msg.go": {
			"session.CheckSessionDelivery(",
		},
		"agm/internal/daemon/daemon.go": {
			"session.CheckSessionDelivery(",
			"sendPrompt",
			"SendMultiLinePromptSafeForHarness",
		},
	} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return fmt.Errorf("read lifecycle surface %s: %w", path, err)
		}
		for _, call := range forbidden {
			if strings.Contains(string(data), call) {
				return fmt.Errorf("%s retains superseded delivery authority %s", path, call)
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

func lifecycleSurfacesShouldDelegateToSharedOperations(ctx context.Context) error {
	state, err := requireHarnessSpecGuardrailState(ctx)
	if err != nil {
		return err
	}
	return state.ownershipErr
}

func cliResumeShouldDelegateItsTransactionToSharedOperations(ctx context.Context) error {
	state, err := requireHarnessSpecGuardrailState(ctx)
	if err != nil {
		return err
	}
	if !state.resumeOwnerOK {
		return fmt.Errorf("resume lifecycle is not owned by one stable-ID shared operation with a single CLI delegation")
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
