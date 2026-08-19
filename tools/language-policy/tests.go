package main

import (
	"os"
	"path"
	"path/filepath"
	"strings"
)

// testDir holds the repository's shell test suite. The shell-matrix workflow
// already runs these across several interpreters; the policy reuses that
// harness rather than introducing a second notion of "tested".
const testDir = "tests/bats"

// TestIndex records which shell scripts are exercised by a bats test.
type TestIndex struct{ covered map[string]bool }

// BuildTestIndex scans the bats suite for references to shell scripts.
//
// Matching is by basename. A bats file invokes a script by whatever path is
// convenient (repo-relative, "$BATS_TEST_DIRNAME/..", a variable), so matching
// full paths would report almost everything as untested. Basename matching can
// credit the wrong script when two scripts share a name; that is the safe
// direction for a rule whose purpose is to stop *new* untested shell, and the
// waiver store still records anything deliberately exempt.
func BuildTestIndex(repoRoot string) (*TestIndex, error) {
	idx := &TestIndex{covered: map[string]bool{}}
	entries, err := os.ReadDir(filepath.Join(repoRoot, testDir))
	if err != nil {
		// No suite is not an error: the rule simply has no test evidence to
		// credit, and every over-limit script falls back to needing a waiver.
		if os.IsNotExist(err) {
			return idx, nil
		}
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".bats" {
			continue
		}
		b, rerr := os.ReadFile(filepath.Join(repoRoot, testDir, e.Name()))
		if rerr != nil {
			return nil, rerr
		}
		for _, name := range scriptRefs(string(b)) {
			idx.covered[name] = true
		}
	}
	return idx, nil
}

// isShellSeparator reports the characters that can bound a script path inside
// a bats file: whitespace, quoting, and common shell punctuation.
func isShellSeparator(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '"', '\'', '(', ')', ';', '`', '|', '&', '<', '>':
		return true
	}
	return false
}

// scriptRefs returns the basenames of every .sh path mentioned in a bats file.
func scriptRefs(content string) []string {
	var out []string
	for _, tok := range strings.FieldsFunc(content, isShellSeparator) {
		if strings.HasSuffix(tok, ".sh") {
			out = append(out, path.Base(tok))
		}
	}
	return out
}

// Covered reports whether a bats test references this script.
func (t *TestIndex) Covered(scriptPath string) bool {
	return t.covered[path.Base(normalizePath(scriptPath))]
}
