// Package agent provides the Agent interface for multi-agent support in AGM.
//
// # Architecture
//
// AGM (Agent Manager) uses the Agent interface to support multiple AI providers
// (Claude, Gemini, GPT) with a unified session management experience.
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
//	  ┌──────┴──────┬──────┐
//	  │             │      │
//	┌─▼───┐   ┌─────▼──┐  ┌▼────┐
//	│Claude│  │ Gemini │  │ GPT │
//	└──────┘  └────────┘  └─────┘
//
// # Usage
//
// Agent implementations are in subdirectories:
//   - internal/agent/gemini/   (Gemini API adapter)
//   - internal/agent/gpt/      (GPT API adapter)
//
// Example:
//
//	agent := claude.NewAdapter()
//	ctx := agent.SessionContext{
//	    Name:             "my-session",
//	    WorkingDirectory: "~/project",
//	}
//	sessionID, err := agent.CreateSession(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
// # Design References
//
//   - ~/src/ai-tools/AGM-MULTI-AGENT-ROADMAP.md (Phase 0, Task 2)
//   - Bead oss-6tm6 (Priority P1, 480 minutes)
package agent
