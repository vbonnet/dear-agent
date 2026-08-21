package craplens

import (
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestComplexityParityWithGocycloBinary compares this package's counter against
// the real gocyclo binary over this repository's own source. CRAPLENS-05 claims
// the two agree, and a claim of parity that nothing checks against the actual
// tool is exactly the kind of unverified assertion this signal exists to catch.
//
// Skipped when gocyclo is not installed, since it is a developer tool rather
// than a module dependency.
func TestComplexityParityWithGocycloBinary(t *testing.T) {
	bin, err := exec.LookPath("gocyclo")
	if err != nil {
		t.Skip("gocyclo not installed")
	}

	targets := []string{"internal/craplens", "tools/crap-lint", "internal/prconcern", "agm/cmd/agm-bus", "cmd/vroom-dispatch", "internal/specguard", "cmd/ai-review"}
	root := repoRoot(t)

	for _, target := range targets {
		dir := filepath.Join(root, target)
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		out, err := exec.Command(bin, "-over", "0", dir).Output()
		if err != nil {
			t.Fatalf("gocyclo %s: %v", dir, err)
		}
		compareGocycloOutput(t, root, string(out))
	}
}

// compareGocycloOutput checks every non-test function gocyclo reported against
// this package's own count for the same declaration.
func compareGocycloOutput(t *testing.T, root, out string) {
	t.Helper()

	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		// Format: "<complexity> <package> <func> <file>:<line>:<col>"
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		want, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		loc := fields[len(fields)-1]
		path, rest, ok := strings.Cut(loc, ":")
		if !ok || strings.HasSuffix(path, "_test.go") {
			continue
		}
		lineStr, _, _ := strings.Cut(rest, ":")
		declLine, err := strconv.Atoi(lineStr)
		if err != nil {
			continue
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		got, found := complexityAt(t, path, declLine)
		if !found {
			t.Errorf("%s:%d: gocyclo reported %s but no declaration was found there", rel, declLine, fields[2])
			continue
		}
		if got != want {
			t.Errorf("%s:%d %s: this package counted %d, gocyclo counted %d", rel, declLine, fields[2], got, want)
		}
	}
}

// complexityAt parses a file and returns the complexity of the declaration
// starting on the given line.
func complexityAt(t *testing.T, path string, line int) (int, bool) {
	t.Helper()

	src, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
	if err != nil {
		return 0, false
	}
	for _, decl := range parsed.Decls {
		for _, cand := range declaredFuncs(decl) {
			// gocyclo reports a package-level function literal at the line of
			// its `var` declaration, which is where the GenDecl starts, while
			// the literal itself starts on the same line here. Accept either.
			if fset.Position(cand.node.Pos()).Line == line || fset.Position(decl.Pos()).Line == line {
				return complexity(cand.node), true
			}
		}
	}
	return 0, false
}

// repoRoot walks up from the working directory to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("module root not found")
		}
		dir = parent
	}
}
