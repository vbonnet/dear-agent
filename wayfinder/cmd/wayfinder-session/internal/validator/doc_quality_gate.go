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

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/telemetry"
)

const (
	minDocQualityScore  = 8.0      // Minimum score required (from W0 decision)
	maxDocFileSizeBytes = 10485760 // 10MB limit for documentation files
)

// DocQualityCache represents the cache entry for a documentation file
type DocQualityCache struct {
	FileHash  string    `json:"file_hash"`
	Score     float64   `json:"score"`
	Timestamp time.Time `json:"timestamp"`
}

// DocQualityCacheFile represents the entire cache file structure
type DocQualityCacheFile map[string]DocQualityCache

// DocumentReviewResult holds review results for a single document
type DocumentReviewResult struct {
	DocumentPath string
	DocumentName string
	SkillUsed    string
	Score        float64
	Issues       []string
	CacheHit     bool
	Timestamp    time.Time
}

// validateDocQuality checks documentation quality for D3/D4/S6 phases.
// Returns ValidationError if documentation scores below 8.0/10 threshold.
// Uses SHA-256 hash-based caching to skip re-validation for unchanged files.
//
// Extended to support D3 phase validation (NEW).
func validateDocQuality(phaseName, projectDir string) error {
	// Route to appropriate validation based on phase
	switch phaseName {
	case "D3", "DESIGN":
		return validateD3Documents(projectDir)
	case "D4", "SPEC":
		return validateSingleDocument(projectDir, phaseName, "SPEC.md", "review-spec")
	case "S6", "PLAN":
		return validateSingleDocument(projectDir, phaseName, "ARCHITECTURE.md", "review-architecture")
	default:
		// Not a phase that requires doc quality validation
		return nil
	}
}

// validateD3Documents validates all D3 architecture documents
// (ARCHITECTURE.md + ADR-*.md files) using appropriate review skills.
//
// Returns nil if all documents pass (score ≥8.0), error otherwise.
func validateD3Documents(projectDir string) error {
	// Check ARCHITECTURE.md exists (required)
	archPath := filepath.Join(projectDir, "ARCHITECTURE.md")
	if _, err := os.Stat(archPath); err != nil {
		if os.IsNotExist(err) {
			return NewValidationError(
				"complete D3",
				"ARCHITECTURE.md does not exist (required)",
				"Create ARCHITECTURE.md before completing D3 phase",
			)
		}
		return NewValidationError(
			"complete D3",
			fmt.Sprintf("failed to check ARCHITECTURE.md: %v", err),
			"Check file permissions and try again",
		)
	}

	// Find all ADR files
	adrPattern := filepath.Join(projectDir, "ADR-*.md")
	adrFiles, err := filepath.Glob(adrPattern)
	if err != nil {
		return NewValidationError(
			"complete D3",
			fmt.Sprintf("failed to search for ADR files: %v", err),
			"Check file permissions and try again",
		)
	}

	// Build list of all documents to review
	var docsToReview []struct {
		path  string
		skill string
	}

	// Add ARCHITECTURE.md
	docsToReview = append(docsToReview, struct {
		path  string
		skill string
	}{archPath, "review-architecture"})

	// Add all ADRs
	for _, adrPath := range adrFiles {
		docsToReview = append(docsToReview, struct {
			path  string
			skill string
		}{adrPath, "review-adr"})
	}

	// Review each document
	var results []DocumentReviewResult
	for _, doc := range docsToReview {
		score, issues, cacheHit, err := reviewDocument(doc.path, doc.skill)
		if err != nil {
			return err // Already wrapped in ValidationError
		}

		results = append(results, DocumentReviewResult{
			DocumentPath: doc.path,
			DocumentName: filepath.Base(doc.path),
			SkillUsed:    doc.skill,
			Score:        score,
			Issues:       issues,
			CacheHit:     cacheHit,
			Timestamp:    time.Now(),
		})
	}

	// Emit telemetry for each reviewed document
	emitDocQualityTelemetry("D3", projectDir, results)

	// Check if ALL documents pass (score ≥8.0)
	var failedDocs []DocumentReviewResult
	for _, result := range results {
		if result.Score < minDocQualityScore {
			failedDocs = append(failedDocs, result)
		}
	}

	// If any failed, return error with details
	if len(failedDocs) > 0 {
		return formatD3ValidationError(results, failedDocs)
	}

	// All passed
	fmt.Fprintf(os.Stderr, "✓ D3 document quality check passed (%d documents reviewed)\n", len(results))
	return nil
}

// validateSingleDocument validates a single document (D4 SPEC.md or S6 ARCHITECTURE.md)
func validateSingleDocument(projectDir, phaseName, docFile, skillName string) error {
	docPath := filepath.Join(projectDir, docFile)

	// Check file exists
	if _, err := os.Stat(docPath); err != nil {
		if os.IsNotExist(err) {
			return NewValidationError(
				"complete "+phaseName,
				fmt.Sprintf("%s does not exist (score 0/10, minimum 8.0 required)", docFile),
				fmt.Sprintf("Create %s with complete documentation before completing %s", docFile, phaseName),
			)
		}
		return NewValidationError(
			"complete "+phaseName,
			fmt.Sprintf("failed to check %s: %v", docFile, err),
			"Check file permissions and try again",
		)
	}

	// Check file size
	if err := validateDocFileSize(docPath, phaseName, docFile); err != nil {
		return err
	}

	// Review document
	score, issues, cacheHit, err := reviewDocument(docPath, skillName)
	if err != nil {
		return err
	}

	// Emit telemetry for this document
	emitDocQualityTelemetry(phaseName, projectDir, []DocumentReviewResult{{
		DocumentPath: docPath,
		DocumentName: docFile,
		SkillUsed:    skillName,
		Score:        score,
		Issues:       issues,
		CacheHit:     cacheHit,
		Timestamp:    time.Now(),
	}})

	// Report result
	cacheMarker := ""
	if cacheHit {
		cacheMarker = " (cached)"
	}

	// Check score against threshold
	if score < minDocQualityScore {
		issueList := ""
		if len(issues) > 0 {
			issueList = "\n\nIssues found:\n"
			for i, issue := range issues {
				issueList += fmt.Sprintf("%d. %s\n", i+1, issue)
			}
		}

		return NewValidationError(
			"complete "+phaseName,
			fmt.Sprintf("%s scored %.1f/10 (minimum 8.0 required)", docFile, score),
			fmt.Sprintf("Fix documentation issues and re-run: wayfinder session complete-phase %s%s", phaseName, issueList),
		)
	}

	fmt.Fprintf(os.Stderr, "✓ %s quality check passed (score: %.1f/10%s)\n", docFile, score, cacheMarker)
	return nil
}

// reviewDocument reviews a document using the specified skill.
// Returns (score, issues, cacheHit, error).
func reviewDocument(docPath, skillName string) (float64, []string, bool, error) {
	// Calculate file hash for cache lookup
	fileHash, err := calculateFileHash(docPath)
	if err != nil {
		return 0, nil, false, NewValidationError(
			"complete phase",
			fmt.Sprintf("failed to calculate file hash: %v", err),
			"Check file permissions and try again",
		)
	}

	// Check cache
	projectDir := filepath.Dir(docPath)
	docFile := filepath.Base(docPath)
	cachedScore, cacheHit := checkCache(projectDir, docFile, fileHash)
	if cacheHit && cachedScore >= minDocQualityScore {
		return cachedScore, nil, true, nil
	}

	// Run review skill
	score, issues, err := runReviewSkill(skillName, docPath)
	if err != nil {
		return 0, nil, false, err
	}

	// Update cache
	if err := updateCache(projectDir, docFile, fileHash, score); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Failed to update quality cache: %v\n", err)
	}

	return score, issues, false, nil
}

// runReviewSkill executes a review skill and returns the score + issues.
// Supports: review-spec, review-adr, review-architecture
func runReviewSkill(skillName string, docPath string) (float64, []string, error) {
	// Find review skill script
	scriptPath, err := findReviewSkillScript(skillName)
	if err != nil {
		return 0, nil, NewValidationError(
			"complete phase",
			fmt.Sprintf("review skill not found: %s", skillName),
			fmt.Sprintf("Ensure engram is installed with review skills.\n\nError: %v", err),
		)
	}

	// Create temporary file for JSON output
	tmpFile, err := os.CreateTemp("", "doc-quality-*.json")
	if err != nil {
		return 0, nil, NewValidationError(
			"complete phase",
			"failed to create temp file for review output",
			fmt.Sprintf("Error: %v", err),
		)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Execute Python skill with JSON output
	args := []string{scriptPath, docPath, "--output-json", tmpPath}

	// Skip LLM validation if ANTHROPIC_API_KEY is not set
	// This ensures tests run in CI/test environments without requiring API access
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		args = append(args, "--skip-llm")
	}

	cmd := exec.Command("python3", args...)

	// Capture stderr for error messages
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, nil, NewValidationError(
			"complete phase",
			fmt.Sprintf("%s skill execution failed", skillName),
			fmt.Sprintf("Ensure Python 3.9+ is installed and ANTHROPIC_API_KEY is set.\n\nError: %v\n\nOutput: %s", err, string(output)),
		)
	}

	// Read JSON output
	jsonData, err := os.ReadFile(tmpPath)
	if err != nil {
		return 0, nil, NewValidationError(
			"complete phase",
			"failed to read review output",
			fmt.Sprintf("Error: %v", err),
		)
	}

	// Parse JSON output
	type ValidationResult struct {
		OverallScore float64 `json:"overall_score"`
		Decision     string  `json:"decision"`
	}

	var result ValidationResult
	if err := json.Unmarshal(jsonData, &result); err != nil {
		return 0, nil, NewValidationError(
			"complete phase",
			"failed to parse review output",
			fmt.Sprintf("Parse error: %v\n\nJSON: %s", err, string(jsonData)),
		)
	}

	// Extract issues from decision field
	var issues []string
	if result.Decision != "PASS" && result.Decision != "WARN" {
		issues = append(issues, result.Decision)
	}

	return result.OverallScore, issues, nil
}

// findReviewSkillScript locates the review skill script by searching common locations.
func findReviewSkillScript(skillName string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	// Map skill name to script filename
	scriptMap := map[string]string{
		"review-spec":         "review_spec.py",
		"review-adr":          "review_adr.py",
		"review-architecture": "review_architecture.py",
	}

	scriptFile, ok := scriptMap[skillName]
	if !ok {
		return "", fmt.Errorf("unknown skill: %s", skillName)
	}

	// Search common skill locations
	skillPaths := []string{
		filepath.Join(".", "skills", skillName, scriptFile),
		filepath.Join(homeDir, "src", "ws", "oss", "repos", "engram", "skills", skillName, scriptFile),
		filepath.Join(homeDir, "src", "engram", "skills", skillName, scriptFile),
		filepath.Join(homeDir, "engram", "skills", skillName, scriptFile),
		filepath.Join(homeDir, ".local/share/engram/skills", skillName, scriptFile),
		filepath.Join("/usr/local/share/engram/skills", skillName, scriptFile),
		filepath.Join("/opt/engram/skills", skillName, scriptFile),
	}

	for _, path := range skillPaths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("%s skill not found in common locations", skillName)
}

// formatD3ValidationError creates a detailed error message for D3 validation failures.
func formatD3ValidationError(allResults, failedDocs []DocumentReviewResult) error {
	var msg strings.Builder
	msg.WriteString("❌ D3 document quality gate failed\n\n")
	msg.WriteString("Documents reviewed:\n")

	// Show all documents with pass/fail status
	for _, result := range allResults {
		status := "✅ PASSED"
		if result.Score < minDocQualityScore {
			status = "⚠️  FAILED"
		}
		msg.WriteString(fmt.Sprintf("- %s: %.1f/10 %s\n", result.DocumentName, result.Score, status))
	}

	msg.WriteString(fmt.Sprintf("\nMinimum score required: %.1f/10\n\n", minDocQualityScore))
	msg.WriteString("Fix failing documents and re-run:\n")
	msg.WriteString("  wayfinder session complete-phase D3\n\n")

	// Show how to review manually
	if len(failedDocs) > 0 {
		msg.WriteString("Or review manually to see issues:\n")
		firstFailed := failedDocs[0]
		skillPath := filepath.Join("~/src/ws/oss/repos/engram/skills", firstFailed.SkillUsed, strings.ReplaceAll(firstFailed.SkillUsed, "-", "_")+".py")
		msg.WriteString(fmt.Sprintf("  python3 %s %s --output-json /tmp/review.json\n", skillPath, firstFailed.DocumentName))
		msg.WriteString("  cat /tmp/review.json\n\n")
	}

	return NewValidationError(
		"complete D3",
		msg.String(),
		"",
	)
}

// validateDocFileSize checks if documentation file is within size limits.
func validateDocFileSize(docPath, phaseName, docFile string) error {
	info, err := os.Stat(docPath)
	if err != nil {
		return NewValidationError(
			"complete "+phaseName,
			fmt.Sprintf("failed to check %s size: %v", docFile, err),
			"Check file permissions and try again",
		)
	}

	if info.Size() > maxDocFileSizeBytes {
		sizeMB := float64(info.Size()) / 1048576.0
		return NewValidationError(
			"complete "+phaseName,
			fmt.Sprintf("%s file too large (%.1fMB > 10MB limit)", docFile, sizeMB),
			fmt.Sprintf("Reduce %s file size by splitting content or removing large examples", docFile),
		)
	}

	return nil
}

// calculateFileHash computes SHA-256 hash of file content.
func calculateFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// checkCache checks if file hash exists in cache with passing score.
func checkCache(projectDir, docFile, fileHash string) (float64, bool) {
	cachePath := filepath.Join(projectDir, ".wayfinder-cache", "doc-quality-scores.json")

	data, err := os.ReadFile(cachePath)
	if err != nil {
		return 0, false
	}

	var cache DocQualityCacheFile
	if err := json.Unmarshal(data, &cache); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Corrupted quality cache, ignoring: %v\n", err)
		return 0, false
	}

	entry, exists := cache[docFile]
	if !exists || entry.FileHash != fileHash {
		return 0, false
	}

	return entry.Score, true
}

// updateCache updates cache with new file hash and score.
func updateCache(projectDir, docFile, fileHash string, score float64) error {
	cacheDir := filepath.Join(projectDir, ".wayfinder-cache")
	cachePath := filepath.Join(cacheDir, "doc-quality-scores.json")

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	var cache DocQualityCacheFile
	data, err := os.ReadFile(cachePath)
	if err == nil {
		if err := json.Unmarshal(data, &cache); err != nil {
			cache = make(DocQualityCacheFile)
		}
	} else {
		cache = make(DocQualityCacheFile)
	}

	cache[docFile] = DocQualityCache{
		FileHash:  fileHash,
		Score:     score,
		Timestamp: time.Now(),
	}

	newData, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache: %w", err)
	}

	if err := os.WriteFile(cachePath, newData, 0644); err != nil {
		return fmt.Errorf("failed to write cache: %w", err)
	}

	return nil
}

// emitDocQualityTelemetry emits telemetry events for document quality assessments.
// Failures are logged to stderr but do not block validation.
func emitDocQualityTelemetry(phase, projectDir string, results []DocumentReviewResult) {
	telemetryPath, err := telemetry.DefaultTelemetryPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: skipping telemetry emission: %v\n", err)
		return
	}

	projectName := filepath.Base(projectDir)

	for _, result := range results {
		var contextSources []string
		contextSources = append(contextSources, result.DocumentName)

		event := telemetry.QualityAssessedEvent{
			Phase:          phase,
			Score:          result.Score,
			ContextSources: contextSources,
			JudgeModel:     result.SkillUsed,
			Timestamp:      result.Timestamp,
			ProjectName:    projectName,
		}

		if err := telemetry.EmitQualityEvent(context.Background(), event, telemetryPath); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to emit quality telemetry: %v\n", err)
		}
	}
}
