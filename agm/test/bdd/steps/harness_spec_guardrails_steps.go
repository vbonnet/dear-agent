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
	duplicateIDs       []string
	ownershipErr       error
	resumeOwnerOK      bool
	harnessBoundaryErr error
	apiDeliveryErr     error
	constructionErr    error
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
	ctx.Step(`^AGM harness adapter contract sources$`, agmHarnessAdapterContractSources)
	ctx.Step(`^AGM validates harness capability ownership$`, agmValidatesHarnessCapabilityOwnership)
	ctx.Step(`^harness discovery should expose metadata without a universal lifecycle facade$`, harnessDiscoveryShouldExposeMetadataWithoutAUniversalLifecycleFacade)
	ctx.Step(`^pure API delivery should require only context-aware readiness and message delivery$`, pureAPIDeliveryShouldRequireOnlyContextAwareReadinessAndMessageDelivery)
	ctx.Step(`^adapter constructors should return concrete types from one finite discovery catalog$`, adapterConstructorsShouldReturnConcreteTypesFromOneFiniteDiscoveryCatalog)
}

func agmHarnessParitySpecificationAndLifecycleSurfaces(context.Context) error {
	return nil
}

func agmHarnessAdapterContractSources(context.Context) error {
	return nil
}

func agmValidatesHarnessCapabilityOwnership(ctx context.Context) error {
	state, err := requireHarnessSpecGuardrailState(ctx)
	if err != nil {
		return err
	}
	root := packageSpecBDDRepoRoot()
	state.harnessBoundaryErr = requireMetadataOnlyHarnessBoundary(root)
	state.apiDeliveryErr = requireAPIDeliveryBoundary(root)
	state.constructionErr = requireConcreteHarnessConstruction(root)
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

func requireMetadataOnlyHarnessBoundary(root string) error {
	interfaceSource, err := os.ReadFile(filepath.Join(root, "agm", "internal", "agent", "interface.go"))
	if err != nil {
		return fmt.Errorf("read harness interface source: %w", err)
	}
	source := string(interfaceSource)
	if strings.Contains(source, "type Agent interface") {
		return fmt.Errorf("agent package still exposes a universal Agent lifecycle facade")
	}
	start := strings.Index(source, "type Harness interface {")
	if start < 0 {
		return fmt.Errorf("agent package does not expose the Harness metadata contract")
	}
	end := strings.Index(source[start:], "\n}")
	if end < 0 {
		return fmt.Errorf("harness metadata contract is not terminated")
	}
	block := source[start : start+end]
	for _, method := range []string{"Name() string", "Version() string", "Capabilities() Capabilities"} {
		if !strings.Contains(block, method) {
			return fmt.Errorf("harness metadata contract is missing %s", method)
		}
	}
	for _, method := range []string{
		"CreateSession(",
		"ResumeSession(",
		"TerminateSession(",
		"GetSessionStatus(",
		"SendMessage(",
		"GetHistory(",
		"ExportConversation(",
		"ImportConversation(",
		"ExecuteCommand(",
	} {
		if strings.Contains(block, method) {
			return fmt.Errorf("harness metadata contract includes lifecycle method %s", method)
		}
	}

	workflowSource, err := os.ReadFile(filepath.Join(root, "agm", "internal", "workflow", "interface.go"))
	if err != nil {
		return fmt.Errorf("read workflow harness boundary: %w", err)
	}
	if !strings.Contains(string(workflowSource), "Harness agent.Harness") {
		return fmt.Errorf("workflow selection does not use the metadata-only Harness contract")
	}
	return nil
}

func requireAPIDeliveryBoundary(root string) error {
	data, err := os.ReadFile(filepath.Join(root, "agm", "internal", "ops", "api_session_delivery.go"))
	if err != nil {
		return fmt.Errorf("read API delivery boundary: %w", err)
	}
	source := string(data)
	for _, contract := range []string{
		"type APISessionDeliveryAdapter interface {",
		"agent.ContextSessionStatusGetter",
		"agent.ContextMessageSender",
		"(APISessionDeliveryAdapter, error)",
	} {
		if !strings.Contains(source, contract) {
			return fmt.Errorf("API delivery boundary is missing %s", contract)
		}
	}
	if strings.Contains(source, "adapter.(") {
		return fmt.Errorf("API delivery still discovers required capabilities through runtime type assertions")
	}
	return nil
}

func requireConcreteHarnessConstruction(root string) error {
	signatures := map[string][]string{
		"agm/internal/agent/claude_adapter.go": {
			"func NewClaudeAdapter(sessionStore SessionStore) (*ClaudeAdapter, error)",
		},
		"agm/internal/agent/gemini_cli_adapter.go": {
			"func NewGeminiCLIAdapter(sessionStore SessionStore) (*GeminiCLIAdapter, error)",
		},
		"agm/internal/agent/codex_cli_adapter.go": {
			"func NewCodexCLIAdapter(sessionStore SessionStore) (*CodexCLIAdapter, error)",
		},
		"agm/internal/agent/opencode_adapter.go": {
			"func NewOpenCodeAdapter(config *OpenCodeConfig) (*OpenCodeAdapter, error)",
		},
		"agm/internal/agent/agy_adapter.go": {
			"func NewAgyAdapter(sessionStore SessionStore) (*AgyAdapter, error)",
		},
		"agm/internal/agent/pi_adapter.go": {
			"func NewPiAdapter(sessionStore SessionStore) (*PiAdapter, error)",
		},
		"agm/internal/agent/openai_adapter.go": {
			"func NewOpenAIAdapter(ctx context.Context, config *OpenAIConfig) (*OpenAIAdapter, error)",
			"func NewOpenAIAdapterForSession(ctx context.Context, sessionID SessionID, config *OpenAIConfig) (*OpenAIAdapter, error)",
		},
	}
	for path, required := range signatures {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return fmt.Errorf("read adapter constructor %s: %w", path, err)
		}
		for _, signature := range required {
			if !strings.Contains(string(data), signature) {
				return fmt.Errorf("%s does not return its concrete adapter: missing %s", path, signature)
			}
		}
	}

	if _, err := os.Stat(filepath.Join(root, "agm", "internal", "agent", "registry.go")); !os.IsNotExist(err) {
		if err != nil {
			return fmt.Errorf("inspect duplicate runtime registry: %w", err)
		}
		return fmt.Errorf("duplicate mutable runtime adapter registry still exists")
	}
	factorySource, err := os.ReadFile(filepath.Join(root, "agm", "internal", "agent", "factory.go"))
	if err != nil {
		return fmt.Errorf("read harness discovery catalog: %w", err)
	}
	factory := string(factorySource)
	if !strings.Contains(factory, "func newHarnessWithStore(name string, store SessionStore) (Harness, error)") {
		return fmt.Errorf("finite harness discovery catalog is missing")
	}
	for _, harness := range []string{
		`case "claude-code":`,
		`case "gemini-cli":`,
		`case "codex-cli":`,
		`case "opencode-cli":`,
		`case "agy":`,
		`case "pi-cli":`,
	} {
		if !strings.Contains(factory, harness) {
			return fmt.Errorf("finite harness discovery catalog is missing %s", harness)
		}
	}
	if strings.Contains(factory, "func Register(") || strings.Contains(factory, "map[string]func()") {
		return fmt.Errorf("harness discovery catalog is mutable at runtime")
	}
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
			if strings.Contains(string(data), call) {
				continue
			}
			// ops.CreateSessionRouted is the MCP surface's routing wrapper over
			// ops.CreateSessionWithContext (agy retry/backoff + codex fallback);
			// it is safe only on that self-contained create path, not the
			// interactive CLI flows. Accept it as delegation ONLY for the MCP
			// server; every other lifecycle surface must call the direct
			// entrypoint.
			if path == "agm/cmd/agm-mcp-server/tools.go" &&
				call == "ops.CreateSessionWithContext(" &&
				strings.Contains(string(data), "ops.CreateSessionRouted(") {
				continue
			}
			return fmt.Errorf("%s does not delegate through %s", path, call)
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

func harnessDiscoveryShouldExposeMetadataWithoutAUniversalLifecycleFacade(ctx context.Context) error {
	state, err := requireHarnessSpecGuardrailState(ctx)
	if err != nil {
		return err
	}
	return state.harnessBoundaryErr
}

func pureAPIDeliveryShouldRequireOnlyContextAwareReadinessAndMessageDelivery(ctx context.Context) error {
	state, err := requireHarnessSpecGuardrailState(ctx)
	if err != nil {
		return err
	}
	return state.apiDeliveryErr
}

func adapterConstructorsShouldReturnConcreteTypesFromOneFiniteDiscoveryCatalog(ctx context.Context) error {
	state, err := requireHarnessSpecGuardrailState(ctx)
	if err != nil {
		return err
	}
	return state.constructionErr
}

func requireHarnessSpecGuardrailState(ctx context.Context) (*harnessSpecGuardrailState, error) {
	state, ok := ctx.Value(harnessSpecGuardrailStateKey{}).(*harnessSpecGuardrailState)
	if !ok || state == nil {
		return nil, fmt.Errorf("harness SPEC guardrail state is not initialized")
	}
	return state, nil
}
