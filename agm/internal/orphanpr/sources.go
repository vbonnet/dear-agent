package orphanpr

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"time"
)

// ghListTimeout caps the single `gh pr list` call; bdShowTimeout caps each
// per-bead `bd show`. Both are hard-bounded and prompt-disabled so the scan is
// safe to run from supervisor tooling without ever hanging on auth or network.
const (
	ghListTimeout = 20 * time.Second
	bdShowTimeout = 8 * time.Second
)

// ListOpenPRs returns all open pull requests in repo (owner/name) via the gh
// CLI. It never prompts and is bounded by ghListTimeout.
func ListOpenPRs(repo string) ([]PR, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, fmt.Errorf("gh CLI not found on PATH: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), ghListTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "pr", "list",
		"--repo", repo,
		"--state", "open",
		"--limit", "1000",
		"--json", "number,title,body")
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GH_PROMPT_DISABLED=1",
		"GH_NO_UPDATE_NOTIFIER=1",
	)

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh pr list failed for %s: %w", repo, err)
	}

	var prs []PR
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, fmt.Errorf("parsing gh pr list output: %w", err)
	}
	return prs, nil
}

// bdStatusRe captures the status token in a `bd show` header line, e.g.
// "[● P1 · OPEN]" or "[● P0 · CLOSED]". The status is the last "· WORD"
// segment before the closing bracket.
var bdStatusRe = regexp.MustCompile(`·\s*([A-Z_]+)\s*\]`)

// BeadStatusFromDB resolves the lifecycle status of beadID by parsing the
// header line of `bd --db <db> show <beadID>`. Any error or unrecognized
// output yields BeadUnknown — never BeadClosed — so a missing bead or a bd
// failure can't masquerade as an orphan trigger.
func BeadStatusFromDB(db, beadID string) BeadStatus {
	if _, err := exec.LookPath("bd"); err != nil {
		return BeadUnknown
	}

	ctx, cancel := context.WithTimeout(context.Background(), bdShowTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bd", "--db", db, "show", beadID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return BeadUnknown
	}
	return parseBeadStatus(string(out))
}

// parseBeadStatus maps a `bd show` output to a BeadStatus by reading the
// bracketed status token. CLOSED ⇒ closed; any other recognized token
// (OPEN, IN_PROGRESS, BLOCKED, …) ⇒ open; no token ⇒ unknown.
func parseBeadStatus(showOutput string) BeadStatus {
	m := bdStatusRe.FindStringSubmatch(showOutput)
	if m == nil {
		return BeadUnknown
	}
	if m[1] == "CLOSED" {
		return BeadClosed
	}
	return BeadOpen
}
