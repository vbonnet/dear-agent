// stop-wayfinder-guard validates Wayfinder project state before allowing
// an agent session to exit. Only fires if a Wayfinder project is detected.
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/pkg/stophook"
	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/statusread"
)

const (
	statusCompleted = "completed"
	statusAbandoned = "abandoned"
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
		if errors.Is(err, fs.ErrNotExist) {
			r.Pass("retrospective", "no WAYFINDER-STATUS.md, skipped")
			return
		}
		r.Block("retrospective", "invalid WAYFINDER-STATUS.md", err.Error())
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
		if errors.Is(err, fs.ErrNotExist) {
			r.Pass("phase", "no WAYFINDER-STATUS.md, skipped")
			return
		}
		r.Block("phase", "invalid WAYFINDER-STATUS.md", err.Error())
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

func readCanonicalStatus(dir string) (*statusread.Summary, error) {
	return statusread.ParseFromDir(dir)
}
