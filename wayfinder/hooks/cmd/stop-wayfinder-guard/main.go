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
			r.Warn("retrospective",
				"S11-retrospective.md exists but has minimal content",
				"add meaningful retrospective content")
			return
		}
	}

	// Check WAYFINDER-STATUS.md to see if we're at S11
	statusPath := filepath.Join(dir, "WAYFINDER-STATUS.md")
	data, err := os.ReadFile(statusPath)
	if err != nil {
		r.Pass("retrospective", "no WAYFINDER-STATUS.md, skipped")
		return
	}
	content := string(data)
	if strings.Contains(content, "S11") || strings.Contains(content, "complete") {
		r.Warn("retrospective",
			"project appears complete but no S11-retrospective.md found",
			"create a retrospective document")
		return
	}
	r.Pass("retrospective", "project not at S11, retrospective not required")
}

func checkPhase(r *stophook.Result, dir string) {
	statusPath := filepath.Join(dir, "WAYFINDER-STATUS.md")
	data, err := os.ReadFile(statusPath)
	if err != nil {
		r.Pass("phase", "no WAYFINDER-STATUS.md, skipped")
		return
	}
	content := string(data)

	if strings.Contains(content, "abandoned") || strings.Contains(content, "blocked") {
		r.Pass("phase", "project explicitly ended")
		return
	}
	if strings.Contains(content, "S11") || strings.Contains(content, "completed") {
		r.Pass("phase", "project at completion phase")
		return
	}

	r.Warn("phase",
		"Wayfinder project not at completion phase",
		"complete current phase or mark project status explicitly")
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
