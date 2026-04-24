// Package analyzer provides tools for analyzing bash blocker hook denial logs,
// correlating with Claude Code transcripts, and identifying false positives.
package analyzer

import (
	"encoding/json"
	"time"
)

// DenialEntry represents a single parsed denial from the hook log.
type DenialEntry struct {
	Timestamp      time.Time
	SessionID      string
	TranscriptPath string
	ToolUseID      string
	Command        string
	PatternName    string
	PatternIndex   int
	PatternRegex   string
	Remediation    string
	CWD            string
}

// ApprovalEntry represents a parsed approval from the hook log.
type ApprovalEntry struct {
	Timestamp      time.Time
	SessionID      string
	TranscriptPath string
	ToolUseID      string
	Command        string
}

// HookLogStats holds aggregate statistics from log parsing.
type HookLogStats struct {
	TotalInvocations int            `json:"total_invocations"`
	TotalDenials     int            `json:"total_denials"`
	TotalApprovals   int            `json:"total_approvals"`
	DenialsByPattern map[string]int `json:"denials_by_pattern"`
	TimeRange        [2]time.Time   `json:"time_range"`
	UniqueSessionIDs int            `json:"unique_session_ids"`
}

// RawHookInput represents the JSON payload from Claude Code's PreToolUse hook.
type RawHookInput struct {
	SessionID      string    `json:"session_id"`
	TranscriptPath string    `json:"transcript_path"`
	CWD            string    `json:"cwd"`
	PermissionMode string    `json:"permission_mode"`
	HookEventName  string    `json:"hook_event_name"`
	ToolName       string    `json:"tool_name"`
	ToolInput      ToolInput `json:"tool_input"`
	ToolUseID      string    `json:"tool_use_id"`
	Traceparent    string    `json:"traceparent,omitempty"`
	Tracestate     string    `json:"tracestate,omitempty"`
}

// ToolInput holds the command and description from the hook payload.
type ToolInput struct {
	Command     string `json:"command"`
	Description string `json:"description"`
}

// TranscriptEntry represents a single line from a Claude Code transcript JSONL.
type TranscriptEntry struct {
	ParentUUID  string          `json:"parentUuid"`
	SessionID   string          `json:"sessionId"`
	Type        string          `json:"type"` // "user", "assistant"
	UUID        string          `json:"uuid"`
	Timestamp   string          `json:"timestamp"`
	Message     json.RawMessage `json:"message,omitempty"`
	CWD         string          `json:"cwd"`
	IsSidechain bool            `json:"isSidechain"`
}

// TranscriptMessage holds the role and content of a transcript message.
type TranscriptMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// ContentBlock represents a content block in a transcript message.
type ContentBlock struct {
	Type      string          `json:"type"` // "tool_use", "tool_result", "text"
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
}

// DenialOutcome classifies what happened after a denial.
type DenialOutcome int

const (
	OutcomeUnknown           DenialOutcome = iota
	OutcomeRetrySuccess                    // Agent retried with modified command and succeeded
	OutcomeSwitchedTool                    // Agent used the recommended alternative tool
	OutcomeRetryDenied                     // Agent retried and was denied again
	OutcomeGaveUp                          // Agent stopped trying
	OutcomeTranscriptMissing               // Could not find/read the transcript
)

// String returns the string representation of a DenialOutcome.
func (o DenialOutcome) String() string {
	switch o {
	case OutcomeUnknown:
		return "unknown"
	case OutcomeRetrySuccess:
		return "retry_success"
	case OutcomeSwitchedTool:
		return "switched_tool"
	case OutcomeRetryDenied:
		return "retry_denied"
	case OutcomeGaveUp:
		return "gave_up"
	case OutcomeTranscriptMissing:
		return "transcript_missing"
	default:
		return "unknown"
	}
}

// ClassifiedDenial combines a denial with its outcome analysis.
type ClassifiedDenial struct {
	Denial          DenialEntry   `json:"denial"`
	Outcome         DenialOutcome `json:"outcome"`
	NextToolName    string        `json:"next_tool_name"`
	RetryCount      int           `json:"retry_count"`
	WastedCalls     int           `json:"wasted_calls"`
	IsFalsePositive bool          `json:"is_false_positive"`
	Confidence      float64       `json:"confidence"`
}

// PatternAnalysis holds per-pattern aggregate analysis.
type PatternAnalysis struct {
	PatternName       string             `json:"pattern_name"`
	PatternIndex      int                `json:"pattern_index"`
	PatternRegex      string             `json:"pattern_regex"`
	TotalDenials      int                `json:"total_denials"`
	FalsePositives    int                `json:"false_positives"`
	TruePositives     int                `json:"true_positives"`
	Uncertain         int                `json:"uncertain"`
	FalsePositiveRate float64            `json:"false_positive_rate"`
	ExampleFPs        []ClassifiedDenial `json:"example_fps,omitempty"`
	ExampleTPs        []ClassifiedDenial `json:"example_tps,omitempty"`
	ProposedFix       *PatternFix        `json:"proposed_fix,omitempty"`
}

// PatternFix proposes a tighter regex for a pattern.
type PatternFix struct {
	OriginalRegex      string   `json:"original_regex"`
	ProposedRegex      string   `json:"proposed_regex"`
	FPsFixed           int      `json:"fps_fixed"`
	TPsPreserved       int      `json:"tps_preserved"`
	TPsLost            int      `json:"tps_lost"`
	ExampleFixed       []string `json:"example_fixed,omitempty"`
	ExampleStillCaught []string `json:"example_still_caught,omitempty"`
}

// Report is the top-level output structure.
type Report struct {
	GeneratedAt      time.Time         `json:"generated_at"`
	LogPath          string            `json:"log_path"`
	LogLineCount     int               `json:"log_line_count"`
	TimeRange        [2]time.Time      `json:"time_range"`
	Stats            HookLogStats      `json:"stats"`
	Patterns         []PatternAnalysis `json:"patterns"`
	OverallFPRate    float64           `json:"overall_fp_rate"`
	TotalWastedCalls int               `json:"total_wasted_calls"`
}
