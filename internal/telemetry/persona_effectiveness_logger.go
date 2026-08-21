package telemetry

// PersonaEffectivenessLogger logs persona review results to JSONL.
//
// Each line is one completed review:
//
//	{
//	  "timestamp": "2026-01-15T10:30:00Z",
//	  "event_type": "persona_review_completed",
//	  "data": {
//	    "persona_id": "reuse-advocate",
//	    "issues_found": 2,
//	    "severity": "medium",
//	    "time_overhead_ms": 350,
//	    "false_positives": 0,
//	    "classification_metadata": {
//	      "language": "go",
//	      "pattern": "duplicate_code",
//	      "confidence": 0.95
//	    }
//	  }
//	}
//
// Output: ~/.engram/logs/persona-effectiveness.jsonl
//
// The write path is jsonlEventLogger, shared with the other per-subject JSONL
// loggers in this package.
type PersonaEffectivenessLogger struct {
	jsonlEventLogger
}

// NewPersonaEffectivenessLogger creates a new JSONL logger for persona reviews.
func NewPersonaEffectivenessLogger(logDir string) *PersonaEffectivenessLogger {
	return &PersonaEffectivenessLogger{jsonlEventLogger{
		logDir:    logDir,
		filename:  "persona-effectiveness.jsonl",
		eventType: EventPersonaReviewCompleted,
		subject:   "persona",
	}}
}
