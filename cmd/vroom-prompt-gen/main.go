// Command vroom-prompt-gen auto-generates orchestrator dispatch prompt files
// from `bd ready` output, closing the loop on the VROOM dispatch pipeline.
//
// Background (ce-5z0o): the orchestrator only dispatches work for which a prompt
// file already exists under ~/.agm/vroom/prompts/<bead-id>.md. That library was
// hand-curated and finite — once every file had been dispatched, the orchestrator
// reported "library exhausted" and idled, even with 100+ ready beads in the
// backlog. This tool is the source of new work: each tick the orchestrator runs
// it to materialise a prompt file for every ready, not-yet-handled bead.
//
// Deduplication is layered so re-running is idempotent and never double-dispatches:
//  1. existing prompt files (a bead already has a <id>.md)
//  2. open PRs (the bead is already in flight — its id appears in a PR branch/title)
//  3. the human-gated skip list (beads a human must drive, never an autonomous worker)
//
// Exit status is 0 on success even when zero new files are written — "nothing new
// to dispatch" is a normal steady state, not an error.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/vbonnet/dear-agent/internal/vroomprompt"
)

// humanGated lists beads that must never be auto-dispatched to a worker: they
// require a human in the loop (credential rotation, backups, repointing live
// skills, destructive ops) or are otherwise gated by operator decision. Kept in
// sync with the orchestrator skill's skip list (ce-5z0o).
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
var queryReady = func(db string) ([]bead, error) {
	cmd := exec.Command("bd", "--db", db, "ready", "--json")
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
var queryOpenPRs = func(repo string) ([]pullRequest, error) {
	cmd := exec.Command("gh", "pr", "list",
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

// existingPromptIDs returns the set of bead IDs that already have a prompt file
// in dir (e.g. ce-bi19.md -> "ce-bi19"). A missing directory yields an empty set,
// not an error — the orchestrator may run this before the dir is first created.
func existingPromptIDs(dir string) (map[string]bool, error) {
	ids := make(map[string]bool)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return ids, nil
		}
		return nil, err
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") {
			continue
		}
		ids[strings.TrimSuffix(name, ".md")] = true
	}
	return ids, nil
}

// mentionsID reports whether text references the bead id as a whole token rather
// than as a prefix of a longer id. The trailing boundary explicitly excludes '.'
// so that a sub-bead reference like "ce-cd14.2" does NOT count as a mention of
// its parent "ce-cd14" — otherwise an open PR for a child would wrongly suppress
// the parent (and vice versa). The leading boundary excludes alphanumerics so an
// id embedded in a branch path like "feat/fix-ce-5z0o" still matches.
func mentionsID(text, id string) bool {
	re := regexp.MustCompile(`(^|[^A-Za-z0-9])` + regexp.QuoteMeta(id) + `([^A-Za-z0-9.]|$)`)
	return re.MatchString(text)
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

// renderPrompt produces the dispatch prompt markdown for a bead. The body mirrors
// the hand-written prompt files already in ~/.agm/vroom/prompts/ and bakes in the
// standard worker rules (read-only ~/src, canonical Beads access, no --no-verify/--force, etc.).
func renderPrompt(b bead) string {
	return renderPromptForRoute(b, vroomprompt.DefaultRoute())
}

func renderPromptForRoute(b bead, route vroomprompt.Route) string {
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

Bead %s (%s). %s

**Stop after work complete + bead note written, OR provider-visible PR created through safe-pr. Never arm auto-merge. Do NOT close bead. Do NOT create new beads.**

## Goal

%s

## Rules

- ALWAYS use `+"`bd --db ~/beads/context-engine/.beads --dolt-auto-commit on`"+` (never bare bd)
- NEVER write to ~/src/** (read-only — use worktrees only)
- NEVER use --no-verify or --force
- NEVER run chezmoi apply
- ALWAYS publish with `+"`safe-push`"+`
- %s
- Do NOT run `+"`pkill -x gopls`"+`
- STOP after the primary deliverable is done — write a bead note and stop
`, b.ID, b.Title, b.ID, prio, summary, goal, vroomprompt.WorkerRule(route))
}

// candidate is a ready bead paired with the file path its prompt would be
// written to.
type candidate struct {
	bead bead
	path string
}

// selectCandidates filters ready beads down to those needing a fresh prompt
// file: not human-gated, not already prompted, not in flight in an open PR.
// Beads with an empty id are skipped defensively.
func selectCandidates(beads []bead, existing map[string]bool, prs []pullRequest, promptsDir string) []candidate {
	var out []candidate
	for _, b := range beads {
		if b.ID == "" {
			continue
		}
		if humanGated[b.ID] {
			continue
		}
		if existing[b.ID] {
			continue
		}
		if inFlightInPR(b.ID, prs) {
			continue
		}
		out = append(out, candidate{bead: b, path: filepath.Join(promptsDir, b.ID+".md")})
	}
	// Stable ordering so output is deterministic across runs.
	sort.Slice(out, func(i, j int) bool { return out[i].bead.ID < out[j].bead.ID })
	return out
}

// expandHome replaces a leading ~ in a path with the user's home directory.
func expandHome(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

func main() {
	home, err := os.UserHomeDir()
	if err != nil {
		fatal("home dir: %v", err)
	}

	db := flag.String("db", "~/beads/context-engine/.beads", "path to the beads database")
	promptsDir := flag.String("prompts-dir", "~/.agm/vroom/prompts", "directory to write prompt files into")
	repo := flag.String("repo", "vbonnet/dear-agent", "GitHub repo (owner/name) to check for open PRs")
	workerHarness := flag.String("worker-harness", "claude-code", "AGM harness for generated worker prompts")
	workerModel := flag.String("worker-model", "claude-opus-4-8", "model or family alias for generated worker prompts")
	workerMode := flag.String("worker-mode", "auto", "AGM permission mode for generated worker prompts")
	workspace := flag.String("workspace", "oss", "AGM workspace for generated worker prompts")
	dryRun := flag.Bool("dry-run", false, "report what would be written without writing any files")
	flag.Parse()

	dbPath := expandHome(*db, home)
	promptsPath := expandHome(*promptsDir, home)

	beads, err := queryReady(dbPath)
	if err != nil {
		fatal("query ready beads: %v", err)
	}

	// Fail closed on a PR-list failure: if we cannot tell which beads are
	// already in flight, do NOT risk double-dispatching them.
	prs, err := queryOpenPRs(*repo)
	if err != nil {
		fatal("query open PRs (failing closed to avoid double-dispatch): %v", err)
	}

	existing, err := existingPromptIDs(promptsPath)
	if err != nil {
		fatal("scan existing prompts: %v", err)
	}

	candidates := selectCandidates(beads, existing, prs, promptsPath)

	if !*dryRun {
		if err := os.MkdirAll(promptsPath, 0o755); err != nil {
			fatal("create prompts dir: %v", err)
		}
	}

	written := 0
	route := vroomprompt.Route{
		Harness: *workerHarness, Model: *workerModel, Mode: *workerMode, Workspace: *workspace,
	}
	for _, c := range candidates {
		if *dryRun {
			fmt.Printf("would write %s\n", c.path)
			written++
			continue
		}
		// Write atomically: temp file + rename so a crash mid-write never
		// leaves the orchestrator a truncated prompt to dispatch.
		tmp := c.path + ".tmp"
		if err := os.WriteFile(tmp, []byte(renderPromptForRoute(c.bead, route)), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "vroom-prompt-gen: write %s: %v\n", c.path, err)
			continue
		}
		if err := os.Rename(tmp, c.path); err != nil {
			fmt.Fprintf(os.Stderr, "vroom-prompt-gen: rename %s: %v\n", c.path, err)
			_ = os.Remove(tmp)
			continue
		}
		fmt.Println(c.path)
		written++
	}

	fmt.Fprintf(os.Stderr, "vroom-prompt-gen: %d ready, %d new prompt file(s)\n", len(beads), written)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "vroom-prompt-gen: "+format+"\n", args...)
	os.Exit(1)
}
