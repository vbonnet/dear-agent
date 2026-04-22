package errormemory

import "time"

// ErrorRecord represents a deduplicated error pattern in the JSONL database.
type ErrorRecord struct {
	ID            string    `json:"id"`
	Pattern       string    `json:"pattern"`
	ErrorCategory string    `json:"error_category"`
	CommandSample string    `json:"command_sample"`
	Remediation   string    `json:"remediation"`
	Count         int       `json:"count"`
	FirstSeen     time.Time `json:"first_seen"`
	LastSeen      time.Time `json:"last_seen"`
	TTLExpiry     time.Time `json:"ttl_expiry"`
	SessionIDs    []string  `json:"session_ids"`
	Source        string    `json:"source"`
}

// ErrorSummary is the injection payload for SessionStart.
type ErrorSummary struct {
	Entries    []ErrorSummaryEntry `json:"entries"`
	Generated  time.Time           `json:"generated"`
	TokenCount int                 `json:"token_count"`
}

// ErrorSummaryEntry is a single entry in the injected summary.
type ErrorSummaryEntry struct {
	Pattern     string `json:"pattern"`
	Remediation string `json:"remediation"`
	Count       int    `json:"count"`
	LastSeen    string `json:"last_seen"`
}

const (
	DefaultTTL        = 30 * 24 * time.Hour
	DefaultDBPath     = "~/.agm/error-memory.jsonl"
	MaxRecords        = 5000
	MaxSummaryTokens  = 500
	MaxSummaryEntries = 10

	SourceBashBlocker      = "bash-blocker"
	SourceAstrocyte        = "astrocyte"
	SourcePermissionPrompt = "permission-prompt"
)
