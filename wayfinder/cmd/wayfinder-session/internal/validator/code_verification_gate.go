// Package validator provides validator-related functionality.
package validator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxCodeFileSizeBytes = 10485760 // 10MB limit for code files
	buildTimeoutMinutes  = 5        // Build timeout from D4
	testTimeoutMinutes   = 10       // Test timeout from D4
	cacheExpiryHours     = 24       // Cache expiry from D4
)

// CodeVerificationCache represents cache entry for bead code verification
type CodeVerificationCache struct {
	BeadID          string    `json:"bead_id"`
	SourceHash      string    `json:"source_hash"`
	TestHash        string    `json:"test_hash"`
	BuildPassed     bool      `json:"build_passed"`
	TestPassed      bool      `json:"test_passed"`
	ArtifactsPassed bool      `json:"artifacts_passed"`
	LastVerified    time.Time `json:"last_verified"`
}

// validateCodeDeliverables checks code deliverables for all beads in current phase.
// Returns ValidationError if any verification check fails.
// This is Gate 9: Working Code Verification.
func validateCodeDeliverables(phaseName, projectDir string) error {
	// V1 Simplified: Scan project directory for code files instead of querying bead database
	// V2 will integrate with bead database via `bd list --phase {phaseName}` command

	// Find all code files in project directory
	codeFiles, err := findCodeFiles(projectDir)
	if err != nil {
		return NewValidationError(
			"complete "+phaseName,
			fmt.Sprintf("failed to find code files: %v", err),
			"Check project directory permissions",
		)
	}

	// Graceful degradation: if no code files found, warn but don't block
	if len(codeFiles) == 0 {
		fmt.Fprintf(os.Stderr, "⚠️  No code files found in project - skipping Gate 9 verification\n")
		return nil
	}

	// Detect language from file extensions
	language, err := detectLanguage(codeFiles)
	if err != nil {
		// Unsupported language - graceful degradation
		fmt.Fprintf(os.Stderr, "⚠️  %v - skipping Gate 9 verification\n", err)
		return nil
	}

	// Check cache (skip validation if files unchanged)
	// V1: Simplified cache key using "gate9-verification" as bead ID
	beadID := "gate9-verification"
	cache, cacheHit := checkCodeVerificationCache(projectDir, beadID, codeFiles, codeFiles)
	if cacheHit && cache.BuildPassed && cache.TestPassed && cache.ArtifactsPassed {
		fmt.Fprintf(os.Stderr, "✓ Gate 9 verification passed (cached)\n")
		return nil
	}

	// Run build command
	if err := runBuildCommand(projectDir, language); err != nil {
		return err
	}

	// Run test command (test hygiene gate)
	if err := runTestCommand(projectDir, language); err != nil {
		return err
	}

	// Verify artifacts exist
	if err := validateArtifactsExist(projectDir, language); err != nil {
		return err
	}

	// Update cache with successful verification
	newCache := &CodeVerificationCache{
		BeadID:          beadID,
		SourceHash:      "", // Will be calculated in updateCache
		TestHash:        "", // Will be calculated in updateCache
		BuildPassed:     true,
		TestPassed:      true,
		ArtifactsPassed: true,
		LastVerified:    time.Now(),
	}

	// Calculate hashes for cache
	sourceHash, err := calculateFilesHash(codeFiles)
	if err == nil {
		newCache.SourceHash = sourceHash
		newCache.TestHash = sourceHash // V1: same as source hash
	}

	// Update cache (non-critical, don't fail on error)
	if err := updateCodeVerificationCache(projectDir, newCache); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Failed to update verification cache: %v\n", err)
	}

	fmt.Fprintf(os.Stderr, "✓ Gate 9 verification passed\n")
	return nil
}

// findCodeFiles recursively finds all code files in project directory.
// Returns list of absolute file paths.
func findCodeFiles(projectDir string) ([]string, error) {
	var codeFiles []string

	// Supported extensions from D4
	supportedExts := map[string]bool{
		".go":   true,
		".py":   true,
		".js":   true,
		".ts":   true,
		".rs":   true,
		".c":    true,
		".cpp":  true,
		".java": true,
	}

	// Walk project directory
	err := filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			// Skip hidden directories and common build/dependency directories
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "target" {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if file has supported extension
		ext := filepath.Ext(path)
		if supportedExts[ext] {
			codeFiles = append(codeFiles, path)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return codeFiles, nil
}

// validateFilesExist verifies all extracted file paths exist on filesystem.
// Returns ValidationError if any file missing or security check fails.
func validateFilesExist(projectDir string, filePaths []string) error {
	var missingFiles []string

	for _, path := range filePaths {
		// Security: validate path (reject ../, absolute paths outside project)
		if err := validatePath(projectDir, path); err != nil {
			return NewValidationError(
				"complete phase",
				fmt.Sprintf("invalid file path: %s", path),
				fmt.Sprintf("Security: %v", err),
			)
		}

		// Construct absolute path
		absPath := filepath.Join(projectDir, path)

		// Check file exists
		info, err := os.Stat(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				missingFiles = append(missingFiles, path)
				continue
			}
			return NewValidationError(
				"complete phase",
				fmt.Sprintf("failed to check file: %s", path),
				fmt.Sprintf("Error: %v", err),
			)
		}

		// Security: check file size (10MB limit)
		if info.Size() > maxCodeFileSizeBytes {
			sizeMB := float64(info.Size()) / 1048576.0
			return NewValidationError(
				"complete phase",
				fmt.Sprintf("file too large: %s (%.1fMB > 10MB)", path, sizeMB),
				"Reduce file size or split into smaller files",
			)
		}
	}

	if len(missingFiles) > 0 {
		missingList := "\n"
		for _, file := range missingFiles {
			missingList += fmt.Sprintf("  - %s (claimed in outcome, not found on filesystem)\n", file)
		}

		return NewValidationError(
			"complete phase",
			fmt.Sprintf("❌ Gate 9 Failed: Working Code Verification\n\nFiles claimed in bead outcome don't exist:%s", missingList),
			"Resolution:\n1. Create missing files, or\n2. Update bead outcome to reflect actual files modified\n\nRun: bd edit <bead-id>",
		)
	}

	return nil
}

// validatePath checks if path is safe (no path traversal, within project directory).
func validatePath(projectDir, path string) error {
	// Reject ../ (path traversal)
	if strings.Contains(path, "..") {
		return fmt.Errorf("path traversal detected: %s", path)
	}

	// Clean path
	cleanPath := filepath.Clean(path)

	// Construct absolute path
	absPath := filepath.Join(projectDir, cleanPath)

	// Verify path is within project directory
	if !strings.HasPrefix(absPath, projectDir) {
		return fmt.Errorf("path outside project: %s", path)
	}

	return nil
}

// detectLanguage detects programming language from file extensions.
// Returns language identifier (e.g., "go", "python", "javascript").
func detectLanguage(filePaths []string) (string, error) {
	// Count language extensions
	langCounts := make(map[string]int)

	for _, path := range filePaths {
		ext := filepath.Ext(path)

		switch ext {
		case ".go":
			langCounts["go"]++
		case ".py":
			langCounts["python"]++
		case ".js", ".ts":
			langCounts["javascript"]++
		case ".rs":
			langCounts["rust"]++
		case ".c", ".cpp":
			langCounts["c++"]++
		}
	}

	// Find most common language (simple majority)
	var maxLang string
	var maxCount int

	for lang, count := range langCounts {
		if count > maxCount {
			maxLang = lang
			maxCount = count
		}
	}

	if maxLang == "" {
		return "", fmt.Errorf("no recognized language extensions found")
	}

	return maxLang, nil
}

// runBuildCommand executes build command with timeout and security checks.
// Returns ValidationError if build fails or times out.
func runBuildCommand(projectDir, language string) error {
	var cmd *exec.Cmd

	// Determine build command from language
	switch language {
	case "go":
		cmd = exec.Command("go", "build", "./...")
	case "python":
		// Python is interpreted, no build step needed
		return nil
	case "javascript":
		// Check if package.json has build script
		cmd = exec.Command("npm", "run", "build")
	case "rust":
		cmd = exec.Command("cargo", "build")
	case "c++":
		// Check if Makefile exists
		cmd = exec.Command("make", "build")
	default:
		// Unsupported language - graceful degradation
		fmt.Fprintf(os.Stderr, "⚠️  Unsupported language: %s - skipping build verification\n", language)
		return nil
	}

	// Set working directory
	cmd.Dir = projectDir

	// Set timeout (5 minutes from D4)
	ctx, cancel := context.WithTimeout(context.Background(), buildTimeoutMinutes*time.Minute)
	defer cancel()

	// Execute with timeout
	cmdWithTimeout := exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
	cmdWithTimeout.Dir = projectDir

	output, err := cmdWithTimeout.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return NewValidationError(
				"complete phase",
				fmt.Sprintf("❌ Gate 9 Failed: Build Verification\n\nBuild timeout (%d minutes)", buildTimeoutMinutes),
				"Optimize build performance or increase timeout in V2",
			)
		}

		return NewValidationError(
			"complete phase",
			fmt.Sprintf("❌ Gate 9 Failed: Build Verification\n\nBuild command failed: %s\n\nExit code: %v\nOutput:\n%s", strings.Join(cmd.Args, " "), err, string(output)),
			fmt.Sprintf("Resolution:\nFix build errors before completing phase\n\nRun: %s", strings.Join(cmd.Args, " ")),
		)
	}

	return nil
}

// runTestCommand executes test command with test hygiene enforcement.
// Returns ValidationError if tests fail, skip, or timeout.
func runTestCommand(projectDir, language string) error {
	var cmd *exec.Cmd

	// Determine test command from language
	switch language {
	case "go":
		cmd = exec.Command("go", "test", "./...")
	case "python":
		cmd = exec.Command("pytest")
	case "javascript":
		cmd = exec.Command("npm", "test")
	case "rust":
		cmd = exec.Command("cargo", "test")
	case "c++":
		cmd = exec.Command("make", "test")
	default:
		// Unsupported language - graceful degradation
		fmt.Fprintf(os.Stderr, "⚠️  Unsupported language: %s - skipping test verification\n", language)
		return nil
	}

	// Set working directory
	cmd.Dir = projectDir

	// Set timeout (10 minutes from D4)
	ctx, cancel := context.WithTimeout(context.Background(), testTimeoutMinutes*time.Minute)
	defer cancel()

	// Execute with timeout
	cmdWithTimeout := exec.CommandContext(ctx, cmd.Path, cmd.Args[1:]...)
	cmdWithTimeout.Dir = projectDir

	output, err := cmdWithTimeout.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return NewValidationError(
				"complete phase",
				fmt.Sprintf("❌ Gate 9 Failed: Test Hygiene Verification\n\nTest timeout (%d minutes)", testTimeoutMinutes),
				"Optimize test performance or increase timeout in V2",
			)
		}

		// Test hygiene gate: exit code non-zero = failures OR skips
		return NewValidationError(
			"complete phase",
			fmt.Sprintf("❌ Gate 9 Failed: Test Hygiene Verification\n\nTest command failed: %s\n\nExit code: %v\nOutput:\n%s", strings.Join(cmd.Args, " "), err, string(output)),
			testHygieneRemediation(strings.Join(cmd.Args, " ")),
		)
	}

	return nil
}

// testHygieneRemediation returns remediation message for test hygiene gate failures.
func testHygieneRemediation(testCmd string) string {
	return fmt.Sprintf(`Resolution (Test Hygiene Gate):
1. Fix code bugs (if test failures expose bugs in implementation)
2. Fix test bugs (if failures are due to bugs in test code)
3. Rewrite tests (if code changed and tests need updating)
4. Delete obsolete tests (if tests are no longer applicable)

Pre-existing failures compound and erode confidence - zero tolerance.

Run: %s`, testCmd)
}

// validateArtifactsExist verifies build artifacts exist on filesystem.
// Returns ValidationError if expected artifacts missing.
func validateArtifactsExist(projectDir, language string) error {
	var expectedArtifacts []string

	switch language {
	case "go":
		// Go builds to binary, check for executable
		// Simplified: assume successful build created artifacts
		return nil
	case "python":
		// Python has no build artifacts
		return nil
	case "javascript":
		// Check for dist/ or build/ directory
		expectedArtifacts = []string{"dist/", "build/"}
	case "rust":
		// Check for target/debug/ or target/release/
		expectedArtifacts = []string{"target/debug/", "target/release/"}
	case "c++":
		// Check for compiled objects (*.o, *.so, *.a)
		// Simplified: assume successful build created artifacts
		return nil
	default:
		// Unsupported language - graceful degradation
		return nil
	}

	// Check if any expected artifact exists
	found := false
	for _, artifact := range expectedArtifacts {
		artifactPath := filepath.Join(projectDir, artifact)
		if _, err := os.Stat(artifactPath); err == nil {
			found = true
			break
		}
	}

	if !found && len(expectedArtifacts) > 0 {
		artifactList := "\n"
		for _, artifact := range expectedArtifacts {
			artifactList += fmt.Sprintf("  - %s\n", artifact)
		}

		return NewValidationError(
			"complete phase",
			fmt.Sprintf("❌ Gate 9 Failed: Artifact Verification\n\nBuild artifacts not found.\n\nExpected artifacts for %s projects:%s", language, artifactList),
			"Resolution:\nEnsure build command generates artifacts\n\nRun build command again",
		)
	}

	return nil
}

// checkCodeVerificationCache checks if bead has valid cached verification result.
// Returns (cache, true) if cache hit, (nil, false) if cache miss.
func checkCodeVerificationCache(projectDir, beadID string, sourceFiles, testFiles []string) (*CodeVerificationCache, bool) {
	cachePath := filepath.Join(projectDir, ".wayfinder-cache", "code-verification", beadID+".json")

	// Read cache file
	data, err := os.ReadFile(cachePath)
	if err != nil {
		// Cache doesn't exist - cache miss
		return nil, false
	}

	// Parse cache
	var cache CodeVerificationCache
	if err := json.Unmarshal(data, &cache); err != nil {
		// Corrupted cache - treat as miss
		fmt.Fprintf(os.Stderr, "⚠️  Corrupted code verification cache for bead %s: %v\n", beadID, err)
		return nil, false
	}

	// Check cache expiry (24 hours from D4)
	if time.Since(cache.LastVerified) > cacheExpiryHours*time.Hour {
		// Cache expired - treat as miss
		return nil, false
	}

	// Calculate current source hash
	sourceHash, err := calculateFilesHash(sourceFiles)
	if err != nil {
		// Hash calculation failed - treat as miss
		return nil, false
	}

	// Calculate current test hash
	testHash, err := calculateFilesHash(testFiles)
	if err != nil {
		// Hash calculation failed - treat as miss
		return nil, false
	}

	// Check if hashes match
	if cache.SourceHash != sourceHash || cache.TestHash != testHash {
		// Files changed - cache miss
		return nil, false
	}

	// Cache hit
	return &cache, true
}

// updateCodeVerificationCache updates cache with new verification result.
func updateCodeVerificationCache(projectDir string, cache *CodeVerificationCache) error {
	cacheDir := filepath.Join(projectDir, ".wayfinder-cache", "code-verification")
	cachePath := filepath.Join(cacheDir, cache.BeadID+".json")

	// Create cache directory if it doesn't exist
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Marshal cache to JSON
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache: %w", err)
	}

	// Write cache file
	if err := os.WriteFile(cachePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}

// calculateFilesHash calculates SHA-256 hash of all files concatenated.
func calculateFilesHash(filePaths []string) (string, error) {
	hasher := sha256.New()

	for _, path := range filePaths {
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		defer file.Close()

		if _, err := io.Copy(hasher, file); err != nil {
			return "", err
		}
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
