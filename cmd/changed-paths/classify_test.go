package main

import (
	"strings"
	"testing"
)

func classify(t *testing.T, roots []string, files ...string) Selection {
	t.Helper()
	c := &Classifier{EmbedRoots: roots}
	return c.Classify(files)
}

func TestEmptyDiffFailsSafe(t *testing.T) {
	sel := classify(t, nil)
	for _, k := range Keys {
		if !sel.Values[k] {
			t.Fatalf("empty diff must force %q on", k)
		}
	}
	if sel.Reason == "" {
		t.Fatal("fail-safe selection must carry a reason")
	}
}

func TestGlobalInputsForceEverythingOn(t *testing.T) {
	for _, f := range []string{
		"go.mod", "go.sum", "go.work", "Makefile", "agm/go.mod",
		"vendor/x/y.go", ".github/workflows/ci.yml", ".golangci.yml",
		".dear-agent.yml",
	} {
		sel := classify(t, nil, f)
		for _, k := range Keys {
			if !sel.Values[k] {
				t.Fatalf("%s: expected global fail-open, %q was false", f, k)
			}
		}
	}
}

func TestPureDocumentationSkipsTheGoGate(t *testing.T) {
	sel := classify(t, nil, "README.md", "docs/foo.md", "docs/img/a.png")
	if sel.Values["go"] {
		t.Fatal("a pure documentation PR must not force the Go gate on")
	}
	if !sel.Values["docs"] {
		t.Fatal("markdown must set docs=true")
	}
}

// A file whose extension is not on the document denylist is a build input.
// This is the polarity that keeps a newly introduced asset kind from silently
// disabling Build & Test.
func TestUnknownExtensionCountsAsBuildInput(t *testing.T) {
	for _, f := range []string{
		"pkg/source/sqlite/schema.sql",
		"agm/internal/contracts/slo-contracts.yaml",
		"agm/internal/dolt/migrations/020_new.sql",
		"engram/internal/harnesseffort/harness-effort-defaults.yaml",
		"cmd/some-tool/some-new-asset-kind.wasm",
	} {
		if sel := classify(t, nil, f); !sel.Values["go"] {
			t.Fatalf("%s: non-document build input must set go=true", f)
		}
	}
}

// Markdown that is compiled into a binary is product, not documentation.
func TestEmbeddedMarkdownIsABuildInput(t *testing.T) {
	roots := []string{"cmd/vroom-dispatch/skills/plan.md"}
	sel := classify(t, roots, "cmd/vroom-dispatch/skills/plan.md")
	if !sel.Values["go"] {
		t.Fatal("//go:embed-ed markdown must set go=true")
	}
	// A directory embed covers everything beneath it.
	sel = classify(t, []string{"pkg/codeintel/rules"}, "pkg/codeintel/rules/nested/x.md")
	if !sel.Values["go"] {
		t.Fatal("markdown under an embedded directory must set go=true")
	}
	// And an unrelated markdown file still does not.
	if sel := classify(t, roots, "cmd/vroom-dispatch/README.md"); sel.Values["go"] {
		t.Fatal("non-embedded markdown next to an embed root must not set go=true")
	}
}

// agm/agm-plugin/commands is content-hashed by `make plugin-verify-hashes`,
// which runs inside Build & Test. Skipping that job on such a PR would skip the
// only check that can catch a stale hash.
func TestHashVerifiedMarkdownIsABuildInput(t *testing.T) {
	if sel := classify(t, nil, "agm/agm-plugin/commands/foo.md"); !sel.Values["go"] {
		t.Fatal("hash-verified plugin markdown must set go=true")
	}
}

func TestAreaFlags(t *testing.T) {
	sel := classify(t, nil, "agm/internal/db/db.go")
	if !sel.Values["agm"] || sel.Values["engram"] {
		t.Fatalf("agm/** must set agm only: %v", sel.Values)
	}
	sel = classify(t, nil, "engram/internal/x.go")
	if !sel.Values["engram"] || sel.Values["agm"] {
		t.Fatalf("engram/** must set engram only: %v", sel.Values)
	}
	// A directory that merely shares a prefix must not match.
	if sel := classify(t, nil, "agmx/thing.go"); sel.Values["agm"] {
		t.Fatal("agmx/ must not match the agm/ area")
	}
	if sel := classify(t, nil, "docs/adr/ADR-999-x.md"); !sel.Values["adr"] {
		t.Fatal("docs/adr/** must set adr=true")
	}
	if sel := classify(t, nil, "web/pnpm-lock.yaml"); !sel.Values["deps"] {
		t.Fatal("a lockfile must set deps=true")
	}
}

// git reports a detected rename by destination path alone under --name-only.
// Renaming a Go file to a Markdown one would then look like a docs change and
// skip build and analysis, even though a compiled source file was deleted.
func TestRenameReportsBothSides(t *testing.T) {
	out := "R100\x00pkg/x/a.go\x00docs/a.md\x00"
	files := ParseNameStatusZ(out)
	if len(files) != 2 {
		t.Fatalf("rename must yield both sides, got %v", files)
	}
	if sel := classify(t, nil, files...); !sel.Values["go"] {
		t.Fatal("renaming a .go file away must still set go=true")
	}
}

func TestParseNameStatusZ(t *testing.T) {
	out := strings.Join([]string{
		"M", "pkg/a.go",
		"A", "docs/b.md",
		"R096", "old/c.go", "new/c.go",
		"C075", "src/d.go", "copy/d.go",
		"D", "pkg/with space/e.go",
	}, "\x00") + "\x00"
	got := ParseNameStatusZ(out)
	want := []string{
		"copy/d.go", "docs/b.md", "new/c.go", "old/c.go",
		"pkg/a.go", "pkg/with space/e.go", "src/d.go",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

// The old shell implementation piped the file list into `grep -q` under
// `set -o pipefail`. For a list larger than the pipe buffer, grep exits on the
// first match, the writer takes SIGPIPE, and the pipeline returns 141 — read as
// "no match", which under-selects exactly on the largest PRs. The Go
// classifier has no pipeline; this asserts the behaviour it replaced.
func TestLargeChangeSetWithEarlyMatchStillSelects(t *testing.T) {
	files := []string{"aaa/first.go"}
	for i := range 20000 {
		files = append(files, "docs/zzz/note-"+strings.Repeat("x", 40)+string(rune('a'+i%26))+".md")
	}
	if sel := classify(t, nil, files...); !sel.Values["go"] {
		t.Fatal("an early match in a very large change set must still select")
	}
}
