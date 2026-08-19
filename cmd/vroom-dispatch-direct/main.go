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
// derived from ground truth — occupied non-archived `worker-<id>` names and open
// PRs — not from a directory of files that can drift.
//
// Deduplication is layered so re-running is idempotent and never double-dispatches:
//  1. occupied worker names (a non-archived `worker-<id>` session exists)
//  2. open PRs (the bead is already in flight — its id appears in a PR branch/title)
//  3. the human-gated skip list (beads a human must drive, never an autonomous worker)
//
// Capacity is controlled by real spawn backpressure: it dispatches eligible
// beads until the list is exhausted or a spawn is refused. -max-dispatch opts
// into a per-run cap on top of that (see cap.go).
//
// Exit status is 0 on success even when zero beads are dispatched — "nothing new
// to dispatch" (backlog drained, at capacity, or all in flight) is a normal
// steady state, not an error.
//
// The three ground-truth queries (bd ready, agm session list, gh pr list) fail
// closed: any error there aborts the run rather than risk double-dispatching.
// That's correct for the individual tick, but the dispatch loop wrapper reruns
// this binary every INTERVAL forever, so a transient failure (a gh token
// mid-rotation, a momentary dolt lock) can silently stall the whole flywheel
// tick after tick while the loop still looks "alive" in its log. See
// degraded.go: each query is retried before it's treated as fatal, and a
// persisted failure streak (~/.agm/vroom/heartbeat/dispatch-direct.json,
// overridable via -heartbeat-file) escalates past a threshold — a loud stderr
// banner plus a best-effort desktop notification — instead of degrading
// invisibly (incident: 2026-07-20, gh auth rotation, ~20 minutes of dead
// ticks discoverable only by reading dispatch-loop.log line by line).
//
// Bead closure (ce-2n5j / ce-24f1): the worker prompt asks a worker to leave a
// terminal-status note, but nothing ever made that deterministic — a worker
// that finished (or crashed, or hit a permission block) without closing its
// own bead left it `open` and `ready`, so the next run's `bd ready` saw it
// again and redispatched a fresh worker to re-derive the same answer,
// forever. Every run now reconciles beads it previously dispatched a worker
// for (see reconcile.go): a merged PR closes the bead as done, an explicit
// no-op/failure note closes or blocks it, and repeated silent exits with no
// evidence of progress auto-block the bead instead of allowing infinite
// redispatch. Enforcement never depends on the worker having run any bd
// command beyond the note the prompt already requires.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/vbonnet/dear-agent/internal/vroomgate"
)

// subprocessTimeout bounds every bd/gh/agm child so a hung subprocess cannot
// stall a dispatch run indefinitely. The styleguide requires subprocess calls to
// be both context-propagated and timeout-bounded; each runner derives a deadline
// from the caller's ctx so SIGINT/SIGTERM cancellation still composes with it.
const subprocessTimeout = 60 * time.Second

// sessionSpawnTimeout bounds the `agm session new` call specifically. Unlike
// the quick bd/gh/agm-list calls under subprocessTimeout, a spawn boots the
// whole worker harness (tmux session, harness startup, workspace checks)
// and legitimately runs past 60s; killing it mid-boot leaves a half-created
// session AND reads as a spawn failure that stops the run. 180s is generous
// for a healthy boot while still bounding a truly hung spawn.
const sessionSpawnTimeout = 180 * time.Second

// Worker spawn defaults. opus-200k → claude-opus-4-8: design-phase work needs
// Opus, and the 200k variant dodges the 1M credit gate the bare opus/sonnet
// aliases trip on this Max-plan auth (ce-84l2). The human-gated skip list now
// lives in internal/vroomgate so it cannot drift between dispatch binaries.
const (
	defaultHarness   = "claude-code"
	defaultModel     = "opus-200k"
	defaultMode      = "auto"
	defaultWorkspace = "oss"
)

// workerRole is the RBAC profile workers spawn with.
const workerRole = "worker"

// dispatchSender is the --sender label on dispatch messages, so the trail
// attributes them to this tool rather than a human.
const dispatchSender = "vroom-orchestrator"

// workerLaunchConfig is the harness extension boundary for direct dispatch.
// Candidate selection and the work contract are harness-neutral; operators may
// select any AGM-supported harness/model/mode without forking the dispatcher.
type workerLaunchConfig struct {
	Harness    string
	Model      string
	Mode       string
	Workspace  string
	AddDirs    []string
	BeadsDir   string
	Worktrees  string
	GitState   string
	EngramRepo string
	GuardPath  string
}

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
	MergedAt    string `json:"mergedAt"`
}

// queryReady runs `bd --db <db> ready --json` and returns the ready beads. It is
// a package var so tests can stub the bd invocation without a real database.
var queryReady = func(ctx context.Context, db string) ([]bead, error) {
	ctx, cancel := context.WithTimeout(ctx, subprocessTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bd", "--db", db, "ready", "--json")
	out, err := cmd.Output()
	if err != nil {
		return nil, wrapExecErr("bd ready", err)
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
		return nil, wrapExecErr("gh pr list", err)
	}
	var prs []pullRequest
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, fmt.Errorf("parse gh pr list output: %w", err)
	}
	return prs, nil
}

// queryMergedPRs runs `gh pr list --state merged` for the repo. This is the
// ground-truth signal reconcile uses to close a bead as done: a merged PR
// mentioning the bead id is verified independently of anything a worker wrote
// in a bead note, so a worker that forgets (or is killed before) reporting its
// outcome still gets its bead closed once the PR is merged. It is a package
// var so tests can stub the gh invocation.
var queryMergedPRs = func(ctx context.Context, repo string) ([]pullRequest, error) {
	ctx, cancel := context.WithTimeout(ctx, subprocessTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "gh", "pr", "list",
		"--repo", repo,
		"--state", "merged",
		"--limit", "200",
		"--json", "number,headRefName,title,mergedAt")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh pr list --state merged: %w", err)
	}
	var prs []pullRequest
	if err := json.Unmarshal(out, &prs); err != nil {
		return nil, fmt.Errorf("parse gh pr list --state merged output: %w", err)
	}
	return prs, nil
}

const sessionListPageSize = 1000

type sessionNameStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type sessionListPayload struct {
	Sessions []sessionNameStatus `json:"sessions"`
}

// listSessionPage reads one explicit page. The CLI caps a page at 1,000 rows.
var listSessionPage = func(ctx context.Context, offset int) (sessionListPayload, error) {
	ctx, cancel := context.WithTimeout(ctx, subprocessTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "agm", sessionListArgs(offset)...)
	out, err := cmd.Output()
	if err != nil {
		return sessionListPayload{}, fmt.Errorf("agm session list: %w", err)
	}
	var payload sessionListPayload
	if err := json.Unmarshal(out, &payload); err != nil {
		return sessionListPayload{}, fmt.Errorf("parse agm session list: %w", err)
	}
	return payload, nil
}

func sessionListArgs(offset int) []string {
	return []string{"session", "list", "--all", "--stable-order", "--output", "json",
		"--limit", fmt.Sprint(sessionListPageSize), "--offset", fmt.Sprint(offset)}
}

// listSessions retrieves every page before candidate selection. Archived rows
// are included so status can release those names while retaining stopped and
// zombie non-archived names. A partial inventory would make dedup fail open.
func listSessions(ctx context.Context) ([]string, error) {
	all := sessionListPayload{Sessions: []sessionNameStatus{}}
	for offset := 0; ; offset += sessionListPageSize {
		page, err := listSessionPage(ctx, offset)
		if err != nil {
			return nil, err
		}
		all.Sessions = append(all.Sessions, page.Sessions...)
		if len(page.Sessions) < sessionListPageSize {
			break
		}
	}
	data, err := json.Marshal(all)
	if err != nil {
		return nil, fmt.Errorf("encode complete agm session list: %w", err)
	}
	return []string{string(data)}, nil
}

// normalizeSessionID maps a bead id to its tmux-safe form: dots, colons and
// spaces become dashes. This mirrors agm's tmux.NormalizeTmuxSessionName
// (agm/internal/tmux — not importable from cmd/): tmux itself performs the same
// conversion on session creation, and agm's tmux-safety check falls into an
// INTERACTIVE prompt when it sees an unsafe name, which is fatal in a
// detached/no-TTY dispatch context ("could not open a new TTY", ce-b1zw).
// Sanitizing up front means agm never prompts — and dedup must then normalize
// BOTH sides of the comparison, so a live session worker-ce-6as-10 dedups bead
// ce-6as.10 and a legacy dotted session worker-ce-6as.10 dedups it too.
func normalizeSessionID(id string) string {
	id = strings.ReplaceAll(id, ".", "-")
	id = strings.ReplaceAll(id, ":", "-")
	id = strings.ReplaceAll(id, " ", "-")
	return id
}

// workerSessionName returns the tmux-safe session name a bead's worker spawns
// under. All session-name construction goes through here so spawn, send, and
// dedup can never disagree about a bead's session.
func workerSessionName(id string) string {
	return "worker-" + normalizeSessionID(id)
}

// occupiedWorkerIDs scans `agm session list` output for `worker-<id>` session names
// and returns the set of NORMALIZED bead ids whose non-archived session name is
// occupied. Zombie workers remain in the set because AGM rejects a new session
// with the same non-archived name; excluding them would make every dispatch
// retry fail before later candidates are considered.
// This replaces vroom-prompt-gen's "already has a prompt file" proxy with the
// session-name ownership invariant enforced by AGM. Ids are normalized
// (dots→dashes) so lookups with normalizeSessionID match both sanitized session
// names and legacy dotted ones.
func occupiedWorkerIDs(lines []string) map[string]bool {
	ids := make(map[string]bool)

	// `agm session list` defaults to agent-mode JSON in the non-TTY dispatch
	// context. Parse it structurally and read ONLY the session name field, so a
	// "worker-" substring in some other field (e.g. "harness":"worker-harness")
	// can never be misread as a live worker id. Fall back to the legacy text
	// regex when the output is not the expected JSON object.
	joined := strings.TrimSpace(strings.Join(lines, "\n"))
	if strings.HasPrefix(joined, "{") {
		var payload sessionListPayload
		if err := json.Unmarshal([]byte(joined), &payload); err == nil {
			for _, s := range payload.Sessions {
				if !workerStatusOccupiesName(s.Status) {
					continue
				}
				// Exact prefix on the name field: "subworker-x"/"my-worker-x"
				// do not start with "worker-", so they are excluded naturally.
				if rest, ok := strings.CutPrefix(s.Name, "worker-"); ok {
					ids[normalizeSessionID(rest)] = true
				}
			}
			return ids
		}
		// Unmarshal failed: fall through to the text regex (defensive).
	}

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 || !workerStatusOccupiesName(fields[1]) {
			continue
		}
		if rest, ok := strings.CutPrefix(fields[0], "worker-"); ok {
			ids[normalizeSessionID(rest)] = true
		}
	}
	return ids
}

func workerStatusOccupiesName(value string) bool {
	return strings.EqualFold(value, "active") ||
		strings.EqualFold(value, "running") ||
		strings.EqualFold(value, "zombie") ||
		strings.EqualFold(value, "stopped")
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

// renderPrompt produces the harness-neutral work contract sent to a worker.
// Harness/model selection belongs to workerLaunchConfig, not prompt prose.
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

**Own this bead through every applicable delivery gate. PR creation is not done.**

## Goal

%s

## Process

- Use the canonical Wayfinder V2 lifecycle: CHARTER, PROBLEM, RESEARCH, DESIGN,
  SPEC, PLAN, SETUP, BUILD, RETRO. Use the installed command or the harness-native
  Wayfinder skill; never activate a retired lifecycle.
- Wayfinder, audit, plan, design-exploration, and retrospective artifacts are
  temporal. Store them in an engram-research worktree, never in dear-agent.
- Work in the dispatcher-prepared ~/worktrees/dear-agent/%s/ linked worktree.
  Use it as-is; do not run `+"`git worktree add`"+`. ~/src is READ-ONLY.
- Use that worktree's absolute path for apply_patch/Edit/Write targets and use
  `+"`git -C <worktree>`"+` for Git commands. Relative edits from ~/src are blocked.
- Store temporal artifacts in the dispatcher-prepared
  ~/worktrees/engram-research/%s/ linked worktree.
- Commit incrementally after each sub-task — uncommitted work is nonexistent work.
- Open the PR with: safe-pr create --wayfinder <wf-dir>. Address CI and automated
  review feedback. Use only the repository's safe rebase/merge wrappers.
- If stuck after 2 retries on the same error: STOP, report failure with two concrete
  alternatives. Permission/access errors: 0 retries — report immediately.

## Delivery gates

Before closing %s, record evidence for every applicable gate:

1. MERGED — gh pr view <NNN> --repo vbonnet/dear-agent --json state,mergedAt
   reports MERGED and non-null mergedAt.
2. DEPLOYED — if a deployable artifact changed, install it from the merged
   revision and prove its status is clean. Otherwise record "deployment: N/A"
   with the reason.
3. VERIFIED — run relevant source checks and, for runtime changes, exercise the
   installed behavior locally. Include commands and results in the bead note.

If any gate is incomplete, leave the bead open. Human-owned product, security,
money, legal, destructive, merge, or deployment decisions remain blocked until
the human acts.

## Terminal status code (MANDATORY — the first token of your final bead note)

- When you stop working this bead, record exactly one outcome as the FIRST TOKEN
  of a bead note: DONE, DONE_WITH_CONCERNS, or FAILED.
- DONE — all applicable delivery gates passed, no reservations.
- DONE_WITH_CONCERNS — all delivery gates passed, but you hold a reservation (a risky
  assumption, a shortcut taken, a test you could not run, a design tradeoff you
  are unsure about). Ship the work AND document the concern explicitly — what it
  is and why — so a supervisor can decide whether to act on it. Do NOT downgrade
  a completed bead to FAILED just because you have a doubt, and do NOT bury the
  doubt by reporting a bare DONE.
- FAILED — deliverable not complete; report the blocker and two concrete alternatives.

This note is the authoritative signal the dispatcher reconciles on — it, not
you, closes DONE/DONE_WITH_CONCERNS beads and blocks FAILED ones on its next
run. You do not need to (and should not rely on remembering to) close the bead
yourself; write the note and stop.

## Rules

- ALWAYS use `+"`bd --db ~/beads/context-engine/.beads --dolt-auto-commit on`"+` (never bare bd)
- NEVER write to ~/src/** (read-only — use worktrees only)
- NEVER use --no-verify or --force
- NEVER use raw git push or gh pr merge; push with
  `+"`safe-push -C <worktree> origin <branch>`"+` (without `+"`-u`"+`), then use
  the safe PR/merge wrappers
- NEVER run chezmoi apply
- Do NOT run `+"`pkill -x gopls`"+`
- Do NOT create unrelated beads or broaden scope

Bead details: run bd --db ~/beads/context-engine/.beads --dolt-auto-commit on show %s
`, b.ID, b.Title, b.ID, prio, summary, goal, b.ID, b.ID, b.ID, b.ID)
}

// selectCandidates filters ready beads down to those eligible for direct
// dispatch: non-empty id, priority within the band, not human-gated, no live
// worker, not in flight in an open PR. The result is ordered by priority (P0
// first) then id, so the highest-priority work dispatches first when the capacity
// budget is tight.
//
// maxPriority is the numeric priority ceiling (0=P0 only, 1=P0+P1, 2=P0..P2):
// the orchestrator narrows this band as the Meta-Orchestrator heartbeat goes
// stale, so a silent coordination owner restricts new work to the most critical tier
// rather than pouring speculative P2s into an unmonitored queue.
func selectCandidates(beads []bead, occupiedWorkers map[string]bool, prs []pullRequest, maxPriority int) []bead {
	var out []bead
	for _, b := range beads {
		if b.ID == "" {
			continue
		}
		if b.Priority > maxPriority {
			continue
		}
		if vroomgate.IsHumanGated(b.ID) {
			continue
		}
		// Worker-name dedup compares normalized ids: occupiedWorkerIDs normalizes
		// what it reads from `agm session list`, and the bead id is normalized
		// here, so the match holds whether the occupied name was sanitized or
		// the sanitized name or a legacy dotted one (ce-b1zw).
		if occupiedWorkers[normalizeSessionID(b.ID)] {
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

var safeBeadID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// prepareWorkerWorkspace creates task-owned satellite repositories and their
// linked worktrees on the trusted host before Codex starts. A linked worktree
// backed by the source repository cannot update a branch without Git also
// attempting to lock the source repository's packed-refs file. The satellite
// keeps that transaction entirely inside task-owned Git state while borrowing
// immutable base objects from the read-only source repository via alternates.
func prepareWorkerWorkspace(ctx context.Context, beadID string, cfg workerLaunchConfig, repoDir string) ([]string, error) {
	if !safeBeadID.MatchString(beadID) {
		return nil, fmt.Errorf("unsafe bead id %q for worker workspace", beadID)
	}
	if cfg.Worktrees == "" || cfg.GitState == "" || cfg.BeadsDir == "" {
		return nil, errors.New("worker worktree base, Git state base, and Beads directory are required")
	}

	type worktreeSpec struct {
		repo     string
		path     string
		gitState string
		branch   string
	}
	normalized := normalizeSessionID(beadID)
	specs := []worktreeSpec{{
		repo:     repoDir,
		path:     filepath.Join(cfg.Worktrees, filepath.Base(repoDir), beadID),
		gitState: filepath.Join(cfg.GitState, filepath.Base(repoDir), beadID+".git"),
		branch:   "workers/" + normalized + "/change",
	}}
	if cfg.EngramRepo != "" {
		specs = append(specs, worktreeSpec{
			repo:     cfg.EngramRepo,
			path:     filepath.Join(cfg.Worktrees, filepath.Base(cfg.EngramRepo), beadID),
			gitState: filepath.Join(cfg.GitState, filepath.Base(cfg.EngramRepo), beadID+".git"),
			branch:   "workers/" + normalized + "/temporal",
		})
	}

	addDirs := []string{cfg.BeadsDir}
	for _, spec := range specs {
		if err := ensureWorkerWorktree(ctx, spec.repo, spec.gitState, spec.path, spec.branch); err != nil {
			return nil, err
		}
		addDirs = appendUniqueStrings(addDirs, spec.path)
		addDirs = appendUniqueStrings(addDirs, spec.gitState)
	}
	return addDirs, nil
}

func ensureWorkerWorktree(ctx context.Context, repoDir, gitState, target, branch string) error {
	startPoint := "origin/main"
	if _, err := gitOutput(ctx, repoDir, "rev-parse", "--verify", startPoint); err != nil {
		startPoint = "HEAD"
	}
	baseOID, err := gitOutput(ctx, repoDir, "rev-parse", "--verify", startPoint+"^{commit}")
	if err != nil {
		return fmt.Errorf("resolve worker base commit in %s: %w", repoDir, err)
	}
	if err := ensureWorkerGitState(ctx, repoDir, gitState, baseOID); err != nil {
		return err
	}

	exists, err := validateExistingWorkerWorktree(ctx, target, branch, gitState)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create worker worktree parent: %w", err)
	}
	if _, err := gitOutput(ctx, gitState, "show-ref", "--verify", "refs/heads/"+branch); err == nil {
		if _, err := gitOutput(ctx, gitState, "worktree", "add", target, branch); err != nil {
			return fmt.Errorf("attach existing worker branch %s: %w", branch, err)
		}
	} else if _, err := gitOutput(ctx, gitState, "worktree", "add", target, "-b", branch, "refs/remotes/origin/main"); err != nil {
		return fmt.Errorf("create worker worktree %s: %w", target, err)
	}
	return nil
}

func validateExistingWorkerWorktree(ctx context.Context, target, branch, gitState string) (bool, error) {
	info, err := os.Stat(target)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect worker worktree target %s: %w", target, err)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("worker worktree target is not a directory: %s", target)
	}
	got, err := gitOutput(ctx, target, "branch", "--show-current")
	if err != nil {
		return false, fmt.Errorf("inspect existing worker worktree %s: %w", target, err)
	}
	if got != branch {
		return false, fmt.Errorf("existing worker worktree %s is on %q, want %q", target, got, branch)
	}
	gotCommon, err := gitOutput(ctx, target, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return false, fmt.Errorf("inspect existing worker Git state %s: %w", target, err)
	}
	same, err := sameResolvedPath(gotCommon, gitState)
	if err != nil {
		return false, fmt.Errorf("compare existing worker Git state for %s: %w", target, err)
	}
	if !same {
		return false, fmt.Errorf("existing worker worktree %s uses Git state %q, want %q", target, gotCommon, gitState)
	}
	return true, nil
}

func sameResolvedPath(left, right string) (bool, error) {
	resolvedLeft, err := filepath.EvalSymlinks(left)
	if err != nil {
		return false, err
	}
	resolvedRight, err := filepath.EvalSymlinks(right)
	if err != nil {
		return false, err
	}
	return filepath.Clean(resolvedLeft) == filepath.Clean(resolvedRight), nil
}

func ensureWorkerGitState(ctx context.Context, repoDir, gitState, baseOID string) error {
	commonDir, err := gitOutput(ctx, repoDir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return fmt.Errorf("resolve common Git directory for %s: %w", repoDir, err)
	}
	objectsDir := filepath.Join(commonDir, "objects")
	originURL, err := gitOutput(ctx, repoDir, "remote", "get-url", "origin")
	if err != nil {
		return fmt.Errorf("resolve origin URL for %s: %w", repoDir, err)
	}
	userName, err := gitOutput(ctx, repoDir, "config", "--get", "user.name")
	if err != nil {
		return fmt.Errorf("resolve Git user.name for %s: %w", repoDir, err)
	}
	userEmail, err := gitOutput(ctx, repoDir, "config", "--get", "user.email")
	if err != nil {
		return fmt.Errorf("resolve Git user.email for %s: %w", repoDir, err)
	}
	if err := ensureBareWorkerGitState(ctx, gitState); err != nil {
		return err
	}
	if err := ensureWorkerObjectAlternates(gitState, objectsDir); err != nil {
		return err
	}
	return configureWorkerGitState(ctx, gitState, originURL, userName, userEmail, baseOID)
}

func ensureBareWorkerGitState(ctx context.Context, gitState string) error {
	info, err := os.Stat(gitState)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(gitState), 0o755); err != nil {
			return fmt.Errorf("create worker Git state parent: %w", err)
		}
		if _, err := gitCommandOutput(ctx, "init", "--bare", gitState); err != nil {
			return fmt.Errorf("initialize worker Git state %s: %w", gitState, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect worker Git state %s: %w", gitState, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("worker Git state is not a directory: %s", gitState)
	}
	bare, bareErr := gitOutput(ctx, gitState, "rev-parse", "--is-bare-repository")
	if bareErr != nil || bare != "true" {
		return fmt.Errorf("existing worker Git state %s is not a bare repository", gitState)
	}
	return nil
}

func ensureWorkerObjectAlternates(gitState, objectsDir string) error {
	alternates := filepath.Join(gitState, "objects", "info", "alternates")
	wantAlternates := []byte(objectsDir + "\n")
	if got, readErr := os.ReadFile(alternates); readErr == nil {
		if !bytes.Equal(got, wantAlternates) {
			return fmt.Errorf("worker Git state %s borrows objects from %q, want %q", gitState, strings.TrimSpace(string(got)), objectsDir)
		}
	} else if os.IsNotExist(readErr) {
		if writeErr := os.WriteFile(alternates, wantAlternates, 0o600); writeErr != nil {
			return fmt.Errorf("configure worker object alternates: %w", writeErr)
		}
	} else {
		return fmt.Errorf("read worker object alternates: %w", readErr)
	}
	return nil
}

func configureWorkerGitState(ctx context.Context, gitState, originURL, userName, userEmail, baseOID string) error {
	commands := [][]string{
		{"config", "remote.origin.url", originURL},
		{"config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*"},
		{"config", "user.name", userName},
		{"config", "user.email", userEmail},
		{"update-ref", "refs/remotes/origin/main", baseOID},
		{"symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main"},
	}
	for _, args := range commands {
		if _, err := gitOutput(ctx, gitState, args...); err != nil {
			return fmt.Errorf("configure worker Git state %s: %w", gitState, err)
		}
	}
	return nil
}

func gitCommandOutput(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func gitOutput(ctx context.Context, repoDir string, args ...string) (string, error) {
	cmdArgs := append([]string{"-C", repoDir}, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(cmdArgs, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func appendUniqueStrings(values []string, value string) []string {
	if slices.Contains(values, value) {
		return values
	}
	return append(values, value)
}

// sessionNewArgs builds the AGM launch request. The defaults preserve current
// Claude operation, while explicit flags provide the non-Claude path.
//
// repoDir is passed as an explicit --directory so `agm session new`'s
// getWorkDir never falls back to $PWD/os.Getwd(). Under launchd
// (WorkingDirectory=/Users/vbonnet, no $PWD), that fallback resolved to
// $HOME, which made resolveSandboxLowerDirs clone the entire home directory
// — the root cause of the spawn-hang fixed alongside this (ce-fmxv). A
// launchd-driven spawn must never depend on inherited cwd.
func sessionNewArgs(name string, cfg workerLaunchConfig, repoDir string) []string {
	args := []string{
		"session", "new", name,
		"--detached", "--workspace=" + cfg.Workspace, "--harness=" + cfg.Harness,
		"--model=" + cfg.Model,
		"--role", workerRole,
		"--directory", repoDir,
	}
	if cfg.Mode != "" {
		args = append(args, "--mode="+cfg.Mode)
	}
	return args
}

// spawnSession creates the detached worker session. Package var for test stubbing.
// Uses sessionSpawnTimeout (not the blanket subprocessTimeout): harness boot
// legitimately exceeds 60s — see the const's comment.
var spawnSession = func(ctx context.Context, name string, cfg workerLaunchConfig, repoDir string) error {
	ctx, cancel := context.WithTimeout(ctx, sessionSpawnTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "agm", sessionNewArgs(name, cfg, repoDir)...)
	// Mark the spawned session tree as unattended so its own `agm send` calls
	// auto-stash stale input instead of deadlocking as if a human were typing
	// (ce-v9in), and scrub the API key so workers use the session's own auth.
	cmd.Env = append(scrubAPIKey(os.Environ()), "AGM_AUTONOMOUS=1")
	guardPath := ""
	if cfg.Harness == "codex-cli" {
		guardPath = cfg.GuardPath
	}
	handoffEnv, err := trustedAddDirsEnvironment(name, cfg.AddDirs, guardPath)
	if err != nil {
		return err
	}
	cmd.Env = append(cmd.Env, handoffEnv...)
	// Process-group isolation + group-cancel + WaitDelay matches the repo's
	// subprocess-execution convention (see codexcontrol.Client): `agm session
	// new` boots a harness that can itself wedge a descendant holding a pipe
	// open (this is the exact ce-fmxv failure mode this PR fixes elsewhere in
	// the codex boot path); without group-kill on cancel that descendant can
	// outlive the timeout and keep the worker session half-alive.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 1 * time.Second
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func trustedAddDirsEnvironment(sessionName string, dirs []string, guardPath string) ([]string, error) {
	if len(dirs) == 0 {
		return nil, nil
	}
	if guardPath != "" && !filepath.IsAbs(guardPath) {
		return nil, fmt.Errorf("trusted worker guard path must be absolute: %q", guardPath)
	}
	payload, err := json.Marshal(dirs)
	if err != nil {
		return nil, fmt.Errorf("encode trusted add-dir handoff: %w", err)
	}
	env := []string{
		"AGM_TRUSTED_ADD_DIRS_JSON=" + string(payload),
		"AGM_TRUSTED_ADD_DIRS_SESSION=" + sessionName,
	}
	if guardPath != "" {
		env = append(env, "AGM_TRUSTED_GUARD_PATH="+guardPath)
	}
	return env, nil
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

// deterministicSpawnFailures are error signatures that identify a spawn failure
// caused by THIS bead (unsafe/invalid session name, invalid bead, agm falling
// into an interactive prompt with no TTY) rather than by system state. Such
// failures recur identically on every retry, so aborting the run on them lets
// one poisoned bead stall ALL dispatch every tick (ce-b1zw: 0/17 dispatched).
// Matching is positive-signature only: anything unrecognized stays fail-closed.
var deterministicSpawnFailures = []string{
	"could not open a new tty", // agm hit an interactive prompt in a detached/no-TTY context
	"invalid session name",
	"unsafe characters", // agm tmux-safety rejection
	"invalid bead",
}

// skipBeadError marks a deterministic per-bead spawn failure: the dispatcher
// should log it, skip the bead, and keep working the rest of the candidate list.
type skipBeadError struct{ err error }

func (e *skipBeadError) Error() string { return e.err.Error() }
func (e *skipBeadError) Unwrap() error { return e.err }

// isDeterministicSpawnFailure classifies a spawn error: true means a per-bead
// deterministic failure (skip the bead, continue the run); false means
// backpressure, timeout, or unknown (stop the run — the original fail-closed
// behavior, still correct for systemic conditions). Backpressure and
// context-deadline errors are checked FIRST so they can never be misread as
// per-bead even if their text happens to contain a known signature.
func isDeterministicSpawnFailure(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "circuit breaker") || strings.Contains(msg, "spawn refused") {
		return false
	}
	for _, sig := range deterministicSpawnFailures {
		if strings.Contains(msg, sig) {
			return true
		}
	}
	return false
}

// dispatch spawns a worker session for the bead and sends it the rendered prompt.
// The session name is the tmux-safe form of the bead id so agm's tmux-safety
// check never prompts (ce-b1zw); the prompt itself keeps the real bead id.
// A deterministic per-bead spawn failure is returned wrapped in *skipBeadError
// so the caller can skip the bead and continue; any other failure (circuit
// breaker / at capacity / timeout / unknown) is returned as-is and no prompt is
// sent — the bead is simply retried on a later run.
func dispatch(ctx context.Context, b bead, cfg workerLaunchConfig, repoDir string) error {
	name := workerSessionName(b.ID)
	if cfg.Worktrees != "" {
		addDirs, err := prepareWorkerWorkspace(ctx, b.ID, cfg, repoDir)
		if err != nil {
			return &skipBeadError{err: fmt.Errorf("prepare workspace for %s: %w", name, err)}
		}
		cfg.AddDirs = addDirs
		repoDir = filepath.Join(cfg.Worktrees, filepath.Base(repoDir), b.ID)
	}
	if err := spawnSession(ctx, name, cfg, repoDir); err != nil {
		wrapped := fmt.Errorf("spawn %s: %w", name, err)
		if isDeterministicSpawnFailure(err) {
			return &skipBeadError{err: wrapped}
		}
		return wrapped
	}
	if err := sendPrompt(ctx, name, renderPrompt(b)); err != nil {
		return fmt.Errorf("send to %s: %w", name, err)
	}
	return nil
}

func main() {
	db := flag.String("db", "~/beads/context-engine/.beads", "path to the beads database")
	repo := flag.String("repo", "vbonnet/dear-agent", "GitHub repo (owner/name) to check for open PRs")
	harness := flag.String("harness", defaultHarness, "AGM harness for workers")
	model := flag.String("model", defaultModel, "model alias for the selected harness")
	mode := flag.String("mode", defaultMode, "AGM permission mode; empty omits the flag")
	workspace := flag.String("workspace", defaultWorkspace, "AGM workspace for workers")
	repoDir := flag.String("repo-dir", "~/src/dear-agent", "local checkout passed as `agm session new --directory` so a launchd-context spawn never inherits launchd's cwd (ce-fmxv)")
	engramRepoDir := flag.String("engram-repo-dir", "~/src/engram-research", "engram-research source checkout used to pre-create each worker's temporal worktree")
	worktreeBase := flag.String("worktree-base", "~/worktrees", "base directory for host-prepared worker worktrees")
	gitStateBase := flag.String("worker-git-base", "~/.agm/worker-git", "base directory for task-owned satellite Git metadata")
	workerGuard := flag.String("worker-guard", "/etc/codex/hooks/pretool-worker-write-boundary", "system-managed Codex worker apply_patch guard")
	prepareWorker := flag.String("prepare-worker", "", "pre-create one bead's worker workspaces and print its trusted add-directory handoff as JSON, without dispatching")
	maxPriority := flag.Int("max-priority", 2, "numeric priority ceiling: 0=P0 only, 1=P0+P1, 2=P0..P2 (orchestrator narrows this as Meta-O goes stale)")
	maxDispatch := flag.Int("max-dispatch", 0, "cap on beads dispatched in this run (0 = unlimited); bounds blast radius for an unattended/scheduled run")
	dryRun := flag.Bool("dry-run", false, "report what would be dispatched without spawning any sessions")
	heartbeatFile := flag.String("heartbeat-file", "~/.agm/vroom/heartbeat/dispatch-direct.json", "path to the persisted control-plane health state (consecutive fail-closed streak, last error)")
	ledgerPath := flag.String("ledger", "~/.agm/vroom/dispatch-ledger.json", "path to the dispatch ledger used to reconcile bead closure across runs")
	flag.Parse()

	if err := validateMaxDispatch(*maxDispatch); err != nil {
		fatal("%v", err)
	}

	// Cancel in-flight subprocesses if the dispatcher is interrupted (SIGINT/
	// SIGTERM) so a killed run does not leave orphaned bd/gh/agm children.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	home, err := os.UserHomeDir()
	if err != nil {
		fatal("home dir: %v", err)
	}
	dbPath := expandHome(*db, home)
	repoDirPath := expandHome(*repoDir, home)
	engramRepoDirPath := expandHome(*engramRepoDir, home)
	worktreeBasePath := expandHome(*worktreeBase, home)
	gitStateBasePath := expandHome(*gitStateBase, home)
	workerGuardPath := expandHome(*workerGuard, home)
	statePath := expandHome(*heartbeatFile, home)
	launch := workerLaunchConfig{
		Harness: *harness, Model: *model, Mode: *mode, Workspace: *workspace,
		BeadsDir: dbPath, Worktrees: worktreeBasePath, GitState: gitStateBasePath, EngramRepo: engramRepoDirPath, GuardPath: workerGuardPath,
	}
	if *prepareWorker != "" {
		dirs, err := prepareWorkerWorkspace(ctx, *prepareWorker, launch, repoDirPath)
		if err != nil {
			fatal("prepare worker %s: %v", *prepareWorker, err)
		}
		payload := struct {
			Session   string   `json:"session"`
			AddDirs   []string `json:"add_dirs"`
			Directory string   `json:"directory"`
			GuardPath string   `json:"guard_path,omitempty"`
		}{
			Session:   workerSessionName(*prepareWorker),
			AddDirs:   dirs,
			Directory: filepath.Join(launch.Worktrees, filepath.Base(repoDirPath), *prepareWorker),
		}
		if launch.Harness == "codex-cli" {
			payload.GuardPath = launch.GuardPath
		}
		if err := json.NewEncoder(os.Stdout).Encode(payload); err != nil {
			fatal("encode prepared worker handoff: %v", err)
		}
		return
	}

	var beads []bead
	if err := withRetry(ctx, func() error {
		var qErr error
		beads, qErr = queryReady(ctx, dbPath)
		return qErr
	}); err != nil {
		wrapped := fmt.Errorf("query ready beads: %w", err)
		recordFailure(ctx, statePath, wrapped)
		fatal("%v", wrapped)
	}

	// Fail closed on a session-list failure: without it we cannot tell which
	// worker names are already occupied and must not risk a rejected duplicate.
	var sessions []string
	if err := withRetry(ctx, func() error {
		var qErr error
		sessions, qErr = listSessions(ctx)
		return qErr
	}); err != nil {
		wrapped := fmt.Errorf("list sessions (failing closed to avoid double-dispatch): %w", err)
		recordFailure(ctx, statePath, wrapped)
		fatal("%v", wrapped)
	}
	occupied := occupiedWorkerIDs(sessions)

	// Fail closed on a PR-list failure for the same reason. Retried first: a
	// gh token mid-rotation (or any other transient gh failure) is exactly the
	// case retryAttempts exists for — see degraded.go.
	var prs []pullRequest
	if err := withRetry(ctx, func() error {
		var qErr error
		prs, qErr = queryOpenPRs(ctx, *repo)
		return qErr
	}); err != nil {
		wrapped := fmt.Errorf("query open PRs (failing closed to avoid double-dispatch): %w", err)
		recordFailure(ctx, statePath, wrapped)
		fatal("%v", wrapped)
	}

	ledgerFilePath := expandHome(*ledgerPath, home)
	ledger, err := loadLedger(ledgerFilePath)
	if err != nil {
		// A corrupt/unreadable ledger must not stop dispatch — it only degrades
		// reconciliation (nothing looks previously-dispatched) for this run.
		fmt.Fprintf(os.Stderr, "vroom-dispatch-direct: load ledger %s: %v (reconciliation degraded this run)\n", ledgerFilePath, err)
		ledger = &dispatchLedger{Beads: map[string]*ledgerEntry{}}
	}

	// Reconcile before selecting candidates: a bead our ledger shows we
	// previously dispatched a worker for, which no longer has a live worker,
	// gets its terminal outcome determined deterministically — merged work
	// closes it, an explicit no-op/failure note closes or blocks it, and
	// silent no-progress exits accumulate strikes toward an auto-block. This
	// is what stops a finished-but-unclosed bead from being redispatched
	// forever (the ce-2n5j / ce-24f1 flywheel-stall defect).
	mergedPRs, err := queryMergedPRs(ctx, *repo)
	reconciled := map[string]bool{}
	if err != nil {
		fmt.Fprintf(os.Stderr, "vroom-dispatch-direct: query merged PRs (skipping reconcile this run): %v\n", err)
	} else {
		reconciled = reconcile(ctx, dbPath, beads, occupied, prs, mergedPRs, ledger, *dryRun, os.Stdout, os.Stderr)
	}
	beads = excludeReconciled(beads, reconciled)

	// The full eligible list goes into the loop, never a pre-truncated slice.
	candidates := selectCandidates(beads, occupied, prs, *maxPriority)

	dispatched := dispatchCandidates(ctx, candidates, launch, repoDirPath, *maxDispatch, *dryRun, os.Stdout, os.Stderr, ledger)

	if !*dryRun {
		if err := saveLedger(ledgerFilePath, ledger); err != nil {
			fmt.Fprintf(os.Stderr, "vroom-dispatch-direct: save ledger %s: %v\n", ledgerFilePath, err)
		}
	}

	recordSuccess(statePath)

	fmt.Fprintf(os.Stderr,
		"vroom-dispatch-direct: %d ready, %d occupied worker name(s), %d eligible, %d dispatched, %d reconciled\n",
		len(beads), len(occupied), len(candidates), dispatched, len(reconciled))
}

// dispatchCandidates spawns a worker for each candidate in order and returns
// how many beads it actually dispatched. When ledger is non-nil and dryRun is
// false, every successful dispatch is recorded in it so a later run's reconcile
// pass can tell "we dispatched a worker for this bead and it's now gone" apart
// from "this bead was never touched" — the ledger entry is what makes
// deterministic bead-closure reconciliation possible across the tool's
// one-shot-per-tick invocations.
//
// maxDispatch (0 or negative = unlimited) bounds successful dispatches, not list
// positions — cap.go explains why that distinction matters.
func dispatchCandidates(ctx context.Context, candidates []bead, cfg workerLaunchConfig, repoDir string, maxDispatch int, dryRun bool, out, errOut io.Writer, ledger *dispatchLedger) int {
	dispatched := 0
	for i, b := range candidates {
		if ctx.Err() != nil {
			break
		}
		if dispatchCapReached(maxDispatch, dispatched, len(candidates)-i, errOut) {
			break
		}
		if dryRun {
			fmt.Fprintf(out, "would dispatch %s (%s) %s\n", workerSessionName(b.ID), priorityLabel(b.Priority), b.Title)
			dispatched++
			continue
		}
		if err := dispatch(ctx, b, cfg, repoDir); err != nil {
			// Deterministic per-bead failure: it will fail identically on every
			// retry, so skip this bead and keep dispatching — aborting here is
			// how one poisoned bead stalled all dispatch every tick (ce-b1zw).
			var skip *skipBeadError
			if errors.As(err, &skip) {
				fmt.Fprintf(errOut, "vroom-dispatch-direct: skip %s (deterministic spawn failure): %v\n", b.ID, err)
				continue
			}
			// A refused spawn is expected backpressure, not a fatal error: log
			// and stop this run (capacity is likely exhausted), retry next run.
			// Timeouts and unknown errors also stop the run (fail closed).
			fmt.Fprintf(errOut, "vroom-dispatch-direct: dispatch %s: %v\n", b.ID, err)
			break
		}
		fmt.Fprintf(out, "dispatched %s (%s) %s\n", workerSessionName(b.ID), priorityLabel(b.Priority), b.Title)
		dispatched++
		if ledger != nil {
			ledger.recordDispatch(b.ID, workerSessionName(b.ID))
		}
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
