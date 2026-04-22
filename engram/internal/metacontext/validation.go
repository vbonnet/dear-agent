package metacontext

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateWorkingDir validates the working directory path to prevent path traversal attacks.
// Three-layer defense: reject patterns, canonicalization, existence check.
// Implements Security Mitigation M1 (Threat Model T1).
func ValidateWorkingDir(workingDir string) error {
	// Layer 1: Reject path traversal patterns
	if strings.Contains(workingDir, "..") {
		return fmt.Errorf("%w: contains path traversal", ErrInvalidWorkingDir)
	}

	// Layer 2: Canonicalization
	absPath, err := filepath.Abs(workingDir)
	if err != nil {
		return fmt.Errorf("%w: cannot resolve absolute path: %w", ErrInvalidWorkingDir, err)
	}

	cleanPath := filepath.Clean(absPath)
	if cleanPath != absPath {
		return fmt.Errorf("%w: path not canonical", ErrInvalidWorkingDir)
	}

	// Layer 3: Verify directory exists and is accessible
	info, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: directory does not exist", ErrInvalidWorkingDir)
		}
		return fmt.Errorf("%w: cannot access directory: %w", ErrInvalidWorkingDir, err)
	}

	if !info.IsDir() {
		return fmt.Errorf("%w: not a directory", ErrInvalidWorkingDir)
	}

	return nil
}

// validateMetacontext checks if cached metacontext is corrupted.
// Called during cache Get operations to detect nil slices, invalid tokens, etc.
// Implements Security Mitigation M2 (SRE CRITICAL #2).
func validateMetacontext(mc *Metacontext) error {
	if mc == nil {
		return fmt.Errorf("%w: nil metacontext", ErrCacheCorruption)
	}

	// Check nil slices (should be empty slices, not nil)
	if mc.Languages == nil || mc.Frameworks == nil || mc.Tools == nil ||
		mc.Conventions == nil || mc.Personas == nil {
		return fmt.Errorf("%w: nil slices detected", ErrCacheCorruption)
	}

	// Check token count
	tokens := estimateTokens(mc)
	if tokens > MaxMetacontextTokens {
		return fmt.Errorf("%w: %d tokens exceeds max %d", ErrCacheCorruption, tokens, MaxMetacontextTokens)
	}

	// Check array sizes
	if len(mc.Languages) > MaxLanguageSignals ||
		len(mc.Frameworks) > MaxFrameworkSignals ||
		len(mc.Tools) > MaxToolSignals ||
		len(mc.Conventions) > MaxConventions ||
		len(mc.Personas) > MaxPersonas {
		return fmt.Errorf("%w: signal counts exceed limits", ErrCacheCorruption)
	}

	return nil
}
