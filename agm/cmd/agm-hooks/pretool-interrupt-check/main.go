// pretool-interrupt-check is a Claude Code PreToolUse hook that checks for
// pending interrupt flags before each tool call.
//
// Behavior by interrupt type:
//   - stop: Blocks the tool call (exit 2) with a message telling the agent to stop.
//           Consumes the flag so it only fires once.
//   - kill: Blocks the tool call (exit 2) with a hard stop message.
//           Does NOT consume the flag — all subsequent calls are also blocked.
//   - steer: Allows the tool call (exit 0) but prints guidance to stderr.
//            Consumes the flag so guidance is only shown once.
//
// Exit codes (Claude Code hook protocol):
//   - 0: allow tool execution
//   - 2: block tool execution
package main

import (
	"fmt"
	"os"

	"github.com/vbonnet/dear-agent/agm/internal/interrupt"
)

func getSessionName() string {
	if name := os.Getenv("CLAUDE_SESSION_NAME"); name != "" {
		return name
	}
	if name := os.Getenv("AGM_SESSION_NAME"); name != "" {
		return name
	}
	return ""
}

func run() int {
	sessionName := getSessionName()
	if sessionName == "" {
		// Not in an AGM session — allow
		return 0
	}

	dir := interrupt.DefaultDir()
	flag, err := interrupt.Read(dir, sessionName)
	if err != nil {
		// Read error — fail open (allow tool call)
		if os.Getenv("AGM_HOOK_DEBUG") == "1" {
			fmt.Fprintf(os.Stderr, "[interrupt-check] error reading flag: %v\n", err)
		}
		return 0
	}

	if flag == nil {
		// No interrupt pending — allow
		return 0
	}

	switch flag.Type {
	case interrupt.TypeStop:
		// Consume the flag, then block
		_, _ = interrupt.Consume(dir, sessionName)
		fmt.Fprintf(os.Stderr, "⛔ INTERRUPT (stop): %s\n", flag.Reason)
		fmt.Fprintf(os.Stderr, "Issued by: %s\n", flag.IssuedBy)
		fmt.Fprintf(os.Stderr, "Stop what you are doing. Do NOT call any more tools. "+
			"Summarize your progress and wait for further instructions.\n")
		return 2

	case interrupt.TypeKill:
		// Do NOT consume — block all subsequent tool calls too
		fmt.Fprintf(os.Stderr, "🛑 INTERRUPT (kill): %s\n", flag.Reason)
		fmt.Fprintf(os.Stderr, "Issued by: %s\n", flag.IssuedBy)
		fmt.Fprintf(os.Stderr, "HARD STOP. All tool calls are blocked. "+
			"Output a brief status and exit immediately.\n")
		return 2

	case interrupt.TypeSteer:
		// Consume the flag, then allow with guidance
		_, _ = interrupt.Consume(dir, sessionName)
		fmt.Fprintf(os.Stderr, "🔀 INTERRUPT (steer): %s\n", flag.Reason)
		fmt.Fprintf(os.Stderr, "Issued by: %s\n", flag.IssuedBy)
		fmt.Fprintf(os.Stderr, "Adjust your approach based on the above guidance and continue working.\n")
		return 0

	default:
		// Unknown type — fail open
		if os.Getenv("AGM_HOOK_DEBUG") == "1" {
			fmt.Fprintf(os.Stderr, "[interrupt-check] unknown interrupt type: %s\n", flag.Type)
		}
		return 0
	}
}

func main() {
	os.Exit(safeRun())
}

func safeRun() (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "[interrupt-check] FATAL: panic (fail-open): %v\n", r)
			exitCode = 0 // fail-open
		}
	}()
	return run()
}
