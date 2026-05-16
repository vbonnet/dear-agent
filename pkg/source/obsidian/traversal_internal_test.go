package obsidian

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/source"
)

// TestPathFromURI_FailsClosedOnTraversal is the regression test for the
// PR #46 gemini security P0 (incomplete path-traversal fix). pathFromURI
// (and slugFile) used to return `../../secret.md`-style paths, relying
// solely on a downstream containment check. The sanitiser must now reject
// anything that escapes the vault root.
func TestPathFromURI_FailsClosedOnTraversal(t *testing.T) {
	// Genuine traversal via the obsidian:// scheme path component — the
	// exact vector gemini flagged (obsidian:///../secret). The bare-slug
	// branch is NOT in scope: slugFile already maps '.' -> '-', so a ".."
	// segment can never survive into the path there.
	reject := []string{
		"obsidian:///../../secret",
		"obsidian:///../escape",
		"obsidian:///../../../etc/passwd",
		"obsidian:///a/b/../../../../etc/passwd",
	}
	for _, uri := range reject {
		got, err := pathFromURI(uri)
		if err == nil {
			t.Errorf("pathFromURI(%q) = %q, nil — want error (must fail closed — traversal)", uri, got)
		}
	}

	accept := map[string]string{
		"obsidian:///notes/research.md": "notes/research.md",
		"obsidian://Note":               "Note.md",
		"obsidian:///a/../b":            "b.md",                // inner .. that does NOT escape is fine
		"obsidian://../escape":          "escape.md",           // ".." parsed as host and dropped — safe
		"https://example.com/a/b":       "example-com/a/b.md",  // slug branch maps '.' -> '-'
		"../../etc/passwd":              "--/--/etc/passwd.md", // slug branch neutralises '.'
	}
	for uri, want := range accept {
		got, err := pathFromURI(uri)
		if err != nil {
			t.Errorf("pathFromURI(%q) returned error %v — legitimate path must still work", uri, err)
			continue
		}
		if got != want {
			t.Errorf("pathFromURI(%q) = %q, want %q", uri, got, want)
		}
	}
}

// TestAdd_TraversalWritesNothingOutsideRoot proves end to end that a
// hostile URI cannot land a file outside the vault root.
func TestAdd_TraversalWritesNothingOutsideRoot(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "vault")
	a, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	escaped := filepath.Join(tmp, "escape.md") // sibling of root, outside it
	_, err = a.Add(context.Background(), source.Source{
		URI:       "obsidian:///../escape",
		Content:   []byte("pwned"),
		IndexedAt: time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("Add accepted a traversal URI — expected rejection")
	}
	if !strings.Contains(err.Error(), "traversal") && !strings.Contains(err.Error(), "invalid path") {
		t.Logf("note: rejection error was %q", err)
	}
	if _, statErr := os.Stat(escaped); statErr == nil {
		t.Fatalf("TRAVERSAL: file written outside vault root at %s", escaped)
	}
}
