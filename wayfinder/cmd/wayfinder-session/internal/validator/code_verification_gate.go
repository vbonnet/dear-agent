// Package validator provides validator-related functionality.
package validator

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/boundedexec"
)

const (
	maxCodeFileSizeBytes = 10485760 // 10MB limit for code files
	buildTimeoutMinutes  = 5        // SPEC-defined build timeout
	testTimeoutMinutes   = 10       // SPEC-defined test timeout
	cacheExpiryHours     = 24       // SPEC-defined cache expiry
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
	// ADR-001 scopes the build and test commands to BUILD completion: they are
	// evidence that an implementation works. Every other phase delivers prose,
	// so running the project's whole toolchain there verifies nothing about the
	// deliverable while costing the full build and test suite of whatever
	// repository the session happens to live in.
	if !phaseRunsCodeVerification(phaseName) {
		fmt.Fprintf(os.Stderr, "ℹ️  Gate 9: %s has no code deliverables - skipping build and test verification\n", phaseName)
		return nil
	}

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
		newCache.TestHash = sourceHash
	}

	// Update cache (non-critical, don't fail on error)
	if err := updateCodeVerificationCache(projectDir, newCache); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Failed to update verification cache: %v\n", err)
	}

	fmt.Fprintf(os.Stderr, "✓ Gate 9 verification passed\n")
	return nil
}

// phaseRunsCodeVerification reports whether a phase produces code deliverables,
// and therefore whether Gate 9 should shell out to the project's toolchain.
// See ADR-001: the gate verifies BUILD completion.
func phaseRunsCodeVerification(phaseName string) bool {
	return phaseName == "BUILD"
}

// findCodeFiles recursively finds all code files in project directory.
// Returns list of absolute file paths.
func findCodeFiles(projectDir string) ([]string, error) {
	var codeFiles []string

	// Supported extensions from the canonical SPEC contract.
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

// gateCommand names the fixed build and test command for a language, or
// reports that the language needs no such step.
type gateCommand struct {
	name string
	args []string
	skip bool
}

// buildCommandFor returns the fixed build command for a language.
func buildCommandFor(language string) (gateCommand, bool) {
	switch language {
	case "go":
		return gateCommand{name: "go", args: []string{"build", "./..."}}, true
	case "python":
		// Python is interpreted, no build step needed.
		return gateCommand{skip: true}, true
	case "javascript":
		return gateCommand{name: "npm", args: []string{"run", "build"}}, true
	case "rust":
		return gateCommand{name: "cargo", args: []string{"build"}}, true
	case "c++":
		return gateCommand{name: "make", args: []string{"build"}}, true
	default:
		return gateCommand{}, false
	}
}

// testCommandFor returns the fixed test command for a language.
func testCommandFor(language string) (gateCommand, bool) {
	switch language {
	case "go":
		return gateCommand{name: "go", args: []string{"test", "./..."}}, true
	case "python":
		return gateCommand{name: "pytest"}, true
	case "javascript":
		return gateCommand{name: "npm", args: []string{"test"}}, true
	case "rust":
		return gateCommand{name: "cargo", args: []string{"test"}}, true
	case "c++":
		return gateCommand{name: "make", args: []string{"test"}}, true
	default:
		return gateCommand{}, false
	}
}

// runBuildCommand executes the build command under a real wall-clock bound.
// Returns ValidationError if the build fails or times out.
func runBuildCommand(projectDir, language string) error {
	spec, supported := buildCommandFor(language)
	if !supported {
		fmt.Fprintf(os.Stderr, "⚠️  Unsupported language: %s - skipping build verification\n", language)
		return nil
	}
	if spec.skip {
		return nil
	}

	res := boundedexec.Command{
		Dir:     projectDir,
		Label:   "Gate 9 build",
		Name:    spec.name,
		Args:    spec.args,
		Timeout: buildTimeoutMinutes * time.Minute,
	}.Run()

	if res.TimedOut {
		return NewValidationError(
			"complete phase",
			fmt.Sprintf("❌ Gate 9 Failed: Build Verification\n\nBuild timeout (%d minutes)", buildTimeoutMinutes),
			"Optimize build performance or increase timeout in V2",
		)
	}
	if res.Err != nil {
		line := spec.commandLine()
		return NewValidationError(
			"complete phase",
			fmt.Sprintf("❌ Gate 9 Failed: Build Verification\n\nBuild command failed: %s\n\nExit code: %v\nOutput:\n%s", line, res.Err, res.Output),
			fmt.Sprintf("Resolution:\nFix build errors before completing phase\n\nRun: %s", line),
		)
	}

	return nil
}

// runTestCommand executes the test command under a real wall-clock bound, with
// test hygiene enforcement.
// Returns ValidationError if tests fail, skip, or time out.
func runTestCommand(projectDir, language string) error {
	spec, supported := testCommandFor(language)
	if !supported {
		fmt.Fprintf(os.Stderr, "⚠️  Unsupported language: %s - skipping test verification\n", language)
		return nil
	}

	res := boundedexec.Command{
		Dir:     projectDir,
		Label:   "Gate 9 test hygiene",
		Name:    spec.name,
		Args:    spec.args,
		Timeout: testTimeoutMinutes * time.Minute,
	}.Run()

	if res.TimedOut {
		return NewValidationError(
			"complete phase",
			fmt.Sprintf("❌ Gate 9 Failed: Test Hygiene Verification\n\nTest timeout (%d minutes)", testTimeoutMinutes),
			"Optimize test performance or increase timeout in V2",
		)
	}
	if res.Err != nil {
		// Test hygiene gate: exit code non-zero = failures OR skips
		line := spec.commandLine()
		return NewValidationError(
			"complete phase",
			fmt.Sprintf("❌ Gate 9 Failed: Test Hygiene Verification\n\nTest command failed: %s\n\nExit code: %v\nOutput:\n%s", line, res.Err, res.Output),
			testHygieneRemediation(line),
		)
	}

	return nil
}

// commandLine renders the command the way an operator would retype it.
func (g gateCommand) commandLine() string {
	return strings.TrimSpace(g.name + " " + strings.Join(g.args, " "))
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

	// Check the SPEC-defined 24-hour cache expiry.
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
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Marshal cache to JSON
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache: %w", err)
	}

	// Write cache file
	if err := os.WriteFile(cachePath, data, 0o600); err != nil {
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
		defer func() { _ = file.Close() }()

		if _, err := io.Copy(hasher, file); err != nil {
			return "", err
		}
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}
