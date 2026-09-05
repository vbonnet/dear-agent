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
// than a module dependency. A skip is not a pass: .github/workflows/pr-size-scope.yml
// installs a pinned gocyclo and fails the job if this test reports SKIP, so
// CRAPLENS-05 cannot drift unverified in CI.
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
		// The location is everything after the first three
		// whitespace-delimited fields, taken verbatim rather than
		// fields[len(fields)-1]: a source filename containing a space (a
		// valid Go file name) splits gocyclo's location across more than
		// one field, and picking only the last field silently truncates the
		// path down to its final segment.
		loc := skipFields(line, 3)
		if loc == "" {
			continue
		}
		path, declLine, declCol, ok := parseGocycloLocation(loc)
		if !ok || strings.HasSuffix(path, "_test.go") {
			continue
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		got, found := complexityAt(t, path, declLine, declCol)
		if !found {
			t.Errorf("%s:%d: gocyclo reported %s but no declaration was found there", rel, declLine, fields[2])
			continue
		}
		if got != want {
			t.Errorf("%s:%d %s: this package counted %d, gocyclo counted %d", rel, declLine, fields[2], got, want)
		}
	}
}

func parseGocycloLocation(loc string) (string, int, int, bool) {
	last := strings.LastIndexByte(loc, ':')
	if last < 0 {
		return "", 0, 0, false
	}
	second := strings.LastIndexByte(loc[:last], ':')
	if second < 0 {
		return "", 0, 0, false
	}
	line, errLine := strconv.Atoi(loc[second+1 : last])
	col, errCol := strconv.Atoi(loc[last+1:])
	if errLine != nil || errCol != nil {
		return "", 0, 0, false
	}
	return loc[:second], line, col, true
}

// skipFields returns line with its first n whitespace-delimited fields
// removed, leaving the remainder (including any internal spaces) intact and
// leading whitespace trimmed. Returns "" if line has fewer than n fields.
func skipFields(line string, n int) string {
	rest := line
	for range n {
		rest = strings.TrimLeft(rest, " \t")
		idx := strings.IndexAny(rest, " \t")
		if idx < 0 {
			return ""
		}
		rest = rest[idx:]
	}
	return strings.TrimLeft(rest, " \t")
}

// complexityAt parses a file and returns the complexity of the declaration
// starting on the given line.
func complexityAt(t *testing.T, path string, line, col int) (int, bool) {
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
			candPos := fset.Position(cand.node.Pos())
			if (candPos.Line == line || fset.Position(decl.Pos()).Line == line) && (col == 0 || candPos.Column == col) {
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

// TestSkipFieldsPreservesSpacesInTheRemainder guards the exact gap that made
// gocyclo parity silently misread a location for any source filename
// containing a space: fields[len(fields)-1] would return only the final
// whitespace-delimited chunk, dropping the location's leading path segment.
func TestSkipFieldsPreservesSpacesInTheRemainder(t *testing.T) {
	tests := []struct {
		name string
		line string
		n    int
		want string
	}{
		{
			name: "ordinary, no spaces in the location",
			line: "10 main funcName /path/to/file.go:12:1",
			n:    3,
			want: "/path/to/file.go:12:1",
		},
		{
			name: "location contains a space",
			line: "4 pkg Func /path/to/some file.go:8:2",
			n:    3,
			want: "/path/to/some file.go:8:2",
		},
		{name: "too few fields", line: "10 main", n: 3, want: ""},
		{name: "skip zero fields trims only leading whitespace", line: "  a b  ", n: 0, want: "a b  "},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := skipFields(tc.line, tc.n); got != tc.want {
				t.Errorf("skipFields(%q, %d) = %q, want %q", tc.line, tc.n, got, tc.want)
			}
		})
	}
}

func TestParseGocycloLocationUsesTrailingLineAndColumn(t *testing.T) {
	path, line, col, ok := parseGocycloLocation("internal/craplens/a:b.go:12:7")
	if !ok || path != "internal/craplens/a:b.go" || line != 12 || col != 7 {
		t.Fatalf("parse location = %q:%d:%d,%v", path, line, col, ok)
	}
}

func TestComplexityAtUsesColumnForSameLineLiterals(t *testing.T) {
	path := filepath.Join(t.TempDir(), "same.go")
	src := []byte("package p\nvar a, b = func() { if true {} }, func() { if true {}; if true {} }\n")
	if err := os.WriteFile(path, src, 0o600); err != nil {
		t.Fatal(err)
	}
	got, ok := complexityAt(t, path, 2, 35)
	if !ok || got != 3 {
		t.Fatalf("complexityAt = %d,%v; want 3,true", got, ok)
	}
}
