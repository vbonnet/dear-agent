package codeintel

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DetectLanguages scans cwd for manifest files and source globs to determine
// which languages are used in the project. Returns matching LanguageSpecs.
func DetectLanguages(cwd string) []LanguageSpec {
	var detected []LanguageSpec
	for _, spec := range BuiltinSpecs {
		if detectByManifest(cwd, spec) || detectBySourceGlob(cwd, spec) {
			detected = append(detected, spec)
		}
	}
	return detected
}

// detectByManifest checks if any of the spec's manifest files exist in cwd.
func detectByManifest(cwd string, spec LanguageSpec) bool {
	for _, mf := range spec.ManifestFiles {
		path := filepath.Join(cwd, mf)
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

// detectBySourceGlob checks if any source files matching the spec's globs exist.
// This is a slower fallback used only when no manifest file is found.
// Supports ** glob patterns by walking the directory tree.
func detectBySourceGlob(cwd string, spec LanguageSpec) bool {
	for _, pattern := range spec.SourceGlobs {
		if matchSourceGlob(cwd, pattern) {
			return true
		}
	}
	return false
}

// matchSourceGlob matches a glob pattern against files in cwd.
// Handles ** patterns by walking the tree; uses filepath.Glob for simple patterns.
func matchSourceGlob(cwd, pattern string) bool {
	if !strings.Contains(pattern, "**") {
		matches, err := filepath.Glob(filepath.Join(cwd, pattern))
		return err == nil && len(matches) > 0
	}

	// For ** patterns, extract the extension and walk
	ext := filepath.Ext(pattern)
	if ext == "" {
		return false
	}
	found := false
	_ = filepath.WalkDir(cwd, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // intentional: caller signals via separate bool/optional
		}
		if !d.IsDir() && filepath.Ext(path) == ext {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

// DetectAvailableTier determines the highest available verification tier for
// a given language spec by checking which tools are installed.
//
// Returns Tier2 if any semantic tool (deadcode, build) is found,
// Tier1 if ast-grep is installed, or Tier0 (always available).
func DetectAvailableTier(spec LanguageSpec) int {
	// Tier 2: check if semantic tools exist
	if len(spec.DeadcodeCmd) > 0 && commandExists(spec.DeadcodeCmd[0]) {
		return Tier2
	}
	if len(spec.BuildCmd) > 0 && commandExists(spec.BuildCmd[0]) {
		return Tier2
	}
	// Tier 1: check if ast-grep is installed and language is supported
	if spec.ASTGrepLang != "" && commandExists("ast-grep") {
		return Tier1
	}
	// Tier 0: always available
	return Tier0
}

// commandExists checks whether a command is available on PATH.
func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
