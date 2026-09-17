package craplens

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

// TestWorkingTreeIsCleanRejectsIgnoredFilesInTouchedPackages guards two real
// gaps in one shared fixture shape:
//
//   - "quoted .go path": text-mode `git status --porcelain` C-quotes a path
//     containing a space (confirmed: it renders as `!! "pkg/a
//     name_test.go"`), and the retained closing quote made the .go suffix
//     check false, so an ignored test file with a space in its name was
//     silently accepted as clean.
//   - "non-.go asset": an ignored non-.go file inside a touched package (a
//     //go:embed target, or a fixture a test reads by path) can reach
//     `go test`'s build or test run just as surely as an ignored .go file
//     compiles in; the prior fix only caught the .go case.
//
// Both must be treated as dirty inside a touched package, and both must stay
// harmless outside every touched package.
func TestWorkingTreeIsCleanRejectsIgnoredFilesInTouchedPackages(t *testing.T) {
	tests := []struct {
		name           string
		ignorePattern  string
		ignoredRelPath string
	}{
		{name: "quoted .go path", ignorePattern: "pkg/a name_test.go", ignoredRelPath: "pkg/a name_test.go"},
		{name: "non-.go asset", ignorePattern: "pkg/asset.txt", ignoredRelPath: "pkg/asset.txt"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			gittest.Run(t, dir, "init", "-q", "-b", "main")
			gittest.Run(t, dir, "config", "user.email", "test@example.invalid")
			gittest.Run(t, dir, "config", "user.name", "Test")
			if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o750); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "pkg", "keep.go"), []byte("package pkg\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			gittest.Run(t, dir, "add", "-A")
			gittest.Run(t, dir, "commit", "-q", "-m", "base")
			if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(tc.ignorePattern+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			gittest.Run(t, dir, "add", ".gitignore")
			gittest.Run(t, dir, "commit", "-q", "-m", "ignore")
			if err := os.WriteFile(filepath.Join(dir, tc.ignoredRelPath), []byte("local content\n"), 0o600); err != nil {
				t.Fatal(err)
			}

			if workingTreeIsClean(t.Context(), dir, []string{"pkg"}) {
				t.Fatal("an ignored file inside a touched package must not be treated as a clean checkout")
			}

			// The same ignored file outside any touched package is still harmless.
			if !workingTreeIsClean(t.Context(), dir, []string{"other"}) {
				t.Fatal("an ignored file outside every touched package must not force a dirty checkout")
			}
		})
	}
}

func TestTreeHasGoFilesReadsNULDelimitedQuotedNames(t *testing.T) {
	dir := t.TempDir()
	gittest.Run(t, dir, "init", "-q", "-b", "main")
	gittest.Run(t, dir, "config", "user.email", "test@example.invalid")
	gittest.Run(t, dir, "config", "user.name", "Test")
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o750); err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(dir, "pkg", "line\tquote\\\".go")
	if err := os.WriteFile(name, []byte("package pkg\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gittest.Run(t, dir, "add", "-A")
	gittest.Run(t, dir, "commit", "-q", "-m", "base")
	if !treeHasGoFiles(t.Context(), dir, "HEAD", "pkg") {
		t.Fatal("treeHasGoFiles must recognize a Go file whose name requires Git quoting")
	}
}

func TestChangedGoFilesIncludesTypeChanges(t *testing.T) {
	dir := t.TempDir()
	gittest.Run(t, dir, "init", "-q", "-b", "main")
	gittest.Run(t, dir, "config", "user.email", "test@example.invalid")
	gittest.Run(t, dir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "target.go"), []byte("package target\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gittest.Run(t, dir, "add", "target.go")
	gittest.Run(t, dir, "commit", "-q", "-m", "base")
	gittest.Run(t, dir, "rm", "target.go")
	if err := os.Symlink("missing", filepath.Join(dir, "target.go")); err != nil {
		t.Fatal(err)
	}
	gittest.Run(t, dir, "add", "-A")
	gittest.Run(t, dir, "commit", "-q", "-m", "symlink")
	base := "HEAD~1"
	if err := os.Remove(filepath.Join(dir, "target.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "target.go"), []byte("package target\n\nfunc Changed() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gittest.Run(t, dir, "add", "-A")
	gittest.Run(t, dir, "commit", "-q", "-m", "regular")
	files, err := changedGoFiles(t.Context(), dir, base, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := files["target.go"]; !ok {
		t.Fatalf("type-changed Go file missing from touched set: %v", sortedKeys(files))
	}
}

// TestParseHunkHeader pins the diff arithmetic the whole signal rests on. A
// wrong head-side span attributes changes to the wrong function.
func TestParseHunkHeader(t *testing.T) {
	tests := []struct {
		name   string
		line   string
		want   lineRange
		wantOK bool
	}{
		{name: "explicit count", line: "@@ -10,3 +20,5 @@ func x()", want: lineRange{20, 24}, wantOK: true},
		{name: "implicit single line", line: "@@ -10 +20 @@", want: lineRange{20, 20}, wantOK: true},
		{name: "pure deletion has no head lines", line: "@@ -10,3 +20,0 @@", wantOK: false},
		{name: "malformed", line: "@@ nonsense @@", wantOK: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseHunkHeader(tc.line)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok && got != tc.want {
				t.Errorf("range = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestIsScorableGoFile pins the exclusions. Scoring test files would let a diff
// improve its own score by adding a branchy test.
func TestIsScorableGoFile(t *testing.T) {
	tests := map[string]bool{
		"agm/cmd/agm-bus/main.go":      true,
		"internal/craplens/diff.go":    true,
		"agm/cmd/agm-bus/main_test.go": false,
		"vendor/x/y.go":                false,
		"pkg/testdata/fixture.go":      false,
		"api/service.pb.go":            false,
		"pkg/x_generated.go":           false,
		"README.md":                    false,
	}

	for path, want := range tests {
		if got := isScorableGoFile(path); got != want {
			t.Errorf("isScorableGoFile(%q) = %v, want %v", path, got, want)
		}
	}
}

// TestParseUnifiedDiffTracksAddedFiles pins that a wholly-new file is
// recognised as added, which is what distinguishes a new package shipping
// untested from an existing untested package that got edited.
func TestParseUnifiedDiffTracksAddedFiles(t *testing.T) {
	out := strings.Join([]string{
		"diff --git a/pkg/new/thing.go b/pkg/new/thing.go",
		"new file mode 100644",
		"--- /dev/null",
		"+++ b/pkg/new/thing.go",
		"@@ -0,0 +1,12 @@",
		"diff --git a/pkg/old/thing.go b/pkg/old/thing.go",
		"index 111..222 100644",
		"--- a/pkg/old/thing.go",
		"+++ b/pkg/old/thing.go",
		"@@ -5,2 +5,3 @@",
		"diff --git a/pkg/old/thing_test.go b/pkg/old/thing_test.go",
		"--- a/pkg/old/thing_test.go",
		"+++ b/pkg/old/thing_test.go",
		"@@ -1,2 +1,9 @@",
	}, "\n")

	files, err := parseUnifiedDiff(out)
	if err != nil {
		t.Fatalf("parseUnifiedDiff: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("parsed %d files, want 2 (the test file must be excluded): %v", len(files), sortedKeys(files))
	}
	if !files["pkg/new/thing.go"].Added {
		t.Error("pkg/new/thing.go should be marked added")
	}
	if files["pkg/old/thing.go"].Added {
		t.Error("pkg/old/thing.go is an edit, not an addition")
	}
	if got := files["pkg/old/thing.go"].Ranges; len(got) != 1 || got[0] != (lineRange{5, 7}) {
		t.Errorf("pkg/old/thing.go ranges = %+v, want [{5 7}]", got)
	}
}

// TestParseUnifiedDiffTracksRenamedDestinationsAsAdded covers a renamed-and-
// edited file: git emits `rename from`/`rename to` instead of `new file
// mode`, so a naive check for the latter alone left the destination's
// Added false even when it is the only file in a package the diff touched,
// preventing a genuinely new package (one whose destination directory held
// no Go source at the base revision) from being classified as new.
func TestParseUnifiedDiffTracksRenamedDestinationsAsAdded(t *testing.T) {
	out := strings.Join([]string{
		"diff --git a/pkg/old/thing.go b/pkg/newdir/thing.go",
		"similarity index 92%",
		"rename from pkg/old/thing.go",
		"rename to pkg/newdir/thing.go",
		"index 111..222 100644",
		"--- a/pkg/old/thing.go",
		"+++ b/pkg/newdir/thing.go",
		"@@ -5,2 +5,3 @@",
	}, "\n")

	files, err := parseUnifiedDiff(out)
	if err != nil {
		t.Fatalf("parseUnifiedDiff: %v", err)
	}
	if !files["pkg/newdir/thing.go"].Added {
		t.Error("a renamed-and-edited destination should be marked added, same as a wholly new file")
	}
}

// TestParseUnifiedDiffDropsPureDeletions pins that a file whose every hunk
// removed lines is not reported: it has no head-side line left to score.
func TestParseUnifiedDiffDropsPureDeletions(t *testing.T) {
	out := strings.Join([]string{
		"diff --git a/pkg/x/y.go b/pkg/x/y.go",
		"--- a/pkg/x/y.go",
		"+++ b/pkg/x/y.go",
		"@@ -5,9 +4,0 @@",
	}, "\n")

	files, err := parseUnifiedDiff(out)
	if err != nil {
		t.Fatalf("parseUnifiedDiff: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("parsed %v, want nothing for a pure deletion", sortedKeys(files))
	}
}

// TestAllFilesAdded pins the diff-side half of the new-package decision. It is
// necessary but not sufficient on its own: the caller also confirms the
// directory held no Go source at the base, because adding one file to an
// existing package also makes every changed file an addition.
func TestAllFilesAdded(t *testing.T) {
	files := touchedSet{
		"pkg/fresh/a.go": {Path: "pkg/fresh/a.go", Added: true},
		"pkg/fresh/b.go": {Path: "pkg/fresh/b.go", Added: true},
		"pkg/mixed/a.go": {Path: "pkg/mixed/a.go", Added: true},
		"pkg/mixed/b.go": {Path: "pkg/mixed/b.go", Added: false},
	}

	if !files.allFilesAdded("pkg/fresh") {
		t.Error("pkg/fresh should be new")
	}
	if files.allFilesAdded("pkg/mixed") {
		t.Error("pkg/mixed has an edited file, so it is not new")
	}
	if files.allFilesAdded("pkg/absent") {
		t.Error("a package the diff did not touch is not new")
	}
}

// TestInTouchedPackage pins the scope workingTreeIsClean narrows an ignored
// path against: an ignored path inside a touched package directory, or any
// of its subdirectories, can still reach that package's build, while a path
// outside every touched package -- a build artifact left by `make preflight`,
// for one -- must not match. The module root package owns every
// repository-relative path, because a root package reaches nested assets the
// same way any other package reaches its own subdirectories.
func TestInTouchedPackage(t *testing.T) {
	tests := []struct {
		name string
		pkgs []string
		path string
		want bool
	}{
		{name: "inside a touched package", pkgs: []string{"internal/craplens"}, path: "internal/craplens/coverage_test.go", want: true},
		{name: "nested ignored asset below a touched one", pkgs: []string{"internal/craplens"}, path: "internal/craplens/assets/local.json", want: true},
		{name: "untouched package", pkgs: []string{"internal/craplens"}, path: "internal/other/x.go", want: false},
		{name: "build artifact outside every touched package", pkgs: []string{"internal/craplens"}, path: "build/agm", want: false},
		{name: "direct child of the root package", pkgs: []string{"."}, path: "main.go", want: true},
		// The reviewed gap: a root package can embed or read a nested asset,
		// so a descendant must count as dirty just like a direct child.
		{name: "nested asset below the root package", pkgs: []string{"."}, path: "assets/local.json", want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := inTouchedPackage(tc.path, tc.pkgs); got != tc.want {
				t.Errorf("inTouchedPackage(%q, %v) = %v, want %v", tc.path, tc.pkgs, got, tc.want)
			}
		})
	}
}

// TestIsGeneratedSource pins detection by the standard toolchain marker rather
// than by filename. CRAPLENS-02 excludes generated files, and this repository
// contains generated source whose name matches no generated-looking suffix.
func TestIsGeneratedSource(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want bool
	}{
		{
			name: "standard marker",
			src:  "// Code generated by tool. DO NOT EDIT.\n\npackage p\n",
			want: true,
		},
		{
			name: "marker with a different generator",
			src:  "// Code generated from patterns.yaml; DO NOT EDIT.\n\npackage p\n",
			want: true,
		},
		{name: "handwritten", src: "// Package p does a thing.\npackage p\n", want: false},
		{name: "prose mentioning generation", src: "// This code generated a report once.\npackage p\n", want: false},
		{
			// A raw-string byte search over the file's leading bytes cannot
			// tell this apart from a real marker before the package clause —
			// this repo has exactly this shape in
			// engram/hooks-bin/cmd/generate-patterns/main.go's tmplSource,
			// a handwritten generator whose own output template contains
			// the marker text. ast.IsGenerated correctly scopes detection to
			// comments preceding the package clause, so this must not be
			// misclassified as generated just because the marker string
			// appears somewhere in the file.
			name: "marker text embedded in a raw string after the package clause",
			src: "// Package p emits Go source from a template.\n" +
				"package p\n\n" +
				"const tmplSource = `// Code generated by tool. DO NOT EDIT.\n\npackage generated\n`\n",
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isGeneratedSource([]byte(tc.src)); got != tc.want {
				t.Errorf("isGeneratedSource = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestHeaderPathDecodesQuotedPathnames covers the pathnames git C-quotes even
// with core.quotePath=false. Requiring a bare `b/` prefix dropped those files
// silently, which is the same class of miss the quotePath flag fixed for
// non-ASCII bytes.
func TestHeaderPathDecodesQuotedPathnames(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{name: "plain", raw: "b/pkg/a.go", want: "pkg/a.go", ok: true},
		{name: "non-ascii verbatim", raw: "b/pkg/café.go", want: "pkg/café.go", ok: true},
		{name: "quoted tab", raw: `"b/pkg/tab\tname.go"`, want: "pkg/tab\tname.go", ok: true},
		{name: "quoted backslash", raw: `"b/pkg/back\\slash.go"`, want: `pkg/back\slash.go`, ok: true},
		{name: "quoted octal", raw: `"b/pkg/caf\303\251.go"`, want: "pkg/café.go", ok: true},
		{name: "dev null", raw: "/dev/null", ok: false},
		{name: "test file still excluded", raw: "b/pkg/a_test.go", ok: false},
		{name: "quoted test file still excluded", raw: `"b/pkg/tab\tname_test.go"`, ok: false},
		{name: "unparseable quote", raw: `"b/pkg/bad`, ok: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := headerPath(tc.raw)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v (got %q)", ok, tc.ok, got)
			}
			if ok && got != tc.want {
				t.Errorf("path = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestParseUnifiedDiffRejectsTruncatedInput covers the scanner-error path. A
// truncated diff returned as a complete one would silently drop every file
// after the offending hunk out of the signal.
func TestParseUnifiedDiffRejectsTruncatedInput(t *testing.T) {
	huge := strings.Repeat("x", 9<<20)
	out := strings.Join([]string{
		"diff --git a/pkg/a.go b/pkg/a.go",
		"--- a/pkg/a.go",
		"+++ b/pkg/a.go",
		"@@ -1 +1 @@",
		"+" + huge,
		"diff --git a/pkg/b.go b/pkg/b.go",
		"--- a/pkg/b.go",
		"+++ b/pkg/b.go",
		"@@ -1 +1 @@",
	}, "\n")

	if _, err := parseUnifiedDiff(out); err == nil {
		t.Fatal("expected a truncated diff to be rejected rather than returned as complete")
	}
}

// TestParseUnifiedDiffIgnoresAddedLinesThatLookLikeHeaders covers added source
// text beginning with "++ ": git prefixes it with the addition marker, so the
// emitted line starts with "+++ " and was mistaken for a file header, clearing
// the current file and dropping every later hunk in it.
func TestParseUnifiedDiffIgnoresAddedLinesThatLookLikeHeaders(t *testing.T) {
	out := strings.Join([]string{
		"diff --git a/pkg/a.go b/pkg/a.go",
		"--- a/pkg/a.go",
		"+++ b/pkg/a.go",
		"@@ -1,0 +5,2 @@",
		`+	doc := ` + "`" + `++ not a header` + "`",
		"+++ still source, not a header",
		"@@ -20,0 +30,3 @@",
		"+more source",
	}, "\n")

	files, err := parseUnifiedDiff(out)
	if err != nil {
		t.Fatalf("parseUnifiedDiff: %v", err)
	}
	f := files["pkg/a.go"]
	if f == nil {
		t.Fatal("pkg/a.go was dropped entirely")
	}
	if len(f.Ranges) != 2 {
		t.Fatalf("got %d hunks, want 2: a later hunk was lost to a fake header: %+v", len(f.Ranges), f.Ranges)
	}
	if f.Ranges[1] != (lineRange{30, 32}) {
		t.Errorf("second hunk = %+v, want {30 32}", f.Ranges[1])
	}
}
