package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHasKnownPrefix(t *testing.T) {
	tests := []struct {
		ref  string
		want bool
	}{
		{".github/workflows/ci.yml", true},
		{"cmd/agm/main.go", true},
		{"agm/internal/ops/session_get.go", true},
		{"infra/providers.tf", true},
		// Outside the repository's real top-level directories: illustrative,
		// not a claim about this tree.
		{"vendor/example/thing.go", false},
		{"/etc/hosts.yml", false},
		{"node_modules/x/y.json", false},
	}
	for _, tt := range tests {
		if got := hasKnownPrefix(tt.ref); got != tt.want {
			t.Errorf("hasKnownPrefix(%q) = %v, want %v", tt.ref, got, tt.want)
		}
	}
}

func TestRefPatternMatchesOnlyBacktickedPaths(t *testing.T) {
	line := "See `cmd/agm/main.go` and cmd/agm/other.go plus `docs/x.md`."
	got := refPattern.FindAllStringSubmatch(line, -1)
	if len(got) != 2 {
		t.Fatalf("expected 2 backticked matches, got %d: %v", len(got), got)
	}
	if got[0][1] != "cmd/agm/main.go" || got[1][1] != "docs/x.md" {
		t.Errorf("unexpected captures: %q, %q", got[0][1], got[1][1])
	}
}

func TestResolvesAgainstRootThenAncestors(t *testing.T) {
	tracked := map[string]bool{
		"cmd/agm/main.go":                 true,
		"agm/cmd/agm/new.go":              true,
		"agm/internal/ops/session_get.go": true,
	}
	tests := []struct {
		name string
		doc  string
		ref  string
		want bool
	}{
		{"repo-root reference", "docs/guide.md", "cmd/agm/main.go", true},
		// agm/SPEC.md legitimately cites paths relative to its own subtree;
		// resolving only from the root would call these broken.
		{"subtree-relative reference", "agm/SPEC.md", "cmd/agm/new.go", true},
		{"deep subtree-relative", "agm/cmd/agm/SPEC.md", "internal/ops/session_get.go", true},
		{"genuinely missing", "docs/guide.md", "cmd/agm/absent.go", false},
		{"missing from subtree too", "agm/SPEC.md", "cmd/agm/absent.go", false},
	}
	for _, tt := range tests {
		if got := resolves(tt.doc, tt.ref, tracked); got != tt.want {
			t.Errorf("%s: resolves(%q, %q) = %v, want %v", tt.name, tt.doc, tt.ref, got, tt.want)
		}
	}
}

func TestScanDocReportsOnlyMissingKnownPrefixRefs(t *testing.T) {
	dir := t.TempDir()
	doc := filepath.Join(dir, "sample.md")
	body := "" +
		"# Sample\n" +
		"Present: `cmd/agm/main.go`\n" +
		"Missing: `.github/workflows/ghost.yml`\n" +
		"Illustrative: `vendor/other/thing.go`\n" +
		"Unbackticked: .github/workflows/also-ghost.yml\n"
	if err := os.WriteFile(doc, []byte(body), 0o600); err != nil {
		t.Fatalf("write sample: %v", err)
	}
	got, err := scanDoc(doc, map[string]bool{"cmd/agm/main.go": true})
	if err != nil {
		t.Fatalf("scanDoc: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 finding, got %d: %+v", len(got), got)
	}
	if got[0].ref != ".github/workflows/ghost.yml" {
		t.Errorf("ref = %q, want .github/workflows/ghost.yml", got[0].ref)
	}
	if got[0].line != 3 {
		t.Errorf("line = %d, want 3", got[0].line)
	}
}

// A document that cannot be read must surface as an error, never as "no
// findings" — a guardrail that silently passes on the files it failed to
// inspect is worse than no guardrail.
func TestScanDocSurfacesUnreadableDocumentAsError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist.md")
	got, err := scanDoc(missing, map[string]bool{})
	if err == nil {
		t.Fatalf("expected an error for an unreadable document, got findings=%v", got)
	}
	if got != nil {
		t.Errorf("expected no findings alongside the error, got %v", got)
	}
}
