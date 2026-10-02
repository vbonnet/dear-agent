package tmux

import (
	"context"
	"strings"
)

// startupBlockCaptureLines is how much of the pane tail the diagnosis reads. A
// startup dialog and its options render well inside this, and a bounded read
// keeps the timeout path cheap.
const startupBlockCaptureLines = 120

// StartupBlock names an interactive Claude Code startup gate that is holding a
// session short of its prompt. It carries the operator-facing explanation only:
// nothing in this file answers, dismisses, or otherwise acts on a dialog.
type StartupBlock struct {
	// Kind is a short identifier for logs and tests.
	Kind string
	// Summary is the one-line cause shown above the troubleshooting steps.
	Summary string
	// Remedy is the concrete next step for the operator.
	Remedy string
}

// startupBlockSignatures maps a detected gate to its explanation. Matching is
// on durable prompt wording rather than on the exact option layout: the point
// is to name the blocker in the timeout message, so a near-miss costs a vaguer
// hint, never a wrong action.
//
// Ordering matters. The trust dialog gates the MCP dialog, so when a redraw
// leaves traces of both the earlier gate is the one to report.
var startupBlockSignatures = []struct {
	markers []string
	block   StartupBlock
}{
	{
		markers: []string{
			"Is this a project you created or one you trust",
			"Do you trust the files in this folder?",
		},
		block: StartupBlock{
			Kind:    "workspace-trust",
			Summary: "Claude Code is blocked on the workspace-trust dialog and nobody can answer it.",
			Remedy: "A sandbox workspace is a brand-new directory, so Claude Code has never\n" +
				"    recorded trust for that path and asks before starting. Attach and answer\n" +
				"    once to confirm:  tmux -S ~/.agm/agm.sock attach -t ",
		},
	},
	{
		markers: []string{
			"new MCP servers found in this project",
			"Select any you wish to enable",
		},
		block: StartupBlock{
			Kind:    "mcp-approval",
			Summary: "Claude Code is blocked on the project MCP-server approval dialog.",
			Remedy: "The workspace declares MCP servers in .mcp.json that have no recorded\n" +
				"    decision for this path. Attach and choose once:  tmux -S ~/.agm/agm.sock attach -t ",
		},
	},
	{
		markers: []string{
			"Invalid API key",
			"OAuth access token revoked",
			"Please run /login",
			"/login",
		},
		block: StartupBlock{
			Kind:    "authentication",
			Summary: "Claude Code reached its startup screen but is not authenticated.",
			Remedy: "Check the file-backed token store the spawned session reads:\n" +
				"    ~/.claude/.credentials.json. Re-authenticate on the host with  claude /login  then retry: ",
		},
	},
}

// DiagnoseStartupBlock reports which interactive startup gate, if any, is
// holding sessionName short of its prompt. It is read-only: it captures the
// pane and classifies the text, and never sends a key.
//
// This exists because the readiness timeout is otherwise unattributable. A
// blocked session and a crashed one both surface as "timeout waiting for Claude
// prompt", which sends the operator to the generic "is claude installed?"
// checklist for what is really an unanswered dialog.
//
// A capture failure yields no block rather than an error: this runs on a path
// that is already failing, and a diagnosis that cannot be made must not replace
// the original error.
func DiagnoseStartupBlock(ctx context.Context, sessionName string) (StartupBlock, bool) {
	content, err := CapturePaneOutputContext(ctx, sessionName, startupBlockCaptureLines)
	if err != nil {
		return StartupBlock{}, false
	}
	return classifyStartupBlock(content)
}

// classifyStartupBlock is the pure half of DiagnoseStartupBlock, split out so
// the signature table can be tested against recorded pane text.
func classifyStartupBlock(content string) (StartupBlock, bool) {
	plain := stripANSI(content)
	for _, signature := range startupBlockSignatures {
		for _, marker := range signature.markers {
			if strings.Contains(plain, marker) {
				return signature.block, true
			}
		}
	}
	return StartupBlock{}, false
}
