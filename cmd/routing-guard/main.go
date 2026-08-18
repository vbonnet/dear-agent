// Command routing-guard blocks temporal artifacts from this code repo.
//
// Wayfinder runs, retros, designs, research, plans, roadmaps, backlogs, and
// reports are temporal: they capture a moment of thinking and belong in an
// engram-research worktree, never in this code repo. Current policy, SPEC,
// architecture, ADR, source, and tests remain beside their owned code.
//
// The forbidden globs are the SINGLE source of truth in .dear-agent.yml >
// forbidden-paths. This tool reads them at runtime, so the rule can never
// drift from a hand-copied list — the root cause of the 2026-06-19 Wayfinder
// leak (see the DEAR retro in engram-research). One binary, three call sites:
// pre-commit (--staged), CI PR diff (--diff), CI whole-tree (--all).
//
// Usage:
//
//	routing-guard --all                       # scan every tracked file
//	routing-guard --staged                    # scan staged files (pre-commit)
//	routing-guard --diff <base-ref>           # scan files added/renamed vs base
//	routing-guard --files <file|->            # scan paths from a file or stdin
//
// Exit 0 = clean, 1 = violations found, 2 = usage/internal error.
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type config struct {
	ForbiddenPaths map[string][]string `yaml:"forbidden-paths"`
}

func main() {
	mode, operand, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "routing-guard:", err)
		os.Exit(2)
	}

	root, err := gitOutput("rev-parse", "--show-toplevel")
	if err != nil {
		fmt.Fprintln(os.Stderr, "routing-guard: not in a git repo:", err)
		os.Exit(2)
	}
	root = strings.TrimSpace(root)

	patterns, err := loadPatterns(filepath.Join(root, ".dear-agent.yml"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "routing-guard:", err)
		os.Exit(2)
	}
	if len(patterns) == 0 {
		fmt.Fprintln(os.Stderr, "routing-guard: forbidden-paths policy is empty")
		os.Exit(2)
	}

	files, err := gatherFiles(root, mode, operand)
	if err != nil {
		fmt.Fprintln(os.Stderr, "routing-guard:", err)
		os.Exit(2)
	}
	var violations []string
	for _, f := range files {
		if f == "" {
			continue
		}
		if forbidden(f, patterns) {
			violations = append(violations, f)
		}
	}

	if len(violations) == 0 {
		os.Exit(0)
	}
	report(violations)
	os.Exit(1)
}

func parseArgs(args []string) (mode, operand string, err error) {
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "--all", "--staged":
			mode = a
		case "--diff", "--files":
			if i+1 >= len(args) {
				return "", "", fmt.Errorf("%s requires an argument", a)
			}
			mode = a
			i++
			operand = args[i]
		case "-h", "--help":
			fmt.Println(strings.TrimSpace(usage))
			os.Exit(0)
		default:
			return "", "", fmt.Errorf("unknown arg %q (see --help)", a)
		}
	}
	if mode == "" {
		mode = "--all"
	}
	return mode, operand, nil
}

// loadPatterns reads every forbidden glob (across all artifact kinds) from
// .dear-agent.yml > forbidden-paths.
func loadPatterns(ymlPath string) ([]string, error) {
	data, err := os.ReadFile(ymlPath)
	if err != nil {
		return nil, fmt.Errorf("read policy %s: %w", ymlPath, err)
	}
	var c config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", ymlPath, err)
	}
	var out []string
	for _, globs := range c.ForbiddenPaths {
		out = append(out, globs...)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s: forbidden-paths must contain at least one pattern", ymlPath)
	}
	return out, nil
}

func gatherFiles(root, mode, operand string) ([]string, error) {
	switch mode {
	case "--all":
		return gitLines(root, "ls-files")
	case "--staged":
		return gitLines(root, "diff", "--cached", "--name-only", "--diff-filter=ACMR")
	case "--diff":
		return gitLines(root, "diff", "--name-only", "--diff-filter=ACMR", operand+"..HEAD")
	case "--files":
		return readListFile(operand)
	default:
		return nil, fmt.Errorf("unknown mode %q", mode)
	}
}

// forbidden reports whether a repo-relative path matches any forbidden glob.
// A ** segment consumes zero or more complete path segments. Every other
// segment uses path.Match syntax, so matching never crosses a slash.
func forbidden(p string, patterns []string) bool {
	for _, pat := range patterns {
		// Source extensions disambiguate filename-based patterns such as
		// backlog.go. They do not exempt files placed inside an explicitly
		// temporal directory such as wf/** or **/.wayfinder/**.
		if livingSourceFile(p) && !strings.HasSuffix(strings.TrimSpace(pat), "/**") {
			continue
		}
		if globPathMatch(pat, p) {
			return true
		}
	}
	return false
}

func livingSourceFile(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".bash", ".bats", ".c", ".cc", ".cjs", ".cpp", ".cs", ".cts",
		".d2", ".dart", ".ex", ".exs", ".fs", ".fsx", ".go", ".h", ".hpp",
		".hs", ".java", ".js", ".jsx", ".kt", ".kts", ".lua", ".mjs", ".ml",
		".mli", ".mts", ".php", ".pl", ".pm", ".proto", ".py", ".r", ".rb",
		".rs", ".scala", ".sh", ".sol", ".sql", ".swift", ".tf", ".tfvars",
		".ts", ".tsx", ".vue", ".zsh":
		return true
	default:
		return false
	}
}

func globPathMatch(pattern, name string) bool {
	pattern = strings.Trim(path.Clean(pattern), "/")
	name = strings.Trim(path.Clean(name), "/")
	patternParts := strings.Split(pattern, "/")
	nameParts := strings.Split(name, "/")
	type state struct{ pattern, name int }
	memo := map[state]bool{}

	var match func(int, int) bool
	match = func(patternIndex, nameIndex int) bool {
		key := state{pattern: patternIndex, name: nameIndex}
		if result, ok := memo[key]; ok {
			return result
		}

		var result bool
		switch {
		case patternIndex == len(patternParts):
			result = nameIndex == len(nameParts)
		case patternParts[patternIndex] == "**":
			result = match(patternIndex+1, nameIndex) ||
				(nameIndex < len(nameParts) && match(patternIndex, nameIndex+1))
		case nameIndex < len(nameParts):
			segmentMatch, _ := path.Match(patternParts[patternIndex], nameParts[nameIndex])
			result = segmentMatch && match(patternIndex+1, nameIndex+1)
		}
		memo[key] = result
		return result
	}

	return match(0, 0)
}

func report(violations []string) {
	w := os.Stderr
	fmt.Fprintln(w)
	fmt.Fprintln(w, "ROUTING VIOLATION — temporal artifacts must not live in this code repo.")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Forbidden files detected (%d):\n", len(violations))
	for _, v := range violations {
		fmt.Fprintf(w, "  - %s\n", v)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "These belong in the knowledge base instead:")
	fmt.Fprintln(w, "  Use an engram-research worktree under its audit, project, retro, or wf directory.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "How to fix:")
	fmt.Fprintln(w, "  1. git rm --cached the file(s) and remove them from this repo.")
	fmt.Fprintln(w, "  2. Move the content through an engram-research worktree (open a PR there).")
	fmt.Fprintln(w, "  3. Re-commit here without the artifact.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Why: temporal artifacts rot in code repos — they go stale silently,")
	fmt.Fprintln(w, "clutter blame history, and strand the work away from the corpus. The")
	fmt.Fprintln(w, "forbidden globs are defined in .dear-agent.yml > forbidden-paths.")
	fmt.Fprintln(w)
}

func readListFile(src string) ([]string, error) {
	var r *bufio.Scanner
	if src == "-" {
		r = bufio.NewScanner(os.Stdin)
	} else {
		f, err := os.Open(src)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		r = bufio.NewScanner(f)
	}
	var out []string
	for r.Scan() {
		out = append(out, strings.TrimSpace(r.Text()))
	}
	return out, r.Err()
}

func gitLines(root string, args ...string) ([]string, error) {
	out, err := gitOutput(append([]string{"-C", root}, args...)...)
	if err != nil {
		return nil, err
	}
	var lines []string
	for l := range strings.SplitSeq(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	return lines, nil
}

func gitOutput(args ...string) (string, error) {
	// #nosec G702 G204 -- fixed "git" binary; args are internal git flags plus
	// a git ref/path passed as argv (no shell), never an attacker-chosen command.
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	return string(out), err
}

const usage = `routing-guard — block temporal artifacts (Wayfinder runs, retros, designs,
research) from this code repo. Forbidden globs come from .dear-agent.yml.

  routing-guard --all                 scan every tracked file
  routing-guard --staged              scan staged files (pre-commit)
  routing-guard --diff <base-ref>     scan files added/renamed vs a base ref
  routing-guard --files <file|->      scan paths from a file or stdin
Exit 0 = clean, 1 = violations, 2 = usage/internal error.`
