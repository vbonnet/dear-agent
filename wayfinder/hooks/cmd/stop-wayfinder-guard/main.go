// stop-wayfinder-guard validates Wayfinder project state before allowing
// an agent session to exit. Only fires if a Wayfinder project is detected.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/pkg/stophook"
	"gopkg.in/yaml.v3"
)

const (
	statusCompleted = "completed"
	statusAbandoned = "abandoned"
)

type wayfinderStatus struct {
	SchemaVersion   string `yaml:"schema_version"`
	Status          string `yaml:"status"`
	CurrentWaypoint string `yaml:"current_waypoint"`
}

func main() {
	os.Exit(stophook.RunWithTimeout(10*time.Second, run))
}

func run() int {
	input, err := stophook.ReadInput(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[stop-wayfinder-guard] failed to read input: %v\n", err)
		return 0
	}

	dir := input.Cwd
	if dir == "" {
		return 0
	}

	// Only fire if Wayfinder project detected
	if !stophook.HasWayfinder(dir) {
		return 0
	}

	result := &stophook.Result{HookName: "stop-wayfinder-guard"}

	checkBeads(result, dir)
	checkRetrospective(result, dir)
	checkPhase(result, dir)
	checkArtifacts(result, dir)

	result.Report()
	return result.ExitCode()
}

func checkBeads(r *stophook.Result, dir string) {
	cmd := exec.Command("bd", "list", "--status", "open")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		// bd not available — skip gracefully
		r.Pass("beads", "bd not available, skipped")
		return
	}
	lines := strings.TrimSpace(string(out))
	if lines == "" {
		r.Pass("beads", "no open beads")
		return
	}
	count := len(strings.Split(lines, "\n"))
	r.Warn("beads",
		fmt.Sprintf("%d open bead(s)", count),
		"close or update open beads before exiting")
}

func checkRetrospective(r *stophook.Result, dir string) {
	// Look for the canonical RETRO artifact in common locations.
	candidates := []string{
		filepath.Join(dir, "RETRO-retrospective.md"),
		filepath.Join(dir, "wf", "RETRO-retrospective.md"),
		filepath.Join(dir, "docs", "RETRO-retrospective.md"),
	}

	for _, c := range candidates {
		if stophook.FileExists(c) {
			info, err := os.Stat(c)
			if err == nil && info.Size() > 100 {
				r.Pass("retrospective", "RETRO-retrospective.md exists with content")
				return
			}
			// Retro file exists but is effectively empty — block: it was started
			// but not completed before exiting.
			r.Block("retrospective",
				"RETRO-retrospective.md exists but has minimal content (<100 bytes)",
				"add meaningful retrospective content before exiting")
			return
		}
	}

	// Parse WAYFINDER-STATUS.md to detect project completion.
	s, err := readCanonicalStatus(dir)
	if err != nil {
		// No parseable status — skip gracefully.
		r.Pass("retrospective", "no parseable WAYFINDER-STATUS.md, skipped")
		return
	}
	if s.Status == statusCompleted || s.CurrentWaypoint == "RETRO" {
		r.Block("retrospective",
			"project is complete but no RETRO-retrospective.md found",
			"create RETRO-retrospective.md, docs/RETRO-retrospective.md, or wf/RETRO-retrospective.md before exiting")
		return
	}
	r.Pass("retrospective", "project not at RETRO, retrospective not required yet")
}

func checkPhase(r *stophook.Result, dir string) {
	s, err := readCanonicalStatus(dir)
	if err != nil {
		r.Pass("phase", "no parseable WAYFINDER-STATUS.md, skipped")
		return
	}

	switch s.Status {
	case statusCompleted:
		r.Pass("phase", "project completed")
	case statusAbandoned:
		r.Pass("phase", "project abandoned (intentional end state)")
	default:
		// StatusInProgress — or any other value including a stale/custom string.
		// This is not an error; just remind the user the project is still active.
		r.Warn("phase",
			fmt.Sprintf("project status is %q (not yet completed)", s.Status),
			"complete the current phase or mark the project as abandoned when done")
	}
}

func readCanonicalStatus(dir string) (*wayfinderStatus, error) {
	data, err := os.ReadFile(filepath.Join(dir, "WAYFINDER-STATUS.md"))
	if err != nil {
		return nil, err
	}
	parts := strings.SplitN(string(data), "---", 3)
	if len(parts) != 3 || strings.TrimSpace(parts[0]) != "" {
		return nil, fmt.Errorf("invalid WAYFINDER-STATUS.md frontmatter")
	}
	var parsed wayfinderStatus
	if err := yaml.Unmarshal([]byte(parts[1]), &parsed); err != nil {
		return nil, fmt.Errorf("parse WAYFINDER-STATUS.md: %w", err)
	}
	if parsed.SchemaVersion != "2.0" {
		return nil, fmt.Errorf("legacy Wayfinder status requires explicit migration")
	}
	return &parsed, nil
}

func checkArtifacts(r *stophook.Result, dir string) {
	// Check for misplaced Wayfinder artifacts in root
	patterns := []string{"D[0-9]*.md", "S[0-9]*.md", "W[0-9]*.md"}
	misplaced := 0
	for _, p := range patterns {
		matches, _ := filepath.Glob(filepath.Join(dir, p))
		misplaced += len(matches)
	}
	if misplaced > 0 {
		r.Warn("artifacts",
			fmt.Sprintf("%d misplaced Wayfinder artifact(s) in project root", misplaced),
			"move artifacts to wf/ directory")
		return
	}
	r.Pass("artifacts", "no misplaced artifacts")
}
