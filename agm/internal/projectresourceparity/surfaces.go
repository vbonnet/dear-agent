// Package projectresourceparity records the harness-neutral capability matrix
// for repository-provided instructions, skills, configuration, and hooks.
package projectresourceparity

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/configdirparity"
	"github.com/vbonnet/dear-agent/agm/internal/marketplaceparity"
)

// Disposition records how a harness satisfies one project-resource capability.
type Disposition string

const (
	// DispositionSupported means native behavior directly satisfies the contract.
	DispositionSupported Disposition = "supported"
	// DispositionAdapted means an adapter supplies the observable outcome.
	DispositionAdapted Disposition = "adapted"
	// DispositionUnsupported means the capability belongs to the harness role but
	// the harness cannot represent it.
	DispositionUnsupported Disposition = "unsupported"
	// DispositionNotApplicable means the capability is outside the harness role.
	DispositionNotApplicable Disposition = "not-applicable"
)

// Capability describes one project-resource capability without implying that
// every harness implements it in the same way. Owner is a human-readable
// routing note; executable ownership remains in the referenced modules.
type Capability struct {
	Disposition Disposition
	Owner       string
}

// Surface records the bounded project-resource dispositions for one harness.
// Behavioral policy remains owned by the instruction, marketplace,
// configuration-directory, and hook modules named in Owner.
type Surface struct {
	Harness       string
	Instructions  Capability
	Skills        Capability
	Configuration Capability
	Hooks         Capability
}

// SurfaceForHarness returns the explicit project-resource dispositions for a
// supported harness.
func SurfaceForHarness(harness string) (Surface, bool) {
	switch agent.NormalizeHarnessName(harness) {
	case "claude-code":
		return Surface{
			Harness:       "claude-code",
			Instructions:  adapted("IEP-02: CLAUDE.md imports AGENTS.md"),
			Skills:        supported("MKT-08: .claude-plugin/marketplace.json"),
			Configuration: supported("CDP-03: .claude"),
			Hooks:         supported("HHP-01: .claude/settings.json"),
		}, true
	case "codex-cli":
		return Surface{
			Harness:       "codex-cli",
			Instructions:  supported("IEP-01: native root AGENTS.md"),
			Skills:        adapted("MKT-09: agents-md-skill-fallback"),
			Configuration: supported("CDP-04: .codex"),
			Hooks:         adapted("CHOOK-01 and CHOOK-08: attested .codex/hooks.json through AGM_CODEX_HOOK_ROOT"),
		}, true
	case "agy":
		return Surface{
			Harness:       "agy",
			Instructions:  adapted("IEP-05: AGY.md imports AGENTS.md"),
			Skills:        adapted("MKT-09: agents-md-skill-fallback"),
			Configuration: supported("CDP-05: .agents"),
			Hooks:         supported("HHP-01 and HHP-05: .agents/hooks.json"),
		}, true
	case "opencode-cli":
		return Surface{
			Harness:       "opencode-cli",
			Instructions:  adapted("IEP-06: OPENCODE.md imports AGENTS.md"),
			Skills:        adapted("MKT-09: agents-md-skill-fallback"),
			Configuration: supported("CDP-06: .opencode"),
			Hooks:         adapted("OPENCODE-DIR-01 through OPENCODE-DIR-04 and HHP-01: managed OpenCode hook projection"),
		}, true
	case "pi-cli":
		return Surface{
			Harness:       "pi-cli",
			Instructions:  supported("AGP-43: native root AGENTS.md; PRS-03: no divergent repository copy"),
			Skills:        adapted("MKT-09 and DEAR-MARKET-06: neutral catalog plus .pi/settings.json roots"),
			Configuration: supported("CDP-09: .pi"),
			Hooks:         adapted("HHP-08: managed extension projects .pi/hooks.json"),
		}, true
	default:
		return Surface{}, false
	}
}

// ActiveHarnessSurfaces returns one disposition record per active harness.
func ActiveHarnessSurfaces() []Surface {
	harnesses := agent.ActiveHarnesses()
	out := make([]Surface, 0, len(harnesses))
	for _, harness := range harnesses {
		if surface, ok := SurfaceForHarness(harness); ok {
			out = append(out, surface)
		}
	}
	return out
}

// ValidateActiveHarnessSurfaces rejects missing, implicit, or malformed
// dispositions and checks the registry against its canonical skill and
// configuration owners.
func ValidateActiveHarnessSurfaces() error {
	for _, harness := range agent.ActiveHarnesses() {
		surface, ok := SurfaceForHarness(harness)
		if !ok {
			return fmt.Errorf("active harness %q has no project resource surface", harness)
		}
		if surface.Harness != harness {
			return fmt.Errorf("active harness %q surface identifies %q", harness, surface.Harness)
		}
		for name, capability := range map[string]Capability{
			"instructions":  surface.Instructions,
			"skills":        surface.Skills,
			"configuration": surface.Configuration,
			"hooks":         surface.Hooks,
		} {
			if err := validateCapability(capability); err != nil {
				return fmt.Errorf("active harness %q %s capability: %w", harness, name, err)
			}
		}

		wantSkills := DispositionAdapted
		if marketplaceparity.ExpectedMarketplaceMode(harness) == "native-claude-plugin-marketplace" {
			wantSkills = DispositionSupported
		}
		if surface.Skills.Disposition != wantSkills {
			return fmt.Errorf("active harness %q skill disposition = %q, want %q for marketplace mode %q", harness, surface.Skills.Disposition, wantSkills, marketplaceparity.ExpectedMarketplaceMode(harness))
		}

		directory, ok := configdirparity.SurfaceForHarness(harness)
		if !ok {
			return fmt.Errorf("active harness %q has no configuration-directory surface", harness)
		}
		if surface.Configuration.Disposition != DispositionSupported || !strings.Contains(surface.Configuration.Owner, directory.Directory) {
			return fmt.Errorf("active harness %q configuration capability does not match %q: %+v", harness, directory.Directory, surface.Configuration)
		}
	}
	return nil
}

// ValidatePiInstructionSurface checks the cooperative repository checkout
// invariant uniquely owned by this package: root AGENTS.md is present and no
// divergent .pi/AGENTS.md copy is published.
func ValidatePiInstructionSurface(root string) error {
	resolvedRoot, err := resolveRepositoryRoot(root)
	if err != nil {
		return err
	}
	if _, err := requireContainedRegularFile(resolvedRoot, filepath.Join(resolvedRoot, "AGENTS.md")); err != nil {
		return fmt.Errorf("shared instruction entrypoint: %w", err)
	}
	piInstructions := filepath.Join(resolvedRoot, ".pi", "AGENTS.md")
	if _, err := os.Lstat(piInstructions); err == nil {
		return fmt.Errorf("repository must publish root AGENTS.md instead of divergent %s", piInstructions)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect pi instruction entrypoint: %w", err)
	}
	return nil
}

func supported(owner string) Capability {
	return Capability{Disposition: DispositionSupported, Owner: owner}
}

func adapted(owner string) Capability {
	return Capability{Disposition: DispositionAdapted, Owner: owner}
}

func validateCapability(capability Capability) error {
	if !slices.Contains([]Disposition{
		DispositionSupported,
		DispositionAdapted,
		DispositionUnsupported,
		DispositionNotApplicable,
	}, capability.Disposition) {
		return fmt.Errorf("invalid disposition %q", capability.Disposition)
	}
	if strings.TrimSpace(capability.Owner) == "" {
		return fmt.Errorf("missing behavioral-owner routing note")
	}
	return nil
}

func resolveRepositoryRoot(root string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("make repository root absolute: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", fmt.Errorf("resolve repository root: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("stat repository root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("repository root is not a directory")
	}
	return resolved, nil
}

func requireContainedRegularFile(resolvedRoot, candidate string) (string, error) {
	info, err := os.Lstat(candidate)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", candidate)
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", err
	}
	if !containedWithin(resolvedRoot, resolved) {
		return "", fmt.Errorf("%s escapes the repository", candidate)
	}
	return resolved, nil
}

func containedWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
