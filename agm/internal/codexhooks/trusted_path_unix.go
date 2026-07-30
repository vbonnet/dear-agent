//go:build !windows

package codexhooks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func validateTrustedExecutableSearchPath(searchPath string) error {
	for _, candidate := range filepath.SplitList(searchPath) {
		if err := validateTrustedExecutablePathEntry(candidate); err != nil {
			return err
		}
	}
	return nil
}

func validateTrustedHookExecutable(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("trusted hook executable %q must be a clean absolute path", path)
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("resolve trusted hook executable %q: %w", path, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return fmt.Errorf("inspect trusted hook executable %q: %w", resolved, err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("inspect ownership of trusted hook executable %q", resolved)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 ||
		stat.Uid != 0 || info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf(
			"trusted hook executable %q must be an operator-owned, non-writable executable regular file",
			resolved,
		)
	}
	return validateOperatorOwnedPathAncestors(filepath.Dir(resolved))
}

func validateTrustedExecutablePathEntry(candidate string) error {
	if candidate == "" || !filepath.IsAbs(candidate) || filepath.Clean(candidate) != candidate {
		return fmt.Errorf("attested hook PATH entry %q must be a clean absolute path", candidate)
	}
	existing, err := existingPathAncestor(candidate)
	if err != nil {
		return err
	}
	resolved, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return fmt.Errorf("resolve attested hook PATH entry %q: %w", candidate, err)
	}
	return validateOperatorOwnedPathAncestors(filepath.Clean(resolved))
}

func existingPathAncestor(candidate string) (string, error) {
	for existing := candidate; ; existing = filepath.Dir(existing) {
		_, err := os.Lstat(existing)
		if err == nil {
			return existing, nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect attested hook PATH entry %q: %w", candidate, err)
		}
		if filepath.Dir(existing) == existing {
			return "", fmt.Errorf("attested hook PATH entry %q has no existing ancestor", candidate)
		}
	}
}

func validateOperatorOwnedPathAncestors(path string) error {
	for current := path; ; current = filepath.Dir(current) {
		info, err := os.Stat(current)
		if err != nil {
			return fmt.Errorf("inspect attested hook PATH ancestor %q: %w", current, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("attested hook PATH ancestor %q is not a directory", current)
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("inspect ownership of attested hook PATH ancestor %q", current)
		}
		if stat.Uid != 0 || info.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf(
				"attested hook PATH ancestor %q must be operator-owned and not group/world writable (owner=%d mode=%s)",
				current, stat.Uid, strings.TrimPrefix(info.Mode().Perm().String(), "d"),
			)
		}
		if filepath.Dir(current) == current {
			return nil
		}
	}
}
