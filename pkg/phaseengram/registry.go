package phaseengram

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vbonnet/dear-agent/pkg/hash"
)

// Phase-to-engram file mappings. The engram directory is relative to the engram repo root.
const engramWorkflowDir = "core/cortex/engrams/workflows"

// phaseToEngram maps phase identifiers to their engram filenames.
var phaseToEngram = map[string]string{
	"CHARTER":  "w0-project-framing.ai.md",
	"W0":       "w0-project-framing.ai.md",
	"PROBLEM":  "d1-problem-validation.ai.md",
	"D1":       "d1-problem-validation.ai.md",
	"RESEARCH": "d2-existing-solutions.ai.md",
	"D2":       "d2-existing-solutions.ai.md",
	"DECISION": "d3-approach-decision.ai.md",
	"D3":       "d3-approach-decision.ai.md",
	"SPEC":     "d4-solution-requirements.ai.md",
	"D4":       "d4-solution-requirements.ai.md",
	"S4":       "s4-stakeholder-alignment.ai.md",
	"S5":       "s5-research.ai.md",
	"DESIGN":   "s6-design.ai.md",
	"S6":       "s6-design.ai.md",
	"PLAN":     "s7-plan.ai.md",
	"S7":       "s7-plan.ai.md",
	"BUILD":    "s8-build.ai.md",
	"S8":       "s8-build.ai.md",
	"S9":       "s9-validation.ai.md",
	"S10":      "s10-deploy.ai.md",
	"RETRO":    "s11-retrospective.ai.md",
	"S11":      "s11-retrospective.ai.md",
}

// ResolveEngramPath returns the absolute path to the engram file for a given phase.
// It searches for the engram repo root by looking for the workflow directory
// in known locations.
func ResolveEngramPath(phase string) (string, error) {
	filename, ok := phaseToEngram[strings.ToUpper(phase)]
	if !ok {
		return "", fmt.Errorf("unknown phase %q; known phases: %s", phase, knownPhases())
	}

	repoRoot, err := findEngramRepoRoot()
	if err != nil {
		return "", fmt.Errorf("cannot locate engram repo: %w", err)
	}

	path := filepath.Join(repoRoot, engramWorkflowDir, filename)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("engram file not found at %s: %w", path, err)
	}

	return path, nil
}

// ResolveEngramHash computes the SHA-256 hash of the engram file for a given phase.
func ResolveEngramHash(phase string) (string, error) {
	path, err := ResolveEngramPath(phase)
	if err != nil {
		return "", err
	}
	return hash.CalculateFileHash(path)
}

// ResolveEngramPathAndHash returns both the path and hash for a given phase.
func ResolveEngramPathAndHash(phase string) (path, hashValue string, err error) {
	path, err = ResolveEngramPath(phase)
	if err != nil {
		return "", "", err
	}
	hashValue, err = hash.CalculateFileHash(path)
	if err != nil {
		return "", "", err
	}
	return path, hashValue, nil
}

// KnownPhases returns all known phase identifiers.
func KnownPhases() []string {
	seen := make(map[string]bool)
	var phases []string
	for k := range phaseToEngram {
		if !seen[k] {
			seen[k] = true
			phases = append(phases, k)
		}
	}
	return phases
}

func knownPhases() string {
	return strings.Join(KnownPhases(), ", ")
}

// findEngramRepoRoot searches for the engram repository root directory.
func findEngramRepoRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}

	// Check known locations in order of preference
	candidates := []string{
		filepath.Join(home, "src/ws/oss/repos/engram"),
		filepath.Join(home, "src/ws/oss/worktrees/engram"),
		filepath.Join(home, ".engram/repo"),
	}

	// Also check ENGRAM_REPO_ROOT env var
	if envRoot := os.Getenv("ENGRAM_REPO_ROOT"); envRoot != "" {
		candidates = append([]string{envRoot}, candidates...)
	}

	for _, candidate := range candidates {
		workflowDir := filepath.Join(candidate, engramWorkflowDir)
		if info, err := os.Stat(workflowDir); err == nil && info.IsDir() {
			return candidate, nil
		}
		// For worktrees, check subdirectories
		if strings.Contains(candidate, "worktrees") {
			entries, err := os.ReadDir(candidate)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if e.IsDir() {
					subPath := filepath.Join(candidate, e.Name())
					workflowDir := filepath.Join(subPath, engramWorkflowDir)
					if info, err := os.Stat(workflowDir); err == nil && info.IsDir() {
						return subPath, nil
					}
				}
			}
		}
	}

	return "", fmt.Errorf("engram repo not found in any known location; set ENGRAM_REPO_ROOT env var")
}
