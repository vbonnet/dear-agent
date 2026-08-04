package main

import (
	"os/exec"
	"strings"
)

const unknownGitCommit = "unknown"

type gitOutput func(dir string, args ...string) ([]byte, error)

// defaultGitCommit returns one linker-safe provenance token. Git discovery is
// intentionally fail-closed as data: an indeterminate revision or worktree is
// unknown, never a clean commit claim.
func defaultGitCommit(dir string, output gitOutput) string {
	revisionOutput, err := output(dir, "rev-parse", "--short=12", "HEAD")
	if err != nil {
		return unknownGitCommit
	}
	revision := strings.TrimSpace(string(revisionOutput))
	if !isShortGitRevision(revision) {
		return unknownGitCommit
	}

	status, err := output(dir, "status", "--porcelain=v1", "--ignored=matching", "--untracked-files=normal", "--ignore-submodules=none")
	if err != nil {
		return unknownGitCommit
	}
	if isWorktreeDirty(string(status)) {
		return revision + "-dirty"
	}
	return revision
}

func isWorktreeDirty(status string) bool {
	lines := strings.Split(status, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		if strings.HasPrefix(line, "!! ") {
			path := strings.TrimSpace(line[3:])
			if isCompilableSource(path) {
				return true
			}
		} else {
			return true
		}
	}
	return false
}

func isCompilableSource(path string) bool {
	exts := []string{".go", ".mod", ".sum", ".c", ".h", ".s", ".cc", ".cpp", ".proto"}
	for _, ext := range exts {
		if strings.HasSuffix(path, ext) {
			return true
		}
	}
	return false
}

func runGit(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	// Treat a successful command that emits a warning as non-clean evidence too.
	return cmd.CombinedOutput()
}

func isShortGitRevision(value string) bool {
	// --short=12 requests a minimum length. Git may extend the abbreviation
	// when twelve hexadecimal characters are not unique, and SHA-256 object
	// repositories can require up to 64.
	if len(value) < 12 || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
