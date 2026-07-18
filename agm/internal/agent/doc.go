// Package agent provides the Agent interface for multi-agent support in AGM.
//
// # Architecture
//
// AGM uses the Agent interface to present a unified session contract over local
// AI command-line harnesses.
//
//	┌─────────────────┐
//	│   AGM CLI       │
//	│ (new, resume)   │
//	└────────┬────────┘
//	         │
//	┌────────▼────────┐
//	│ Session Manager │
//	│ (orchestration) │
//	└────────┬────────┘
//	         │
//	┌────────▼────────┐
//	│ Agent Interface │ <-- This package
//	└────────┬────────┘
//	         │
//	  ┌──────┴──────┬───────────┐
//	  │             │           │
//	┌─▼────┐   ┌────▼────┐  ┌───▼────┐
//	│Claude│   │ Codex   │  │ others │
//	│ Code │   │ CLI     │  │ in registry
//	└──────┘   └─────────┘  └────────┘
//
// # Usage
//
// Implementations live in sibling *_adapter.go files. The canonical active and
// deprecated harness sets are in harnesses.go. Some subdirectories contain
// compatibility documentation rather than Go implementations.
//
// Example:
//
//	harness, err := GetHarness("codex-cli")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	ctx := SessionContext{
//	    Name:             "my-session",
//	    WorkingDirectory: "~/project",
//	}
//	sessionID, err := harness.CreateSession(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
package agent
