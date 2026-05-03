package hippocampus

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// CriticDecision records a critic's evaluation of worker output.
type CriticDecision struct {
	Timestamp  time.Time     `json:"timestamp"`
	WorkerTask string        `json:"worker_task"`
	Approved   bool          `json:"approved"`
	Issues     []CriticIssue `json:"issues,omitempty"`
	Reasoning  string        `json:"reasoning"`
}

// CriticIssue describes a specific problem found by the critic.
type CriticIssue struct {
	Category string `json:"category"` // "sql_injection", "path_traversal", "resource_leak", "security", "correctness"
	Severity string `json:"severity"` // "error", "warning", "info"
	Detail   string `json:"detail"`
	Line     string `json:"line,omitempty"` // the problematic line/snippet
}

// ToolRestrictions defines what tools are allowed or denied for an agent role.
type ToolRestrictions struct {
	AllowBash          bool `json:"allow_bash"`
	AllowFileWrite     bool `json:"allow_file_write"`
	AllowExternalCalls bool `json:"allow_external_calls"`
	AllowFileRead      bool `json:"allow_file_read"`
}

// WorkerRole returns tool restrictions for the worker agent (full access).
func WorkerRole() ToolRestrictions {
	return ToolRestrictions{
		AllowBash:          true,
		AllowFileWrite:     true,
		AllowExternalCalls: true,
		AllowFileRead:      true,
	}
}

// CriticRole returns tool restrictions for the critic agent (reason-only, no execution).
func CriticRole() ToolRestrictions {
	return ToolRestrictions{
		AllowBash:          false,
		AllowFileWrite:     false,
		AllowExternalCalls: false,
		AllowFileRead:      true,
	}
}

// IsAllowed checks whether a given tool name is permitted under these restrictions.
func (tr ToolRestrictions) IsAllowed(toolName string) bool {
	switch toolName {
	case "bash", "Bash":
		return tr.AllowBash
	case "write", "Write", "edit", "Edit", "NotebookEdit":
		return tr.AllowFileWrite
	case "WebFetch", "WebSearch":
		return tr.AllowExternalCalls
	case "read", "Read", "Glob", "Grep":
		return tr.AllowFileRead
	default:
		// Unknown tools are denied for critic, allowed for worker
		return tr.AllowBash // proxy: if bash is allowed, it's a worker
	}
}

// Dyad pairs a worker and critic for a task execution.
type Dyad struct {
	Worker ToolRestrictions
	Critic ToolRestrictions
	LogDir string // directory for critic decision logs
}

// NewDyad creates a worker-critic dyad with default roles.
func NewDyad(logDir string) *Dyad {
	if logDir == "" {
		home, _ := os.UserHomeDir()
		logDir = filepath.Join(home, ".engram", "critic")
	}
	return &Dyad{
		Worker: WorkerRole(),
		Critic: CriticRole(),
		LogDir: logDir,
	}
}

// CriticCheck performs a static analysis of worker output to detect common issues.
// This is the built-in critic that runs without LLM calls.
func CriticCheck(workerOutput string) CriticDecision {
	decision := CriticDecision{
		Timestamp:  time.Now(),
		WorkerTask: truncate(workerOutput, 200),
	}

	var issues []CriticIssue

	// Check for SQL injection patterns
	issues = append(issues, checkSQLInjection(workerOutput)...)

	// Check for path traversal
	issues = append(issues, checkPathTraversal(workerOutput)...)

	// Check for resource leaks
	issues = append(issues, checkResourceLeaks(workerOutput)...)

	decision.Issues = issues
	decision.Approved = len(issues) == 0

	if decision.Approved {
		decision.Reasoning = "No safety or correctness issues detected"
	} else {
		decision.Reasoning = fmt.Sprintf("Found %d issue(s) requiring attention", len(issues))
	}

	return decision
}

// LogDecision appends a critic decision to the decisions log file.
func (d *Dyad) LogDecision(decision CriticDecision) error {
	logPath := filepath.Join(d.LogDir, "decisions.jsonl")

	if err := os.MkdirAll(d.LogDir, 0o700); err != nil {
		return fmt.Errorf("create critic log dir: %w", err)
	}

	data, err := json.Marshal(decision)
	if err != nil {
		return fmt.Errorf("marshal decision: %w", err)
	}

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open decisions log: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write decision: %w", err)
	}

	return nil
}

// --- Built-in critic checks ---

var sqlInjectionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)fmt\.Sprintf\([^)]*SELECT\b`),
	regexp.MustCompile(`(?i)fmt\.Sprintf\([^)]*INSERT\b`),
	regexp.MustCompile(`(?i)fmt\.Sprintf\([^)]*UPDATE\b`),
	regexp.MustCompile(`(?i)fmt\.Sprintf\([^)]*DELETE\b`),
	regexp.MustCompile(`(?i)fmt\.Sprintf\([^)]*DROP\b`),
	regexp.MustCompile(`(?i)"SELECT\s[^"]*"\s*\+`),
	regexp.MustCompile(`(?i)"INSERT\s[^"]*"\s*\+`),
}

func checkSQLInjection(output string) []CriticIssue {
	var issues []CriticIssue
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		for _, pat := range sqlInjectionPatterns {
			if pat.MatchString(line) {
				issues = append(issues, CriticIssue{
					Category: "sql_injection",
					Severity: "error",
					Detail:   "Potential SQL injection: use parameterized queries instead of string formatting",
					Line:     strings.TrimSpace(line),
				})
				break // one issue per line
			}
		}
	}
	return issues
}

var pathTraversalPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\.\.(/|\\)`),
	regexp.MustCompile(`(?i)filepath\.Join\([^)]*\.\./`),
}

func checkPathTraversal(output string) []CriticIssue {
	var issues []CriticIssue
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		for _, pat := range pathTraversalPatterns {
			if pat.MatchString(line) {
				issues = append(issues, CriticIssue{
					Category: "path_traversal",
					Severity: "error",
					Detail:   "Potential path traversal: validate paths and reject ../ components",
					Line:     strings.TrimSpace(line),
				})
				break
			}
		}
	}
	return issues
}

var resourceLeakPatterns = []*regexp.Regexp{
	regexp.MustCompile(`os\.Open\(`),
	regexp.MustCompile(`os\.Create\(`),
	regexp.MustCompile(`os\.OpenFile\(`),
	regexp.MustCompile(`net\.Dial\(`),
	regexp.MustCompile(`http\.Get\(`),
}

var deferClosePattern = regexp.MustCompile(`defer\s+\w*\.?Close\(\)`)

func checkResourceLeaks(output string) []CriticIssue {
	var issues []CriticIssue
	lines := strings.Split(output, "\n")

	for i, line := range lines {
		for _, pat := range resourceLeakPatterns {
			if pat.MatchString(line) {
				// Check if there's a defer close within the next 5 lines
				hasDefer := false
				for j := i + 1; j < len(lines) && j <= i+5; j++ {
					if deferClosePattern.MatchString(lines[j]) {
						hasDefer = true
						break
					}
				}
				if !hasDefer {
					issues = append(issues, CriticIssue{
						Category: "resource_leak",
						Severity: "warning",
						Detail:   "Resource opened without visible defer Close() — verify cleanup",
						Line:     strings.TrimSpace(line),
					})
				}
				break
			}
		}
	}
	return issues
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
