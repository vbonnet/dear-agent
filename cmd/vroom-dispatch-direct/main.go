// Command vroom-dispatch-direct dispatches worker sessions straight from
// `bd ready`, with no intermediate prompt-file layer.
//
// Background (ce-1jm2): the original dispatch pipeline materialised a prompt
// file under ~/.agm/vroom/prompts/<bead-id>.md for every ready bead (ce-5z0o),
// then dispatched from that on-disk library. The file layer was a stopgap: it
// added a stateful directory the orchestrator had to keep in sync with reality,
// and "already has a prompt file" is a poor proxy for "already dispatched" — a
// file lingers after its worker dies, and a fresh file is no guarantee a worker
// ever spawned. This tool eliminates the layer: it reads `bd ready`, renders the
// worker prompt in memory, and dispatches the worker directly. Dispatch state is
// derived from ground truth — live `worker-<id>` sessions and open PRs — not from
// a directory of files that can drift.
//
// Deduplication is layered so re-running is idempotent and never double-dispatches:
//  1. live worker sessions (a `worker-<id>` session already exists)
//  2. open PRs (the bead is already in flight — its id appears in a PR branch/title)
//  3. the human-gated skip list (beads a human must drive, never an autonomous worker)
//
// Capacity is controlled by real spawn backpressure. This dispatcher does not
// impose a hard worker-count cap; it dispatches eligible beads until the
// candidate list is exhausted or `agm session new` refuses a spawn.
//
// Exit status is 0 on success even when zero beads are dispatched — "nothing new
// to dispatch" (backlog drained, at capacity, or all in flight) is a normal
// steady state, not an error.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// subprocessTimeout bounds every bd/gh/agm child so a hung subprocess cannot
// stall a dispatch run indefinitely. The styleguide requires subprocess calls to
// be both context-propagated and timeout-bounded; each runner derives a deadline
// from the caller's ctx so SIGINT/SIGTERM cancellation still composes with it.
const subprocessTimeout = 60 * time.Second

// humanGated lists beads that must never be auto-dispatched to a worker: they
// require a human in the loop (credential rotation, backups, repointing live
// skills, destructive ops) or are otherwise gated by operator decision. Kept in
// sync with vroom-prompt-gen and the orchestrator skill's skip list (ce-5z0o).
var humanGated = map[string]bool{
	"ce-pqha":    true,
	"ce-8qi":     true,
	"ce-kgd":     true,
	"ce-9uo":     true,
	"ce-xulg.14": true,
	"ce-126c":    true,
	"ce-cd14":    true,
	"ce-cd14.2":  true,
	"ce-cd14.1":  true,
	"ce-rrry":    true,
	"ce-clm6":    true,
}

// defaultModel is the model alias workers spawn with. opus-200k → claude-opus-4-8:
// design-phase work needs Opus, and the 200k variant dodges the 1M credit gate
// that the bare opus/sonnet aliases trip on this Max-plan auth (ce-84l2).
const defaultModel = "opus-200k"

// workerRole is the RBAC profile workers spawn with.
const workerRole = "worker"

// dispatchSender is the --sender label on dispatch messages, so the trail
// attributes them to this tool rather than a human.
const dispatchSender = "vroom-orchestrator"

// bead mirrors the fields of a `bd ready --json` array element that we consume.
type bead struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    int    `json:"priority"`
	IssueType   string `json:"issue_type"`
}

// pullRequest mirrors the fields of a `gh pr list --json` array element.
type pullRequest struct {
	Number      int    `json:"number"`
	HeadRefName string `json:"headRefName"`
	Title       string `json:"title"`
}

// queryReady runs `bd --db <db> ready --json` and returns the ready beads. It is
// a package var so tests can stub the bd invocation without a real database.
var queryReady = func(ctx context.Context, db string) ([]bead, error) {
	ctx, cancel := context.WithTimeout(ctx, subprocessTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bd", "--db", db, "ready", "--json")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("bd ready: %w", err)
	}
	var beads []bead
	if err := json.Unmarshal(out, &beads); err != nil {
		return nil, fmt.Errorf("parse bd ready output: %w", err)
	}
	return beads, nil
}

// queryOpenPRs runs `gh pr list` for the repo and returns open PRs. It is a
// package var so tests can stub the gh invocation. A gh failure is returned as
// an error so the caller can fail closed rather than silently dispatching beads
// that already have an open PR.
var queryOpenPRs = func(ctx context.Context, repo string) ([]pullRequest, error) {
	ctx, cancel := context.WithTimeout(ctx, subprocessTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "pr", "list",
		"--repo", repo,
		"--state", "open",
		"--limit", "200",
		"--json", "number,headRefName,title")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh pr list: %w", err)
	}
	var prs []pullRequest
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, fmt.Errorf("parse gh pr list output: %w", err)
	}
	return prs, nil
}

// listSessions runs `agm session list` and returns its raw lines. It is a package
// var so tests can stub the agm invocation. A failure is returned as an error so
// the caller can fail closed: without the session list we cannot tell which beads
// are already being worked, and must not risk double-dispatching them.
var listSessions = func(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, subprocessTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "agm", "session", "list")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("agm session list: %w", err)
	}
	return strings.Split(string(out), "\n"), nil
}

// workerSessionRe matches a worker session name and captures the bead id, e.g.
// "worker-ce-bi19" -> "ce-bi19" and "worker-ce-cd14.2" -> "ce-cd14.2". The id
// runs to a whitespace or end-of-token boundary so trailing status columns in the
// `agm session list` output do not bleed into the captured id.
//
// "worker-" must sit at the start of a line or field (anchored to line-start or
// preceding whitespace) rather than a bare \b word boundary: dispatched sessions
// are named exactly "worker-<id>", so a hyphen-joined name like "my-worker-x" or
// "subworker-x" is a different session and must NOT be read as a live worker.
var workerSessionRe = regexp.MustCompile(`(?m)(?:^|\s)worker-([A-Za-z0-9.-]+)`)

// liveWorkerIDs scans `agm session list` output for `worker-<id>` session names
// and returns the set of bead ids that already have a live worker. This is the
// ground-truth replacement for vroom-prompt-gen's "already has a prompt file"
// check: a session exists iff a worker is actually running the bead.
func liveWorkerIDs(lines []string) map[string]bool {
	ids := make(map[string]bool)
	for _, line := range lines {
		for _, m := range workerSessionRe.FindAllStringSubmatch(line, -1) {
			ids[m[1]] = true
		}
	}
	return ids
}

// mentionsID reports whether text references the bead id as a whole token rather
// than as a prefix of a longer id. The trailing boundary explicitly excludes '.'
// so that a sub-bead reference like "ce-cd14.2" does NOT count as a mention of
// its parent "ce-cd14" — otherwise an open PR for a child would wrongly suppress
// the parent (and vice versa). The leading boundary excludes alphanumerics so an
// id embedded in a branch path like "feat/fix-ce-5z0o" still matches.
// mentionsIDCache memoizes the compiled whole-token matcher for each bead id.
// Without it, scanning N PRs for M beads recompiled the same per-id regex on
// every call (O(N*M) compilations); caching makes it one compilation per id.
var (
	mentionsIDMu    sync.Mutex
	mentionsIDCache = map[string]*regexp.Regexp{}
)

func mentionsIDRe(id string) *regexp.Regexp {
	mentionsIDMu.Lock()
	defer mentionsIDMu.Unlock()
	if re, ok := mentionsIDCache[id]; ok {
		return re
	}
	re := regexp.MustCompile(`(^|[^A-Za-z0-9])` + regexp.QuoteMeta(id) + `([^A-Za-z0-9.]|$)`)
	mentionsIDCache[id] = re
	return re
}

func mentionsID(text, id string) bool {
	return mentionsIDRe(id).MatchString(text)
}

// inFlightInPR reports whether any open PR already covers this bead, matching the
// id against each PR's branch name and title.
func inFlightInPR(id string, prs []pullRequest) bool {
	for _, pr := range prs {
		if mentionsID(pr.HeadRefName, id) || mentionsID(pr.Title, id) {
			return true
		}
	}
	return false
}

// priorityLabel renders bd's numeric priority (0,1,2,3) as the P0/P1/... label
// used throughout the dispatch tooling and bead summaries.
func priorityLabel(p int) string {
	return fmt.Sprintf("P%d", p)
}

// firstParagraph returns the first paragraph of a description (everything up to
// the first blank line), collapsed to a single line. Used for the prompt's
// one-line summary so the worker sees the gist before the full goal block.
func firstParagraph(desc string) string {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return ""
	}
	if idx := strings.Index(desc, "\n\n"); idx >= 0 {
		desc = desc[:idx]
	}
	return strings.Join(strings.Fields(desc), " ")
}

// renderPrompt produces the dispatch prompt sent to a worker session. It is full
// parity with the manual orchestrator dispatch prompt (the one eliminated with
// the prompt-file layer): the worker drives the bead through the wayfinder SDLC
// workflow, works in an isolated worktree off the read-only ~/src checkout, and
// honours the merged-PR Definition of Done. Dispatching directly from beads must
// not water the worker's instructions down — hence the wayfinder process and DoD
// rules live here, not just the terse rule list.
func renderPrompt(b bead) string {
	prio := priorityLabel(b.Priority)
	summary := firstParagraph(b.Description)
	if summary == "" {
		summary = b.Title
	}
	goal := strings.TrimSpace(b.Description)
	if goal == "" {
		goal = b.Title
	}

	return fmt.Sprintf(`# Worker: %s — %s

You are a worker session assigned to bead %s (%s): %s

**Stop after work complete + bead note written, OR PR created + auto-merge armed. Do NOT close bead. Do NOT create new beads.**

## Goal

%s

## Process (MANDATORY — wayfinder SDLC, not raw code-first execution)

- Invoke /wayfinder and drive the bead through its phases (CHARTER -> ... -> RETRO).
  You are running on Opus specifically so the design/audit phases are rigorous —
  do not shortcut CHARTER/DESIGN/AUDIT to jump straight to code.
- Wayfinder artifacts (wf/, W0, design docs, audits, retros) are temporal: they go
  to the knowledge base (~/src/engram-research), NEVER committed into dear-agent.
- Work in ~/worktrees/dear-agent/%s/ (create the worktree from ~/src/dear-agent;
  ~/src is READ-ONLY).
- Commit incrementally after each sub-task — uncommitted work is nonexistent work.
- VERIFICATION GATE (MANDATORY — no ghost completions): Before writing 'done'
  in a bead note or stopping, run ≥1 verification step (go test ./...,
  make preflight, deploy-status check, or equivalent) and include the output.
  Code written but never run is NOT done.
- When the implementation phase is complete: open a PR via 'safe-pr create --wayfinder <wf-dir>'.
- If stuck after 2 retries on the same error: STOP, report failure with two concrete
  alternatives. Permission/access errors: 0 retries — report immediately.

## Bead closure (DoD — MANDATORY, do NOT skip)

- A bead is Done ONLY when its PR is MERGED to main. 'PR created' / 'PR open' /
  'PR approved' are NOT done.
- Before running 'bd ... close %s', you MUST verify the PR is merged:
    gh pr view <NNN> --repo vbonnet/dear-agent --json state,mergedAt
- If state is not MERGED (or mergedAt is null): do NOT close the bead. Add a bead
  note recording the block and leave the bead OPEN.
- Only close once mergedAt is non-null; put the merged PR reference in the close reason.

## Terminal status code (MANDATORY — the first token of your final bead note)

- When you stop working this bead, record exactly one outcome as the FIRST TOKEN
  of a bead note: DONE, DONE_WITH_CONCERNS, or FAILED.
- DONE — deliverable complete, no reservations.
- DONE_WITH_CONCERNS — deliverable complete, but you hold a reservation (a risky
  assumption, a shortcut taken, a test you could not run, a design tradeoff you
  are unsure about). Ship the work AND document the concern explicitly — what it
  is and why — so a supervisor can decide whether to act on it. Do NOT downgrade
  a completed bead to FAILED just because you have a doubt, and do NOT bury the
  doubt by reporting a bare DONE.
- FAILED — deliverable not complete; report the blocker and two concrete alternatives.

## Rules

- ALWAYS use `+"`bd --db ~/beads/context-engine/.beads`"+` (never bare bd)
- NEVER write to ~/src/** (read-only — use worktrees only)
- NEVER use --no-verify or --force
- NEVER run chezmoi apply
- ALWAYS use `+"`GIT_TERMINAL_PROMPT=0 gtimeout 30`"+` for git push
- Workers MUST use claude-opus-4-8, --mode=auto, --workspace=oss
- Do NOT run `+"`pkill -x gopls`"+`
- STOP after the primary deliverable is done — write a bead note and stop

Bead details: run bd --db ~/beads/context-engine/.beads show %s
`, b.ID, b.Title, b.ID, prio, summary, goal, b.ID, b.ID, b.ID)
}

// selectCandidates filters ready beads down to those eligible for direct
// dispatch: non-empty id, priority within the band, not human-gated, no live
// worker, not in flight in an open PR. The result is ordered by priority (P0
// first) then id, so the highest-priority work dispatches first when the capacity
// budget is tight.
//
// maxPriority is the numeric priority ceiling (0=P0 only, 1=P0+P1, 2=P0..P2):
// the orchestrator narrows this band as the Meta-Orchestrator heartbeat goes
// stale, so a silent roadmap owner restricts new work to the most critical tier
// rather than pouring speculative P2s into an unmonitored queue.
func selectCandidates(beads []bead, liveWorkers map[string]bool, prs []pullRequest, maxPriority int) []bead {
	var out []bead
	for _, b := range beads {
		if b.ID == "" {
			continue
		}
		if b.Priority > maxPriority {
			continue
		}
		if humanGated[b.ID] {
			continue
		}
		if liveWorkers[b.ID] {
			continue
		}
		if inFlightInPR(b.ID, prs) {
			continue
		}
		out = append(out, b)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority < out[j].Priority // P0 before P1 before P2
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// sessionNewArgs builds the `agm session new` argument list for spawning a worker.
// Model and mode are pinned (not left to agm defaults) for the same reason as the
// supervisors: the defaults are credit-gated 1M-context sonnet in non-executable
// plan mode (ce-84l2). Workers run detached in the oss workspace under the worker
// RBAC role.
func sessionNewArgs(name, model string) []string {
	return []string{
		"session", "new", name,
		"--detached", "--workspace=oss", "--harness=claude-code",
		"--model=" + model,
		"--mode=auto",
		"--role", workerRole,
	}
}

// spawnSession creates the detached worker session. Package var for test stubbing.
var spawnSession = func(ctx context.Context, name, model string) error {
	ctx, cancel := context.WithTimeout(ctx, subprocessTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "agm", sessionNewArgs(name, model)...)
	// Mark the spawned session tree as unattended so its own `agm send` calls
	// auto-stash stale input instead of deadlocking as if a human were typing
	// (ce-v9in), and scrub the API key so workers use the session's own auth.
	cmd.Env = append(scrubAPIKey(os.Environ()), "AGM_AUTONOMOUS=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// sendPrompt sends the rendered work prompt to a worker session. Package var for
// test stubbing.
var sendPrompt = func(ctx context.Context, name, prompt string) error {
	ctx, cancel := context.WithTimeout(ctx, subprocessTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "agm", "send", "msg", name,
		"--sender", dispatchSender,
		"--autonomous",
		"--prompt", prompt)
	cmd.Env = scrubAPIKey(os.Environ())
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// dispatch spawns a worker session for the bead and sends it the rendered prompt.
// If the spawn is refused (agm circuit breaker / at capacity), the error is
// returned and no prompt is sent — the bead is simply retried on a later run.
func dispatch(ctx context.Context, b bead, model string) error {
	name := "worker-" + b.ID
	if err := spawnSession(ctx, name, model); err != nil {
		return fmt.Errorf("spawn %s: %w", name, err)
	}
	if err := sendPrompt(ctx, name, renderPrompt(b)); err != nil {
		return fmt.Errorf("send to %s: %w", name, err)
	}
	return nil
}

func main() {
	db := flag.String("db", "~/beads/context-engine/.beads", "path to the beads database")
	repo := flag.String("repo", "vbonnet/dear-agent", "GitHub repo (owner/name) to check for open PRs")
	model := flag.String("model", defaultModel, "model alias workers spawn with (avoid bare opus/sonnet — credit-gated 1M)")
	maxPriority := flag.Int("max-priority", 2, "numeric priority ceiling: 0=P0 only, 1=P0+P1, 2=P0..P2 (orchestrator narrows this as Meta-O goes stale)")
	dryRun := flag.Bool("dry-run", false, "report what would be dispatched without spawning any sessions")
	flag.Parse()

	// Cancel in-flight subprocesses if the dispatcher is interrupted (SIGINT/
	// SIGTERM) so a killed run does not leave orphaned bd/gh/agm children.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	home, err := os.UserHomeDir()
	if err != nil {
		fatal("home dir: %v", err)
	}
	dbPath := expandHome(*db, home)

	beads, err := queryReady(ctx, dbPath)
	if err != nil {
		fatal("query ready beads: %v", err)
	}

	// Fail closed on a session-list failure: without it we cannot tell which
	// beads already have a live worker and must not risk double-dispatching.
	sessions, err := listSessions(ctx)
	if err != nil {
		fatal("list sessions (failing closed to avoid double-dispatch): %v", err)
	}
	live := liveWorkerIDs(sessions)

	// Fail closed on a PR-list failure for the same reason.
	prs, err := queryOpenPRs(ctx, *repo)
	if err != nil {
		fatal("query open PRs (failing closed to avoid double-dispatch): %v", err)
	}

	candidates := selectCandidates(beads, live, prs, *maxPriority)

	dispatched := dispatchCandidates(ctx, candidates, *model, *dryRun, os.Stdout, os.Stderr)

	fmt.Fprintf(os.Stderr,
		"vroom-dispatch-direct: %d ready, %d live worker(s), %d eligible, %d dispatched\n",
		len(beads), len(live), len(candidates), dispatched)
}

func dispatchCandidates(ctx context.Context, candidates []bead, model string, dryRun bool, out, errOut io.Writer) int {
	dispatched := 0
	for _, b := range candidates {
		if ctx.Err() != nil {
			break
		}
		if dryRun {
			fmt.Fprintf(out, "would dispatch worker-%s (%s) %s\n", b.ID, priorityLabel(b.Priority), b.Title)
			dispatched++
			continue
		}
		if err := dispatch(ctx, b, model); err != nil {
			// A refused spawn is expected backpressure, not a fatal error: log
			// and stop this run (capacity is likely exhausted), retry next run.
			fmt.Fprintf(errOut, "vroom-dispatch-direct: dispatch %s: %v\n", b.ID, err)
			break
		}
		fmt.Fprintf(out, "dispatched worker-%s (%s) %s\n", b.ID, priorityLabel(b.Priority), b.Title)
		dispatched++
	}
	return dispatched
}

// expandHome replaces a leading ~ in a path with the user's home directory.
func expandHome(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return home + path[1:]
	}
	return path
}

// scrubAPIKey removes ANTHROPIC_API_KEY from an environment slice so spawned
// sessions fall back to their own OAuth rather than inheriting a raw key.
func scrubAPIKey(env []string) []string {
	const prefix = "ANTHROPIC_API_KEY="
	out := make([]string, 0, len(env))
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			continue
		}
		out = append(out, e)
	}
	return out
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "vroom-dispatch-direct: "+format+"\n", args...)
	os.Exit(1)
}
