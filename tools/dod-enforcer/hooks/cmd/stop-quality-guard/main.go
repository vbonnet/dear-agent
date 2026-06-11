// stop-quality-guard validates test suite, documentation freshness, and code
// quality before allowing a Claude Code session to exit.
package main

import (
	"context"
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
		fmt.Fprintf(os.Stderr, "[stop-quality-guard] failed to read input: %v\n", err)
		return 0
	}

	dir := input.Cwd
	if dir == "" {
		return 0
	}

	if !stophook.IsGitRepo(dir) {
		return 0
	}

	result := &stophook.Result{HookName: "stop-quality-guard"}

	checkTests(result, dir)
	checkDocs(result, dir)
	checkCodeMarkers(result, dir)

	result.Report()
	return result.ExitCode()
}

func checkTests(r *stophook.Result, dir string) {
	framework := stophook.DetectTestFramework(dir)
	if framework == "" {
		r.Pass("tests", "no test framework detected")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var cmd *exec.Cmd
	switch framework {
	case "go":
		cmd = exec.CommandContext(ctx, "go", "test", "./...")
		cmd.Env = append(os.Environ(), "GOWORK=off")
	case "npm":
		cmd = exec.CommandContext(ctx, "npm", "test", "--", "--passWithNoTests")
	case "pytest":
		cmd = exec.CommandContext(ctx, "python", "-m", "pytest", "--tb=short", "-q")
	case "cargo":
		cmd = exec.CommandContext(ctx, "cargo", "test")
	default:
		r.Pass("tests", fmt.Sprintf("unknown framework %q, skipped", framework))
		return
	}
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	if err != nil {
		// Show last few lines of output for context
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		tail := lines
		if len(tail) > 5 {
			tail = tail[len(tail)-5:]
		}
		r.Block("tests",
			fmt.Sprintf("%s tests failing", framework),
			fmt.Sprintf("fix test failures:\n%s", strings.Join(tail, "\n")))
		return
	}
	r.Pass("tests", fmt.Sprintf("%s tests passing", framework))
}

func checkDocs(r *stophook.Result, dir string) {
	specPath := filepath.Join(dir, "SPEC.md")
	readmePath := filepath.Join(dir, "README.md")

	issues := 0

	if !stophook.FileExists(specPath) {
		// Only warn if project has substantial code
		goFiles, _ := filepath.Glob(filepath.Join(dir, "**", "*.go"))
		pyFiles, _ := filepath.Glob(filepath.Join(dir, "**", "*.py"))
		if len(goFiles)+len(pyFiles) > 5 {
			r.Warn("docs", "no SPEC.md found in project with >5 source files",
				"create a SPEC.md for the project")
			issues++
		}
	}

	if !stophook.FileExists(readmePath) {
		r.Warn("docs", "no README.md found",
			"create a README.md")
		issues++
	} else {
		info, err := os.Stat(readmePath)
		if err == nil {
			age := time.Since(info.ModTime())
			if age > 30*24*time.Hour {
				r.Warn("docs",
					fmt.Sprintf("README.md is %d days old", int(age.Hours()/24)),
					"review and update README.md")
				issues++
			}
		}
	}

	if issues == 0 {
		r.Pass("docs", "documentation present")
	}
}

// grepCount returns the total number of matches for keyword across tracked
// source files in dir, or 0 on error/no-match.
func grepCount(dir, keyword string) int {
	cmd := exec.Command("git", "-C", dir, "grep", "-c", keyword, "--", "*.go", "*.py", "*.js", "*.ts")
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	count := 0
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
			var n int
			fmt.Sscanf(parts[1], "%d", &n)
			count += n
		}
	}
	return count
}

// checkCodeMarkers checks for TODO (advisory), FIXME, and HACK (both warned).
func checkCodeMarkers(r *stophook.Result, dir string) {
	todos := grepCount(dir, "TODO")
	fixmes := grepCount(dir, "FIXME")
	hacks := grepCount(dir, "HACK")

	if todos > 0 {
		r.Warn("todos",
			fmt.Sprintf("%d TODO comment(s) in source files", todos),
			"review and address TODO items before closing the session")
	}
	if fixmes > 0 {
		r.Warn("fixmes",
			fmt.Sprintf("%d FIXME comment(s) in source files", fixmes),
			"FIXME markers indicate known bugs — address or create a tracking bead before closing")
	}
	if hacks > 0 {
		r.Warn("hacks",
			fmt.Sprintf("%d HACK comment(s) in source files", hacks),
			"HACK markers indicate technical debt — document the trade-off or create a tracking bead")
	}
	if todos == 0 && fixmes == 0 && hacks == 0 {
		r.Pass("code-markers", "no TODO/FIXME/HACK markers found")
	}
}
