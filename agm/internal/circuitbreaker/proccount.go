package circuitbreaker

import (
	"context"
	"fmt"
	"os/exec"
	"path"
	"regexp"
	"strings"
	"time"
)

// psTimeout bounds the `ps` subprocess so a hung probe fails the gate closed
// instead of hanging session spawns.
const psTimeout = 5 * time.Second

// agentProcExcludeRe drops GUI-app bundles and their helper processes that
// share the claude/codex executable names but are not headless mesh workers:
// Claude.app / ChatGPT.app and their frameworks, crashpad handlers, and
// anything under /Applications or Application Support. It is matched against
// the full command line so paths containing spaces (e.g. "Codex Computer
// Use.app") are still excluded.
var agentProcExcludeRe = regexp.MustCompile(`(?i)\.app/|/Applications/|Application Support/|Frameworks/|Helper|crashpad`)

// agentProcNames is the set of executable basenames that identify a mesh agent
// harness process. Matching argv[0]'s basename (rather than any substring of
// the command line) avoids counting shell wrappers whose arguments merely
// mention a ~/.claude path.
var agentProcNames = map[string]bool{
	"claude": true,
	"codex":  true,
}

// PsAgentProcCounter counts agent harness processes machine-wide by reading
// the process table with `ps`. This counts real PIDs rather than AGM's session
// records, so Dispatch-spawned workers are visible to the admission gate.
type PsAgentProcCounter struct{}

// CountAgentProcs returns the number of agent harness processes running
// machine-wide. It returns an error (so the gate fails closed) when `ps`
// cannot be run.
func (PsAgentProcCounter) CountAgentProcs() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), psTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "ps", "-axo", "command=").Output()
	if ctx.Err() != nil {
		return 0, fmt.Errorf("ps -axo command=: %w", ctx.Err())
	}
	if err != nil {
		return 0, fmt.Errorf("ps -axo command=: %w", err)
	}
	return countAgentProcs(string(out)), nil
}

// countAgentProcs parses `ps -axo command=` output and counts lines whose
// argv[0] basename is an agent harness executable, excluding GUI-app and
// helper processes. It is pure to keep it unit-testable without spawning ps.
func countAgentProcs(psOutput string) int {
	count := 0
	for line := range strings.SplitSeq(psOutput, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Exclude GUI apps / helpers using the full command line so paths
		// with spaces (e.g. "Codex Computer Use.app") are still caught.
		if agentProcExcludeRe.MatchString(line) {
			continue
		}
		// argv[0] is the first whitespace-delimited field; match its
		// basename against the known harness executables.
		argv0 := line
		if i := strings.IndexAny(argv0, " \t"); i >= 0 {
			argv0 = argv0[:i]
		}
		base := strings.ToLower(path.Base(argv0))
		if agentProcNames[base] {
			count++
		}
	}
	return count
}

// DefaultProcCounter returns the machine-wide agent-process counter.
func DefaultProcCounter() ProcCounter {
	return PsAgentProcCounter{}
}
