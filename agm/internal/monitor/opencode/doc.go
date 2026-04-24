// Package opencode provides monitoring infrastructure for OpenCode agent sessions.
//
// This package is part of the AGM multi-agent integration architecture and provides
// tools for parsing OpenCode SSE events and mapping them to AGM state transitions.
//
// OpenCode runs as a client-server architecture with a headless server (opencode serve)
// and a separate TUI client (opencode attach). The server exposes a Server-Sent Events
// (SSE) endpoint at GET /event that streams real-time execution state events.
//
// Key features:
//   - Parse OpenCode SSE event JSON payloads
//   - Map OpenCode event types to AGM state model
//   - Extract event metadata (permission IDs, tool names, file paths)
//   - Validate event schema with graceful error handling
//   - Handle unknown event types with safe defaults
//
// Event Mapping:
//   - permission.asked     → AWAITING_PERMISSION
//   - tool.execute.before  → THINKING
//   - tool.execute.after   → IDLE
//   - session.created      → READY
//   - session.closed       → TERMINATED
//   - unknown event types  → THINKING (safe default)
//
// Example usage:
//
//	// Parse an OpenCode SSE event
//	parser := opencode.NewEventParser()
//	eventJSON := []byte(`{
//	    "type": "permission.asked",
//	    "timestamp": 1709654321,
//	    "properties": {
//	        "permission": {
//	            "id": "perm-123",
//	            "action": "file.write",
//	            "path": "~/main.go"
//	        }
//	    }
//	}`)
//
//	agmEvent, err := parser.Parse(eventJSON)
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("State: %s, Timestamp: %d, Permission: %s\n",
//	    agmEvent.State, agmEvent.Timestamp, agmEvent.Metadata["permission_id"])
//
// The parser is designed to be resilient to malformed events and version changes,
// logging warnings for unknown event types while continuing to process events.
//
// This package integrates with the AGM EventBus architecture, where parsed events
// are published to the canonical EventBus for consumption by notification managers,
// state file writers, and tmux status displays.
//
// See ARCHITECTURE.md and MULTI-AGENT-INTEGRATION-SPEC.md for complete integration details.
package opencode
