package telemetry

// EcphoryAuditLogger logs ecphory correctness audit results to JSONL.
//
// Each line is one completed audit:
//
//	{
//	  "timestamp": "2026-01-15T10:30:00Z",
//	  "event_type": "ecphory_audit_completed",
//	  "data": {
//	    "total_retrievals": 10,
//	    "appropriate_count": 9,
//	    "inappropriate_count": 1,
//	    "correctness_score": 0.90,
//	    "audit_duration_ms": 450,
//	    "context": {
//	      "language": "python",
//	      "framework": "django",
//	      "task_type": "debugging"
//	    }
//	  }
//	}
//
// Output: ~/.engram/logs/ecphory-audit.jsonl
//
// The write path is jsonlEventLogger, shared with the other per-subject JSONL
// loggers in this package.
type EcphoryAuditLogger struct {
	jsonlEventLogger
}

// NewEcphoryAuditLogger creates a new JSONL logger for ecphory audits.
func NewEcphoryAuditLogger(logDir string) *EcphoryAuditLogger {
	return &EcphoryAuditLogger{jsonlEventLogger{
		logDir:    logDir,
		filename:  "ecphory-audit.jsonl",
		eventType: EventEcphoryAuditCompleted,
		subject:   "audit",
	}}
}
