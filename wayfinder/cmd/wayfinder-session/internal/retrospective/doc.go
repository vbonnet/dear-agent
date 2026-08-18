// Package retrospective provides rewind event logging for Wayfinder sessions.
//
// When users rewind to a previous phase, this package captures why they
// rewound, what they learned, and context at rewind time. The resulting audit
// trail helps track iteration patterns and improve the Wayfinder methodology.
//
// # Architecture
//
// The package uses dual logging:
//
//   - WAYFINDER-HISTORY.jsonl: structured JSON Lines events for programmatic analysis
//   - RETRO-retrospective.md: human-readable markdown for reflection and review
//
// # Main entry point
//
// LogRewindEvent calculates the rewind magnitude, optionally prompts for
// context, captures a snapshot, and appends both canonical records. A
// same-phase replay has magnitude zero and still records the same evidence.
//
// # Error handling
//
// Required status, history, and retrospective persistence failures are returned
// to the caller. The rewind command can therefore avoid a normal success claim
// when its audit evidence is incomplete; context probes preserve available
// diagnostic information.
//
// # Example usage
//
//	flags := RewindFlags{
//		Reason:    "Design was too complex",
//		Learnings: "Simpler approaches work better",
//	}
//	err := LogRewindEvent(projectDir, "BUILD", "PLAN", flags)
//	// err reports required persistence failure.
//
// # Testing
//
// Package tests cover canonical rewinds, same-phase replays, prompting, context
// capture, and persistence errors.
package retrospective
