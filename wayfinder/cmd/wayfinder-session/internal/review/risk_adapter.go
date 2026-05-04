package review

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// Risk patterns are compiled once at package init so they can be reused
// across deliverables without paying the regexp.Compile cost per call.
//
// Pattern 4 ("yaml.load(?!_safe)") used a Perl-only negative lookahead
// (`(?!`) that Go's RE2 engine rejects. Replaced with a two-step check:
// match `yaml.load` and reject the safe form separately.
var (
	rxSQLInjection         = regexp.MustCompile(`(?i)execute\(.*\+.*\)`)
	rxEvalExec             = regexp.MustCompile(`\b(eval|exec)\s*\(`)
	rxHardcodedSecret      = regexp.MustCompile(`(?i)(password|secret|api[_-]?key)\s*=\s*["']`)
	rxUnsafePickle         = regexp.MustCompile(`pickle\.loads`)
	rxYAMLLoad             = regexp.MustCompile(`yaml\.load`)
	rxYAMLLoadSafe         = regexp.MustCompile(`yaml\.load_safe|yaml\.safe_load`)
	rxShellInjection       = regexp.MustCompile(`os\.system\(|subprocess\.call\(.*shell=True`)
	rxPanic                = regexp.MustCompile(`panic\(`)
	rxIgnoredError         = regexp.MustCompile(`_\s*=.*error`)
)

// RiskLevel represents the risk level of a task
type RiskLevel int

// Task RiskLevel values, ordered from least to most risk.
const (
	RiskLevelXS RiskLevel = iota // Extra Small (1-50 LOC)
	RiskLevelS                   // Small (51-200 LOC)
	RiskLevelM                   // Medium (201-500 LOC)
	RiskLevelL                   // Large (501-1000 LOC)
	RiskLevelXL                  // Extra Large (1001+ LOC)
)

// String returns the string representation of a risk level
func (r RiskLevel) String() string {
	switch r {
	case RiskLevelXS:
		return "XS"
	case RiskLevelS:
		return "S"
	case RiskLevelM:
		return "M"
	case RiskLevelL:
		return "L"
	case RiskLevelXL:
		return "XL"
	default:
		return "UNKNOWN"
	}
}

// RiskAdapter calculates risk levels for tasks
type RiskAdapter struct {
	config *ReviewConfig
}

// NewRiskAdapter creates a new risk adapter
func NewRiskAdapter(config *ReviewConfig) *RiskAdapter {
	return &RiskAdapter{
		config: config,
	}
}

// CalculateRiskLevel determines the risk level for a task
func (r *RiskAdapter) CalculateRiskLevel(task *status.Task, projectDir string) RiskLevel {
	// Factor 1: Lines of Code
	totalLOC := r.calculateTotalLOC(task, projectDir)

	// Factor 2: File criticality
	criticalityScore := r.calculateFileCriticality(task.Deliverables)

	// Factor 3: Change type
	changeTypeScore := r.analyzeChangeType(task)

	// Factor 4: Existing coverage (simplified - would need actual coverage data)
	coverageScore := r.calculateCoverageRisk(task)

	// Factor 5: Pattern detection
	patternScore := r.detectRiskyPatterns(task, projectDir)

	// Weighted composite score
	compositeScore := float64(totalLOC)*0.3 +
		criticalityScore*0.3 +
		changeTypeScore*0.2 +
		coverageScore*0.1 +
		patternScore*0.1

	// Map composite score to base risk level
	baseRisk := r.scoreToRiskLevel(compositeScore)

	// Apply escalation rules
	escalatedRisk := r.applyEscalationRules(baseRisk, task, projectDir)

	return escalatedRisk
}

// calculateTotalLOC counts total lines of code in task deliverables
func (r *RiskAdapter) calculateTotalLOC(task *status.Task, projectDir string) int {
	totalLOC := 0

	for _, deliverable := range task.Deliverables {
		fullPath := filepath.Join(projectDir, deliverable)
		loc := countLOC(fullPath)
		totalLOC += loc
	}

	return totalLOC
}

// countLOC counts lines of code in a file
func countLOC(filePath string) int {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return 0
	}

	lines := strings.Split(string(data), "\n")
	loc := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Skip empty lines and comments
		if trimmed != "" && !strings.HasPrefix(trimmed, "//") && !strings.HasPrefix(trimmed, "#") {
			loc++
		}
	}

	return loc
}

// calculateFileCriticality calculates criticality score based on file paths
func (r *RiskAdapter) calculateFileCriticality(files []string) float64 {
	if len(files) == 0 {
		return 0.0
	}

	score := 0.0

	for _, file := range files {
		fileLower := strings.ToLower(file)

		// Critical file patterns (high risk)
		switch {
		case r.matchesCriticalPattern(fileLower):
			score += 500
		case r.matchesImportantPattern(fileLower):
			// Important file patterns (medium risk)
			score += 200
		default:
			// Standard files (low risk)
			score += 50
		}
	}

	return score / float64(len(files))
}

// matchesCriticalPattern checks if file matches critical patterns
func (r *RiskAdapter) matchesCriticalPattern(filePath string) bool {
	criticalPatterns := []string{
		"auth", "authz", "security", "crypto",
		"payment", "billing", "database/migration",
		"infrastructure", "deploy", ".env",
		"secrets", "admin",
	}

	for _, pattern := range criticalPatterns {
		if strings.Contains(filePath, pattern) {
			return true
		}
	}

	return false
}

// matchesImportantPattern checks if file matches important patterns
func (r *RiskAdapter) matchesImportantPattern(filePath string) bool {
	importantPatterns := []string{
		"api/", "models/", "schema",
		"config", "middleware", "plugin",
		"integration",
	}

	for _, pattern := range importantPatterns {
		if strings.Contains(filePath, pattern) {
			return true
		}
	}

	return false
}

// analyzeChangeType determines risk based on change description
func (r *RiskAdapter) analyzeChangeType(task *status.Task) float64 {
	descLower := strings.ToLower(task.Description)

	// High-risk change types
	highRiskKeywords := []string{
		"new feature", "add feature", "implement",
		"refactor", "rewrite", "redesign",
	}

	for _, keyword := range highRiskKeywords {
		if strings.Contains(descLower, keyword) {
			return 300
		}
	}

	// Medium-risk change types
	mediumRiskKeywords := []string{
		"enhance", "improve", "update", "modify",
	}

	for _, keyword := range mediumRiskKeywords {
		if strings.Contains(descLower, keyword) {
			return 150
		}
	}

	// Low-risk change types
	lowRiskKeywords := []string{
		"fix bug", "fix", "bugfix", "patch",
		"typo", "documentation", "comment",
	}

	for _, keyword := range lowRiskKeywords {
		if strings.Contains(descLower, keyword) {
			return 50
		}
	}

	// Unknown change type
	return 100
}

// calculateCoverageRisk calculates risk based on existing test coverage
func (r *RiskAdapter) calculateCoverageRisk(task *status.Task) float64 {
	// This is a simplified implementation
	// Real implementation would integrate with coverage tools

	// If task has tests_status, use it as a proxy
	if task.TestsStatus != nil {
		switch *task.TestsStatus {
		case "passed":
			return 0 // Good coverage
		case "pending":
			return 200 // No coverage
		}
	}

	// Default to medium risk
	return 100
}

// detectRiskyPatterns scans files for risky code patterns
func (r *RiskAdapter) detectRiskyPatterns(task *status.Task, projectDir string) float64 {
	riskScore := 0.0
	fileCount := 0

	for _, deliverable := range task.Deliverables {
		fullPath := filepath.Join(projectDir, deliverable)
		data, err := os.ReadFile(fullPath)
		if err != nil {
			continue
		}

		content := string(data)
		fileCount++

		if rxSQLInjection.MatchString(content) {
			riskScore += 200
		}
		if rxEvalExec.MatchString(content) {
			riskScore += 300
		}
		if rxHardcodedSecret.MatchString(content) {
			riskScore += 400
		}
		if rxUnsafePickle.MatchString(content) ||
			(rxYAMLLoad.MatchString(content) && !rxYAMLLoadSafe.MatchString(content)) {
			riskScore += 200
		}
		if rxShellInjection.MatchString(content) {
			riskScore += 150
		}
		if rxPanic.MatchString(content) {
			riskScore += 100
		}
		if rxIgnoredError.MatchString(content) {
			riskScore += 50
		}
	}

	if fileCount == 0 {
		return 0
	}

	return riskScore / float64(fileCount)
}

// scoreToRiskLevel maps composite score to risk level
func (r *RiskAdapter) scoreToRiskLevel(score float64) RiskLevel {
	switch {
	case score <= 50:
		return RiskLevelXS
	case score <= 200:
		return RiskLevelS
	case score <= 500:
		return RiskLevelM
	case score <= 1000:
		return RiskLevelL
	default:
		return RiskLevelXL
	}
}

// applyEscalationRules escalates risk based on specific conditions
func (r *RiskAdapter) applyEscalationRules(baseRisk RiskLevel, task *status.Task, projectDir string) RiskLevel {
	escalated := baseRisk

	// Rule 1: Critical files always L or XL
	for _, deliverable := range task.Deliverables {
		if r.matchesCriticalPattern(strings.ToLower(deliverable)) {
			if escalated < RiskLevelL {
				escalated = RiskLevelL
			}
		}
	}

	// Rule 2: Multiple critical files = XL
	criticalFileCount := 0
	for _, deliverable := range task.Deliverables {
		if r.matchesCriticalPattern(strings.ToLower(deliverable)) {
			criticalFileCount++
		}
	}

	if criticalFileCount >= 3 {
		escalated = RiskLevelXL
	}

	// Rule 3: High complexity = upgrade one level
	maxComplexity := r.calculateMaxComplexity(task, projectDir)
	if maxComplexity > 15 {
		escalated = r.upgradeRiskLevel(escalated)
	}

	// Rule 4: Cross-cutting changes = upgrade one level
	if r.spansMultipleDomains(task.Deliverables) {
		escalated = r.upgradeRiskLevel(escalated)
	}

	return escalated
}

// calculateMaxComplexity estimates maximum cyclomatic complexity
func (r *RiskAdapter) calculateMaxComplexity(task *status.Task, projectDir string) int {
	maxComplexity := 0

	for _, deliverable := range task.Deliverables {
		fullPath := filepath.Join(projectDir, deliverable)
		complexity := estimateComplexity(fullPath)
		if complexity > maxComplexity {
			maxComplexity = complexity
		}
	}

	return maxComplexity
}

// estimateComplexity estimates cyclomatic complexity of a file
func estimateComplexity(filePath string) int {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return 0
	}

	content := string(data)

	// Rough estimate: count branching keywords
	complexity := 1 // Base complexity

	keywords := []string{
		"if ", "else if", "for ", "while ", "switch ", "case ",
		"&&", "||", "?", "catch ", "rescue ",
	}

	for _, keyword := range keywords {
		complexity += strings.Count(content, keyword)
	}

	return complexity
}

// spansMultipleDomains checks if files span multiple architectural domains
func (r *RiskAdapter) spansMultipleDomains(files []string) bool {
	domains := make(map[string]bool)

	for _, file := range files {
		// Extract domain from path (first directory)
		parts := strings.Split(file, "/")
		if len(parts) > 1 {
			domain := parts[0]
			domains[domain] = true
		}
	}

	// If files span 3+ domains, consider it cross-cutting
	return len(domains) >= 3
}

// upgradeRiskLevel increases risk level by one step
func (r *RiskAdapter) upgradeRiskLevel(risk RiskLevel) RiskLevel {
	switch risk {
	case RiskLevelXS:
		return RiskLevelS
	case RiskLevelS:
		return RiskLevelM
	case RiskLevelM:
		return RiskLevelL
	case RiskLevelL:
		return RiskLevelXL
	case RiskLevelXL:
		return RiskLevelXL // Already max
	default:
		return risk
	}
}

// GetRiskLevelThresholds returns the LOC thresholds for each risk level
func (r *RiskAdapter) GetRiskLevelThresholds() map[RiskLevel]int {
	return map[RiskLevel]int{
		RiskLevelXS: r.config.XSMaxLOC,
		RiskLevelS:  r.config.SMaxLOC,
		RiskLevelM:  r.config.MMaxLOC,
		RiskLevelL:  r.config.LMaxLOC,
		RiskLevelXL: r.config.XLMinLOC,
	}
}

// GetReviewStrategy returns the review strategy for a risk level
func (r *RiskAdapter) GetReviewStrategy(riskLevel RiskLevel) string {
	if riskLevel >= r.config.PerTaskMinRisk {
		return "per_task"
	}
	return "batch"
}

// ClassifyRisk calculates the risk level for a task and returns the corresponding
// ProfileConfig. This is the primary entry point for other layers (e.g. Swarm) that
// need both the risk classification and the process-depth profile in a single call.
func (r *RiskAdapter) ClassifyRisk(task *status.Task, projectDir string) (RiskLevel, ProfileConfig) {
	risk := r.CalculateRiskLevel(task, projectDir)
	return risk, GetProfileConfigForRisk(risk)
}
