// stop-wayfinder-guard validates Wayfinder project state before allowing
// a Claude Code session to exit. Only fires if a Wayfinder project is detected.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/pkg/stophook"
	"github.com/vbonnet/dear-agent/wayfinder/internal/status"
)

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
	// Look for S11-retrospective.md in common locations
	candidates := []string{
		filepath.Join(dir, "S11-retrospective.md"),
		filepath.Join(dir, "wf", "S11-retrospective.md"),
	}

	for _, c := range candidates {
		if stophook.FileExists(c) {
			info, err := os.Stat(c)
			if err == nil && info.Size() > 100 {
				r.Pass("retrospective", "S11-retrospective.md exists with content")
				return
			}
			// Retro file exists but is effectively empty — block: it was started
			// but not completed before exiting.
			r.Block("retrospective",
				"S11-retrospective.md exists but has minimal content (<100 bytes)",
				"add meaningful retrospective content before exiting")
			return
		}
	}

	// Parse WAYFINDER-STATUS.md to detect project completion.
	s, err := status.ReadFrom(dir)
	if err != nil {
		// No parseable status — skip gracefully.
		r.Pass("retrospective", "no parseable WAYFINDER-STATUS.md, skipped")
		return
	}
	if s.Status == status.StatusCompleted || s.CurrentPhase == "S11" {
		// Project is at or past S11 but has no retrospective — block.
		r.Block("retrospective",
			"project is complete but no S11-retrospective.md found",
			"create docs/S11-retrospective.md or wf/S11-retrospective.md before exiting")
		return
	}
	r.Pass("retrospective", "project not at S11, retrospective not required yet")
}

func checkPhase(r *stophook.Result, dir string) {
	s, err := status.ReadFrom(dir)
	if err != nil {
		r.Pass("phase", "no parseable WAYFINDER-STATUS.md, skipped")
		return
	}

	switch s.Status {
	case status.StatusCompleted:
		r.Pass("phase", "project completed")
	case status.StatusAbandoned:
		r.Pass("phase", "project abandoned (intentional end state)")
	default:
		// StatusInProgress — or any other value including a stale/custom string.
		// This is not an error; just remind the user the project is still active.
		r.Warn("phase",
			fmt.Sprintf("project status is %q (not yet completed)", s.Status),
			"complete the current phase or mark the project as abandoned when done")
	}
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
