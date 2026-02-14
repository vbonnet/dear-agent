package activities

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

// MonitorOutputInput contains parameters for monitoring agent output
type MonitorOutputInput struct {
	SessionID   string        // Session to monitor
	PID         int           // Process ID to monitor
	Reader      io.Reader     // Output stream to monitor (stdout/stderr)
	Timeout     time.Duration // How long to monitor before timing out
	MaxLines    int           // Maximum lines to buffer (0 = unlimited)
}

// EscalationPattern represents a pattern that indicates escalation is needed
type EscalationPattern struct {
	Pattern     string // Regular expression pattern
	Type        string // Type of escalation: "error", "prompt", "warning"
	Description string // Human-readable description
}

// EscalationEvent represents a detected escalation in the output
type EscalationEvent struct {
	Type        string    // Type of escalation
	Pattern     string    // Pattern that matched
	Line        string    // The actual line that triggered escalation
	LineNumber  int       // Line number in the output
	DetectedAt  time.Time // When the escalation was detected
	Description string    // Description of the escalation
}

// MonitorOutputOutput contains the results of monitoring
type MonitorOutputOutput struct {
	SessionID    string             // Session that was monitored
	LinesRead    int                // Total lines read
	Escalations  []EscalationEvent  // Detected escalations
	LastActivity time.Time          // Time of last output activity
	Completed    bool               // Whether monitoring completed normally
	Error        string             // Error message if monitoring failed
}

// Default escalation patterns for Claude Code and Gemini CLI
var defaultEscalationPatterns = []EscalationPattern{
	// Error patterns
	{
		Pattern:     `(?i)error:`,
		Type:        "error",
		Description: "Generic error message",
	},
	{
		Pattern:     `(?i)fatal:`,
		Type:        "error",
		Description: "Fatal error",
	},
	{
		Pattern:     `(?i)failed to`,
		Type:        "error",
		Description: "Operation failure",
	},
	{
		Pattern:     `(?i)permission denied`,
		Type:        "error",
		Description: "Permission error",
	},

	// Prompt patterns (user input required)
	{
		Pattern:     `(?i)(yes/no|y/n)\s*[?:]?\s*$`,
		Type:        "prompt",
		Description: "Yes/No confirmation prompt",
	},
	{
		Pattern:     `(?i)enter.*:?\s*$`,
		Type:        "prompt",
		Description: "Input prompt",
	},
	{
		Pattern:     `(?i)continue\?`,
		Type:        "prompt",
		Description: "Continue confirmation",
	},
	{
		Pattern:     `(?i)press.*key`,
		Type:        "prompt",
		Description: "Key press required",
	},

	// Warning patterns
	{
		Pattern:     `(?i)warning:`,
		Type:        "warning",
		Description: "Warning message",
	},
	{
		Pattern:     `(?i)deprecated:`,
		Type:        "warning",
		Description: "Deprecation warning",
	},

	// Agent-specific patterns
	{
		Pattern:     `(?i)rate limit`,
		Type:        "error",
		Description: "API rate limit exceeded",
	},
	{
		Pattern:     `(?i)authentication.*failed`,
		Type:        "error",
		Description: "Authentication failure",
	},
	{
		Pattern:     `(?i)api.*key.*invalid`,
		Type:        "error",
		Description: "Invalid API key",
	},
}

// MonitorOutputActivity monitors stdout/stderr for escalation patterns
// This activity reads process output streams and detects patterns that require user intervention
func MonitorOutputActivity(ctx context.Context, input MonitorOutputInput) (*MonitorOutputOutput, error) {
	// Validate input
	if input.SessionID == "" {
		return nil, fmt.Errorf("session ID cannot be empty")
	}
	if input.Reader == nil {
		return nil, fmt.Errorf("reader cannot be nil")
	}

	// Set defaults
	if input.Timeout == 0 {
		input.Timeout = 30 * time.Second // Default 30s timeout
	}
	if input.MaxLines == 0 {
		input.MaxLines = 10000 // Default max buffer
	}

	// Compile escalation patterns
	compiledPatterns, err := compilePatterns(defaultEscalationPatterns)
	if err != nil {
		return nil, fmt.Errorf("failed to compile patterns: %w", err)
	}

	// Set up output tracking
	output := &MonitorOutputOutput{
		SessionID:   input.SessionID,
		LinesRead:   0,
		Escalations: make([]EscalationEvent, 0),
		Completed:   false,
	}

	// Create scanner for line-by-line reading
	scanner := bufio.NewScanner(input.Reader)

	// Set up timeout
	deadline := time.Now().Add(input.Timeout)

	// Monitor loop
	lineNumber := 0
	for scanner.Scan() {
		// Check context cancellation
		select {
		case <-ctx.Done():
			output.Error = "monitoring cancelled"
			return output, ctx.Err()
		default:
		}

		// Check timeout
		if time.Now().After(deadline) {
			output.Completed = false
			return output, nil
		}

		line := scanner.Text()
		lineNumber++
		output.LinesRead = lineNumber
		output.LastActivity = time.Now()

		// Check against escalation patterns
		for _, pattern := range compiledPatterns {
			if pattern.regex.MatchString(line) {
				escalation := EscalationEvent{
					Type:        pattern.escalation.Type,
					Pattern:     pattern.escalation.Pattern,
					Line:        line,
					LineNumber:  lineNumber,
					DetectedAt:  time.Now(),
					Description: pattern.escalation.Description,
				}
				output.Escalations = append(output.Escalations, escalation)
			}
		}

		// Prevent unbounded memory growth
		if lineNumber >= input.MaxLines {
			output.Error = fmt.Sprintf("max lines (%d) exceeded", input.MaxLines)
			output.Completed = false
			return output, nil
		}
	}

	// Check for scanner errors
	if err := scanner.Err(); err != nil {
		output.Error = fmt.Sprintf("scanner error: %v", err)
		return output, err
	}

	output.Completed = true
	return output, nil
}

// compiledPattern wraps a regex with its source pattern
type compiledPattern struct {
	regex      *regexp.Regexp
	escalation EscalationPattern
}

// compilePatterns compiles all escalation patterns
func compilePatterns(patterns []EscalationPattern) ([]compiledPattern, error) {
	compiled := make([]compiledPattern, 0, len(patterns))

	for _, pattern := range patterns {
		regex, err := regexp.Compile(pattern.Pattern)
		if err != nil {
			return nil, fmt.Errorf("failed to compile pattern '%s': %w", pattern.Pattern, err)
		}
		compiled = append(compiled, compiledPattern{
			regex:      regex,
			escalation: pattern,
		})
	}

	return compiled, nil
}

// DetectEscalation checks a single line for escalation patterns
// This is a helper function for testing or ad-hoc pattern detection
func DetectEscalation(line string) *EscalationEvent {
	// Compile patterns (cached in real usage)
	compiledPatterns, err := compilePatterns(defaultEscalationPatterns)
	if err != nil {
		return nil
	}

	// Check each pattern
	for _, pattern := range compiledPatterns {
		if pattern.regex.MatchString(line) {
			return &EscalationEvent{
				Type:        pattern.escalation.Type,
				Pattern:     pattern.escalation.Pattern,
				Line:        line,
				DetectedAt:  time.Now(),
				Description: pattern.escalation.Description,
			}
		}
	}

	return nil
}

// FormatEscalations returns a human-readable summary of escalations
func FormatEscalations(escalations []EscalationEvent) string {
	if len(escalations) == 0 {
		return "No escalations detected"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Detected %d escalation(s):\n", len(escalations)))

	for i, esc := range escalations {
		sb.WriteString(fmt.Sprintf("%d. [%s] %s\n", i+1, esc.Type, esc.Description))
		sb.WriteString(fmt.Sprintf("   Line %d: %s\n", esc.LineNumber, esc.Line))
	}

	return sb.String()
}
