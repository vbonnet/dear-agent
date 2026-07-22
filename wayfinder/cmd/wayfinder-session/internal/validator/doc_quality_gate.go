package validator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/telemetry"
	"gopkg.in/yaml.v3"
)

const (
	minDocQualityScore  = 8.0      // Minimum score required by the canonical quality gate
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

// validateDocQuality checks documentation quality for DESIGN/SPEC/PLAN phases.
// Returns ValidationError if documentation scores below 8.0/10 threshold.
// Uses SHA-256 hash-based caching to skip re-validation for unchanged files.
//
// Extended to support DESIGN phase validation (NEW).
func validateDocQuality(phaseName, projectDir string) error {
	// Route to appropriate validation based on phase
	switch phaseName {
	case "DESIGN":
		return validateDesignDocuments(projectDir)
	case "SPEC":
		// The SPEC phase deliverable is gated by the deterministic EARS linter (replaces the
		// former Python LLM "review-spec" rubric). See spec_ears_gate.go.
		return validateSpecEARS(projectDir, phaseName)
	case "PLAN":
		return validateSingleDocument(projectDir, phaseName, "PLAN-design.md", "review-architecture")
	default:
		// Not a phase that requires doc quality validation
		return nil
	}
}

// validateDesignDocuments validates all DESIGN architecture documents
// (ARCHITECTURE.md + ADR-*.md files) using appropriate review skills.
//
// Returns nil if all documents pass (score ≥8.0), error otherwise.
func validateDesignDocuments(projectDir string) error {
	// Check ARCHITECTURE.md exists (required)
	archPath := filepath.Join(projectDir, "ARCHITECTURE.md")
	if _, err := os.Stat(archPath); err != nil {
		if os.IsNotExist(err) {
			return NewValidationError(
				"complete DESIGN",
				"ARCHITECTURE.md does not exist (required)",
				"Create ARCHITECTURE.md before completing DESIGN phase",
			)
		}
		return NewValidationError(
			"complete DESIGN",
			fmt.Sprintf("failed to check ARCHITECTURE.md: %v", err),
			"Check file permissions and try again",
		)
	}

	// Find all ADR files
	adrPattern := filepath.Join(projectDir, "ADR-*.md")
	adrFiles, err := filepath.Glob(adrPattern)
	if err != nil {
		return NewValidationError(
			"complete DESIGN",
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
	emitDocQualityTelemetry("DESIGN", projectDir, results)

	// Check if ALL documents pass (score ≥8.0)
	var failedDocs []DocumentReviewResult
	for _, result := range results {
		if result.Score < minDocQualityScore {
			failedDocs = append(failedDocs, result)
		}
	}

	// If any failed, return error with details
	if len(failedDocs) > 0 {
		return formatDesignValidationError(results, failedDocs)
	}

	// All passed
	fmt.Fprintf(os.Stderr, "✓ DESIGN document quality check passed (%d documents reviewed)\n", len(results))
	return nil
}

// validateSingleDocument validates a single canonical phase document.
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

	projectDir := filepath.Dir(docPath)
	docFile := filepath.Base(docPath)

	// Do not consult historical scores here. Earlier builds cached a structural
	// preflight as a perfect review, so those entries do not carry enough
	// provenance to satisfy the provider-backed quality gate.
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

// runReviewSkill applies the deterministic semantic rubric bundled in the
// Wayfinder binary. The rubric is deliberately local so the phase gate remains
// reachable when no external provider is configured.
func runReviewSkill(skillName string, docPath string) (float64, []string, error) {
	data, err := os.ReadFile(docPath)
	if err != nil {
		return 0, nil, NewValidationError(
			"complete phase",
			fmt.Sprintf("failed to read %s", filepath.Base(docPath)),
			fmt.Sprintf("Check file permissions and try again.\n\nError: %v", err),
		)
	}
	issues, err := builtinReviewIssues(skillName, string(data))
	if err != nil {
		return 0, nil, err
	}
	if len(issues) > 0 {
		return 0, issues, nil
	}
	return 10, nil, nil
}

func builtinReviewIssues(skillName, content string) ([]string, error) {
	if skillName != "review-adr" && skillName != "review-architecture" {
		return nil, fmt.Errorf("unknown built-in document review %q", skillName)
	}
	body, err := markdownBodyForReview(content)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(body)
	issues := commonDocumentReviewIssues(trimmed)
	issues = append(issues, skillDocumentReviewIssues(skillName, trimmed, levelTwoHeadings(trimmed))...)
	return issues, nil
}

// markdownBodyForReview returns the Markdown body after one canonical leading
// YAML frontmatter block. Methodology validation requires phase deliverables
// such as PLAN-design.md to carry that block, while document quality applies to
// the authored Markdown beneath it. A leading but unclosed block is malformed
// input and must not be reviewed as ordinary prose.
func markdownBodyForReview(content string) (string, error) {
	yamlContent, body, found, err := splitLeadingFrontmatter(content)
	if err != nil {
		return "", fmt.Errorf("document has unclosed YAML frontmatter: %w", err)
	}
	if !found {
		return content, nil
	}

	var metadata map[string]any
	if err := yaml.Unmarshal([]byte(yamlContent), &metadata); err != nil {
		return "", fmt.Errorf("document has invalid YAML frontmatter: %w", err)
	}
	return body, nil
}

func commonDocumentReviewIssues(trimmed string) []string {
	var issues []string
	if len(trimmed) < 100 {
		issues = append(issues, "document must contain at least 100 bytes of substantive content")
	}
	if !strings.HasPrefix(trimmed, "# ") {
		issues = append(issues, "document must begin with a level-one heading")
	}
	if strings.Count("\n"+trimmed, "\n## ") < 2 {
		issues = append(issues, "document must contain at least two level-two sections")
	}
	words, uniqueWords, maxFrequency := documentWordStats(trimmed)
	if words < 20 || uniqueWords < 12 {
		issues = append(issues, "document must contain substantive prose with at least 20 words and 12 distinct words")
	}
	if words > 0 && maxFrequency*5 > words*2 {
		issues = append(issues, "document repeats one word too heavily to be substantive")
	}
	return issues
}

func skillDocumentReviewIssues(skillName, trimmed string, headings []string) []string {
	var issues []string
	if skillName == "review-adr" && !strings.HasPrefix(trimmed, "# ADR-") {
		issues = append(issues, "ADR must begin with a canonical # ADR- heading")
	}
	if skillName == "review-adr" && !hasADRStatusLine(trimmed) {
		issues = append(issues, "ADR must contain a Status line")
	}
	if !containsHeading(headings, "context") {
		issues = append(issues, "document must contain a Context section")
	}
	if skillName == "review-adr" && !containsHeading(headings, "decision") {
		issues = append(issues, "ADR must contain a Decision section")
	}
	if skillName == "review-architecture" &&
		!containsAnyHeading(headings, "design", "architecture", "decision") {
		issues = append(issues, "architecture document must contain a Design, Architecture, or Decision section")
	}
	return issues
}

func documentWordStats(content string) (total, unique, maxFrequency int) {
	counts := make(map[string]int)
	for _, word := range strings.FieldsFunc(strings.ToLower(content), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if len([]rune(word)) < 3 {
			continue
		}
		total++
		counts[word]++
		maxFrequency = max(maxFrequency, counts[word])
	}
	return total, len(counts), maxFrequency
}

func levelTwoHeadings(content string) []string {
	var headings []string
	for line := range strings.SplitSeq(content, "\n") {
		line = strings.TrimSpace(line)
		if heading, found := strings.CutPrefix(line, "## "); found {
			headings = append(headings, strings.ToLower(strings.TrimSpace(heading)))
		}
	}
	return headings
}

func containsHeading(headings []string, required string) bool {
	for _, heading := range headings {
		if strings.Contains(heading, required) {
			return true
		}
	}
	return false
}

func containsAnyHeading(headings []string, required ...string) bool {
	for _, candidate := range required {
		if containsHeading(headings, candidate) {
			return true
		}
	}
	return false
}

func hasADRStatusLine(content string) bool {
	for line := range strings.SplitSeq(content, "\n") {
		normalized := strings.TrimSpace(strings.ReplaceAll(line, "**", ""))
		value, found := strings.CutPrefix(normalized, "Status:")
		if found && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

// formatDesignValidationError creates a detailed error message for DESIGN validation failures.
func formatDesignValidationError(allResults, failedDocs []DocumentReviewResult) error {
	var msg strings.Builder
	msg.WriteString("❌ DESIGN document quality gate failed\n\n")
	msg.WriteString("Documents reviewed:\n")

	// Show all documents with pass/fail status
	for _, result := range allResults {
		status := "✅ PASSED"
		if result.Score < minDocQualityScore {
			status = "⚠️  FAILED"
		}
		fmt.Fprintf(&msg, "- %s: %.1f/10 %s\n", result.DocumentName, result.Score, status)
	}

	fmt.Fprintf(&msg, "\nMinimum score required: %.1f/10\n\n", minDocQualityScore)
	msg.WriteString("Fix failing documents and re-run:\n")
	msg.WriteString("  wayfinder session complete-phase DESIGN\n\n")

	for _, failed := range failedDocs {
		if len(failed.Issues) > 0 {
			fmt.Fprintf(&msg, "%s issues: %s\n", failed.DocumentName, strings.Join(failed.Issues, "; "))
		}
	}

	return NewValidationError(
		"complete DESIGN",
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
	defer func() { _ = file.Close() }()

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

	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
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

	if err := os.WriteFile(cachePath, newData, 0o600); err != nil {
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
