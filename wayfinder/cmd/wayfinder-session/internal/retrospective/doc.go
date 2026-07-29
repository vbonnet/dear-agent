// Package retrospective provides rewind event logging for Wayfinder sessions.
//
// When users rewind to a previous phase, this package captures why they rewound,
// what they learned, and the context at rewind time. This retrospective data helps
// track iteration patterns and improve the Wayfinder methodology.
//
// # Architecture
//
// The package uses a dual-logging strategy:
//   - WAYFINDER-HISTORY.jsonl: Structured JSON Lines events for programmatic analysis
//   - RETRO-retrospective.md: Human-readable markdown for reflection and review
//
// # Main Entry Point
//
// LogRewindEvent() orchestrates the complete logging flow:
//  1. Calculate rewind magnitude (phases moved backwards)
//  2. Prompt user for reason/learnings (if magnitude >= 1)
//  3. Capture context snapshot (git state, deliverables, phase state)
//  4. Log to both HISTORY (JSON) and RETRO (markdown)
//
// # Error Handling
//
// All operations use fail-gracefully error handling - logging failures are logged
// to stderr but never block the rewind operation. This ensures retrospective logging
// adds observability without compromising core functionality.
//
// # Example Usage
//
//	flags := RewindFlags{
//	    Reason: "Design was too complex",
//	    Learnings: "Simpler approaches work better",
//	}
//	err := LogRewindEvent(projectDir, "BUILD", "PLAN", flags)
//	// err is always nil (fail-gracefully design)
//
// # Testing
//
// The package includes 75%+ unit test coverage and 8 integration test scenarios
// covering canonical rewinds, prompting, context capture, and error handling.
package retrospective
