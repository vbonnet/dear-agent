package safegit

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"golang.org/x/term"
)

// BreakGlassConfig holds options for the break-glass emergency merge path.
type BreakGlassConfig struct {
	PRNumber int
	Repo     string // "owner/repo"
	Now      time.Time
}

// minReasonLen is the minimum character count for a break-glass justification.
const minReasonLen = 20

// breakGlassRetroReminder is printed after every break-glass use.
const breakGlassRetroReminder = `
DEAR RETRO REQUIRED: every break-glass use must produce a retro entry.
  1. Create docs/retros/<date>-break-glass-pr<N>.md
  2. Document: what failed, why gates could not be waited, what to fix
  3. File a follow-up bead for the root-cause fix
`

// BreakGlass runs the emergency merge path: TTY-only, reason-gated, fully
// audited. It refuses to run when stdin is not an interactive terminal (which
// structurally excludes agent-driven invocations).
//
// Sequence:
//  1. Refuse if stdin is not a TTY.
//  2. Prompt for a typed justification (minimum minReasonLen chars).
//  3. Post the justification as a PR comment.
//  4. Execute squash merge (bypassing the normal gates).
//  5. Append a break-glass audit record.
//  6. File a P1 bead via `bd` for the break-glass event.
//  7. Print the DEAR retro reminder.
func BreakGlass(cfg BreakGlassConfig) error {
	if cfg.PRNumber <= 0 {
		return fmt.Errorf("--pr must be a positive integer")
	}
	if cfg.Repo == "" {
		return fmt.Errorf("--repo is required (owner/repo format)")
	}
	if !strings.Contains(cfg.Repo, "/") {
		return fmt.Errorf("--repo must be in owner/repo format, got %q", cfg.Repo)
	}

	// Gate 0: must be run interactively. Agents cannot satisfy this.
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("break-glass requires an interactive TTY — "+
			"agents are structurally excluded from this path; "+
			"run `safe-merge break-glass --pr %d` directly in a terminal", cfg.PRNumber)
	}

	now := cfg.Now
	if now.IsZero() {
		now = time.Now()
	}

	fmt.Fprintf(os.Stderr, "\n⚠ BREAK-GLASS: PR #%d in %s\n", cfg.PRNumber, cfg.Repo)
	fmt.Fprintln(os.Stderr, "This bypasses all safe-merge gates. Every use is audited and triggers a DEAR retro.")
	fmt.Fprintln(os.Stderr)

	// Prompt for a justification.
	reason, err := promptReason(os.Stdin)
	if err != nil {
		return fmt.Errorf("reading justification: %w", err)
	}
	if len(strings.TrimSpace(reason)) < minReasonLen {
		return fmt.Errorf("justification must be at least %d characters (got %d); "+
			"explain what failed and why gates cannot be waited",
			minReasonLen, len(strings.TrimSpace(reason)))
	}

	// Post the justification as a PR comment.
	comment := fmt.Sprintf("**[BREAK-GLASS MERGE]** Bypassing safe-merge gates on %s\n\n**Justification:** %s",
		now.UTC().Format(time.RFC3339), reason)
	if out, err := exec.Command("gh", "pr", "comment",
		fmt.Sprintf("%d", cfg.PRNumber),
		"--repo", cfg.Repo,
		"--body", comment,
	).CombinedOutput(); err != nil {
		return fmt.Errorf("posting PR comment: %w: %s", err, out)
	}
	fmt.Fprintln(os.Stderr, "safe-merge: ✓ justification posted as PR comment")

	// Execute the squash merge (gates bypassed).
	fmt.Fprintf(os.Stderr, "safe-merge: merging PR #%d (break-glass, squash)…\n", cfg.PRNumber)
	mergeCmd := exec.Command("gh", "pr", "merge",
		fmt.Sprintf("%d", cfg.PRNumber),
		"--repo", cfg.Repo,
		"--squash",
		"--delete-branch",
	)
	mergeCmd.Stdout = os.Stdout
	mergeCmd.Stderr = os.Stderr
	if err := mergeCmd.Run(); err != nil {
		appendAuditEntry(cfg.Repo, cfg.PRNumber, "break_glass_error", "merge failed: "+err.Error())
		return fmt.Errorf("gh pr merge failed: %w", err)
	}
	fmt.Fprintln(os.Stderr, "safe-merge: ✓ break-glass merge complete")

	// Audit record.
	appendAuditEntry(cfg.Repo, cfg.PRNumber, "break_glass",
		fmt.Sprintf("reason: %s", strings.TrimSpace(reason)))

	// File a P1 bead for the break-glass event.
	beadTitle := fmt.Sprintf("DEAR retro: break-glass merge of PR #%d (%s)", cfg.PRNumber, cfg.Repo)
	beadBody := fmt.Sprintf("Break-glass merge executed on %s.\nJustification: %s\n\nRequired actions:\n- Write docs/retros/<date>-break-glass-pr%d.md\n- Root-cause what safe-merge gate failed\n- File fix bead",
		now.UTC().Format(time.RFC3339), strings.TrimSpace(reason), cfg.PRNumber)
	if out, err := exec.Command("bd", "add",
		"--priority", "P1",
		"--title", beadTitle,
		"--body", beadBody,
	).CombinedOutput(); err != nil {
		// Non-fatal: bead creation failure should not block the merge audit.
		fmt.Fprintf(os.Stderr, "safe-merge: warning: failed to file break-glass bead: %v: %s\n", err, out)
	} else {
		fmt.Fprintln(os.Stderr, "safe-merge: ✓ P1 bead filed for break-glass event")
	}

	fmt.Fprint(os.Stderr, breakGlassRetroReminder)
	return nil
}

// promptReason prints a prompt to stderr and reads a line from r.
func promptReason(r *os.File) (string, error) {
	fmt.Fprintln(os.Stderr, "Justification (why gates cannot be waited, min 20 chars):")
	fmt.Fprint(os.Stderr, "> ")
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return "", fmt.Errorf("no input received")
	}
	return scanner.Text(), nil
}
