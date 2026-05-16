package llmwiki

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/source"
)

// TestPathFromURI_FailsClosedOnTraversal is the regression test for the
// PR #46 gemini security P0 (incomplete path-traversal fix). The original
// fix only added a containment check at the Add() call site; pathFromURI
// itself still returned `../../etc/passwd.md`-style paths. The sanitiser
// must now reject any path that escapes the wiki root.
func TestPathFromURI_FailsClosedOnTraversal(t *testing.T) {
	reject := []string{
		"wiki://../../etc/passwd",
		"wiki:///../etc/passwd",
		"wiki://../secret",
		"wiki://a/b/../../../etc/passwd",
		"../../etc/passwd", // generic slug branch
		"../escape",        // generic slug branch
		"/etc/passwd",      // absolute via generic branch
		"wiki://..",        // bare parent
	}
	for _, uri := range reject {
		if got := pathFromURI(uri); got != "" {
			t.Errorf("pathFromURI(%q) = %q, want \"\" (must fail closed — traversal)", uri, got)
		}
	}

	accept := map[string]string{
		"wiki:///pages/research.md": "pages/research.md",
		"wiki://notes/foo":          "notes/foo.md",
		"wiki://a/../b":             "b.md", // inner .. that does NOT escape is fine
		"https://example.com/a/b":   "example.com/a/b.md",
	}
	for uri, want := range accept {
		if got := pathFromURI(uri); got != want {
			t.Errorf("pathFromURI(%q) = %q, want %q (legitimate path must still work)", uri, got, want)
		}
	}
}

// TestAdd_TraversalWritesNothingOutsideRoot proves end to end that a
// hostile URI cannot land a file outside the wiki root.
func TestAdd_TraversalWritesNothingOutsideRoot(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "wiki")
	a, err := Open(root)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer a.Close()

	escaped := filepath.Join(tmp, "escape.md") // sibling of root, outside it
	_, err = a.Add(context.Background(), source.Source{
		URI:       "wiki://../escape",
		Content:   []byte("pwned"),
		IndexedAt: time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("Add accepted a traversal URI — expected rejection")
	}
	if _, statErr := os.Stat(escaped); statErr == nil {
		t.Fatalf("TRAVERSAL: file written outside wiki root at %s", escaped)
	}
}
