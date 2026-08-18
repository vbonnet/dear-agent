package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/pkg/vroom/decisiontrail"
)

type GuardConfig struct {
	BeadID        string
	Repo          string
	BeadsDir      string
	AbandonReason string
	// RepoRoot is the dear-agent checkout the deployed gate resolves source
	// paths against. Empty disables the gate (it records why and does not block).
	RepoRoot string
	// GitRef, when set, makes the deployed gate compare the host against this
	// committed ref (e.g. origin/main) instead of the working tree.
	GitRef string
}

type GuardResult struct {
	BeadID        string
	Title         string
	PRs           []int
	UnmergedPR    []UnmergedPR
	Passed        bool
	AbandonReason string
	// DriftFindings are deploy targets the bead's merged change touched that are
	// not current on the host — the "deployed" gate's block reasons.
	DriftFindings []DeployFinding
	// DeployTargetsChecked is how many deploy targets the merged change touched
	// (0 means the gate had nothing to verify). Surfaced so a clean run shows the
	// gate actually ran rather than silently doing nothing.
	DeployTargetsChecked int
	// DeploySkipNote explains why the deployed gate did not run, when it could
	// not (no checkout, no config). Empty when the gate ran.
	DeploySkipNote string
}

type UnmergedPR struct {
	Number int
	State  string
}

type prViewResult struct {
	Number   int    `json:"number"`
	State    string `json:"state"`
	MergedAt string `json:"mergedAt"`
	Files    []struct {
		Path string `json:"path"`
	} `json:"files"`
}

var prRE = regexp.MustCompile(`(\w*)#(\d+)`)

func CheckDoD(cfg GuardConfig) (GuardResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	detail, err := bdShow(ctx, cfg.BeadsDir, cfg.BeadID)
	if err != nil {
		return GuardResult{}, fmt.Errorf("reading bead %s: %w", cfg.BeadID, err)
	}

	prNums := extractPRNumbers(detail)
	result := GuardResult{
		BeadID:        cfg.BeadID,
		Title:         extractTitle(detail),
		PRs:           prNums,
		AbandonReason: cfg.AbandonReason,
	}

	if len(prNums) == 0 {
		result.Passed = true
		return result, nil
	}

	var mergedFiles []string
	for _, n := range prNums {
		merged, state, files, err := prInfo(ctx, cfg.Repo, n)
		if err != nil {
			// Do not swallow the error: if we cannot verify a PR's state
			// (network, rate limit, gh auth), returning the error makes the
			// command exit non-zero so the hook's "fail open but warn" path
			// runs. Continuing here would leave UnmergedPR empty and silently
			// pass the guard, falsely claiming the PR is merged.
			return GuardResult{}, fmt.Errorf("checking PR #%d: %w", n, err)
		}
		if !merged {
			result.UnmergedPR = append(result.UnmergedPR, UnmergedPR{Number: n, State: state})
			continue
		}
		// Only a merged PR's files can have a deploy expectation: its source is
		// on main, so the host should match it. An unmerged PR is already
		// blocked by the gate above; its files are not yet "supposed" to ship.
		mergedFiles = append(mergedFiles, files...)
	}

	// Deployed gate (Phase 2): a fix can be merged yet never reach the machine.
	// When the merged change touched a deploy target, require that target to be
	// current on the host before the bead may close.
	if len(mergedFiles) > 0 {
		findings, checked, note := deployedGate(ctx, cfg.RepoRoot, cfg.GitRef, mergedFiles)
		result.DriftFindings = findings
		result.DeployTargetsChecked = checked
		result.DeploySkipNote = note
	}

	blocked := len(result.UnmergedPR) > 0 || len(result.DriftFindings) > 0
	result.Passed = !blocked || cfg.AbandonReason != ""
	return result, nil
}

func FormatResult(r GuardResult, w io.Writer) {
	blockedByPR := len(r.UnmergedPR) > 0
	blockedByDeploy := len(r.DriftFindings) > 0

	// Abandoning the bead releases both gates (audited separately): the agent is
	// declaring the work will not be completed, so neither "merged" nor
	// "deployed" applies.
	if r.AbandonReason != "" && (blockedByPR || blockedByDeploy) {
		var reasons []string
		if blockedByPR {
			reasons = append(reasons, fmt.Sprintf("unmerged PR(s) %s", prListString(r.UnmergedPR)))
		}
		if blockedByDeploy {
			reasons = append(reasons, fmt.Sprintf("%d undeployed artifact(s)", len(r.DriftFindings)))
		}
		fmt.Fprintf(w, "OVERRIDE: bead %s closed despite %s — abandoning: %s\n",
			r.BeadID, strings.Join(reasons, " and "), r.AbandonReason)
		return
	}

	if !blockedByPR && !blockedByDeploy {
		if len(r.PRs) == 0 {
			fmt.Fprintf(w, "ok: bead %s has no PR references — close is allowed\n", r.BeadID)
			return
		}
		fmt.Fprintf(w, "ok: bead %s — all %d referenced PR(s) are merged", r.BeadID, len(r.PRs))
		if r.DeployTargetsChecked > 0 {
			fmt.Fprintf(w, "; %d deploy target(s) current on host", r.DeployTargetsChecked)
		}
		fmt.Fprintln(w)
		if r.DeploySkipNote != "" {
			fmt.Fprintf(w, "note: deployed gate skipped — %s\n", r.DeploySkipNote)
		}
		return
	}

	fmt.Fprintf(w, "BLOCKED: cannot close bead %s\n", r.BeadID)

	if blockedByPR {
		fmt.Fprintf(w, "\nDefinition of Done requires all referenced PRs to be merged to main.\n")
		fmt.Fprintf(w, "Unmerged PR(s): %s\n", prListString(r.UnmergedPR))
		fmt.Fprintf(w, "See docs/policies/definition-of-done.ai.md.\n")
		fmt.Fprintf(w, "To fix:\n")
		for _, pr := range r.UnmergedPR {
			fmt.Fprintf(w, "  • Merge PR #%d first, then close the bead\n", pr.Number)
		}
	}

	if blockedByDeploy {
		fmt.Fprintf(w, "\nMerged != deployed: %d artifact(s) the merged change touched are not current on the host.\n",
			len(r.DriftFindings))
		for _, f := range r.DriftFindings {
			label := "stale on host"
			if f.Status == "missing" {
				label = "never installed"
			}
			fmt.Fprintf(w, "  ✗ %s — %s (%s)\n", f.Target, label, f.Deployed)
		}
		fmt.Fprintln(w, "The fix is on main but never reached the machine; see cmd/drift-check/README.md.")
		fmt.Fprintf(w, "To fix, redeploy then re-close:\n")
		for _, cmd := range dedupRemediations(r.DriftFindings) {
			fmt.Fprintf(w, "  • %s\n", cmd)
		}
	}

	fmt.Fprintf(w, "  • Or use --abandon-reason \"<why>\" if abandoning this bead (audited)\n")
}

// prListString renders unmerged PRs as "#449 (OPEN), #551 (DRAFT)".
func prListString(prs []UnmergedPR) string {
	parts := make([]string, len(prs))
	for i, pr := range prs {
		parts[i] = fmt.Sprintf("#%d (%s)", pr.Number, pr.State)
	}
	return strings.Join(parts, ", ")
}

// AbandonAuditRecord is one JSONL line in the bead-abandon audit log.
type AbandonAuditRecord struct {
	Time          string `json:"time"`
	BeadID        string `json:"bead_id"`
	AbandonReason string `json:"abandon_reason"`
	UnmergedPRs   []int  `json:"unmerged_prs,omitempty"`
}

// AppendAbandonAudit appends one record to ~/.local/state/dear-agent/bead-abandon-audit.jsonl.
func AppendAbandonAudit(home, beadID, reason string, unmergedPRs []int) (err error) {
	if home == "" {
		home = "/tmp"
	}
	dir := filepath.Join(home, ".local", "state", "dear-agent")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("cannot create audit dir %s: %w", dir, err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "bead-abandon-audit.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("cannot open abandon audit log: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("cannot close abandon audit log: %w", cerr)
		}
	}()
	rec := AbandonAuditRecord{
		Time:          time.Now().UTC().Format(time.RFC3339),
		BeadID:        beadID,
		AbandonReason: reason,
		UnmergedPRs:   unmergedPRs,
	}
	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("cannot marshal abandon audit record: %w", err)
	}
	if _, err = f.Write(append(line, '\n')); err != nil {
		return err
	}
	return nil
}

// AppendDoDViolationTrail records a BLOCKED bead-close in the VROOM decision
// trail (~/.agm/vroom/trail.jsonl) so supervisor ticks and retros can see every
// prevented Definition-of-Done violation — not only the ones an agent chose to
// abandon (those land in bead-abandon-audit.jsonl). The retro that spawned this
// guard (ce-d2f / ce-y1zl) needed exactly this signal: a count of how often the
// guard fired, surfaced where the supervisors already look.
//
// Best-effort: any failure here is returned for the caller to warn about but
// must NOT change the guard's verdict — the block stands regardless.
func AppendDoDViolationTrail(home, beadID, title string, unmergedPRs []int) (err error) {
	if home == "" {
		home = "/tmp"
	}
	path := filepath.Join(home, ".agm", "vroom", "trail.jsonl")
	trail, err := decisiontrail.OpenJSONL(path)
	if err != nil {
		return fmt.Errorf("cannot open vroom trail: %w", err)
	}
	defer func() {
		if cerr := trail.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("cannot close vroom trail: %w", cerr)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Payload uses []int (not []any) so readers can decode unmerged_prs back to
	// numbers; JSON renders both identically.
	prs := unmergedPRs
	if prs == nil {
		prs = []int{}
	}
	return trail.Append(ctx, decisiontrail.Record{
		Role: "bead-close-guard",
		Kind: "dod.violation.blocked",
		Payload: map[string]any{
			"bead_id":      beadID,
			"title":        title,
			"unmerged_prs": prs,
		},
	})
}

func extractPRNumbers(text string) []int {
	seen := make(map[int]bool)
	var nums []int
	for _, m := range prRE.FindAllStringSubmatch(text, -1) {
		// m[1] is the token immediately preceding '#'. An empty prefix is a
		// bare "#449"; "pr"/"issue" (any case) are common local references
		// like "PR#449". Any other non-empty prefix is a cross-repo reference
		// such as "dotfiles#15" and must be skipped.
		prefix := strings.ToLower(m[1])
		if prefix != "" && prefix != "pr" && prefix != "issue" {
			continue
		}
		var n int
		for _, c := range m[2] {
			n = n*10 + int(c-'0')
		}
		if !seen[n] && n > 0 {
			seen[n] = true
			nums = append(nums, n)
		}
	}
	return nums
}

func extractTitle(text string) string {
	for line := range strings.SplitSeq(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			return line
		}
		rest := strings.Join(parts[2:], " ")
		// strings.Fields collapsed the whitespace, so the separator now leads
		// the joined string as "· " (no preceding space) rather than " · ".
		// Strip the leading separator directly; fall back to the embedded form.
		if trimmed, ok := strings.CutPrefix(rest, "· "); ok {
			rest = trimmed
		} else if idx := strings.Index(rest, " · "); idx >= 0 {
			rest = rest[idx+3:]
		}
		if idx := strings.LastIndex(rest, " ["); idx >= 0 {
			rest = rest[:idx]
		}
		return strings.TrimSpace(rest)
	}
	return "(unknown)"
}

// prInfo returns a PR's merge status, raw state, and changed file paths
// (repo-relative). The file list feeds the deployed gate: it maps a merged PR's
// changes to the deploy targets whose source-of-truth they touched.
func prInfo(ctx context.Context, repo string, prNum int) (merged bool, state string, files []string, err error) {
	args := []string{"pr", "view", fmt.Sprintf("%d", prNum), "--json", "state,mergedAt,number,files"}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return false, "", nil, ctx.Err()
		}
		return false, "", nil, fmt.Errorf("gh pr view %d: %w (stderr: %s)", prNum, err, stderr.String())
	}

	var v prViewResult
	if err := json.Unmarshal(stdout.Bytes(), &v); err != nil {
		return false, "", nil, fmt.Errorf("parse gh output: %w", err)
	}

	for _, f := range v.Files {
		if f.Path != "" {
			files = append(files, f.Path)
		}
	}
	return strings.EqualFold(v.State, "MERGED"), v.State, files, nil
}

func bdShow(ctx context.Context, beadsDir, id string) (string, error) {
	args := []string{"show", id}
	if beadsDir != "" {
		args = append([]string{"--db", beadsDir}, args...)
	}
	cmd := exec.CommandContext(ctx, "bd", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("bd show %s: %w (stderr: %s)", id, err, stderr.String())
	}
	return stdout.String(), nil
}

// detectRepo resolves owner/name from the origin remote. When beadsDir is set
// (the .beads dir lives inside the target repo), run git there so the repo is
// detected correctly even if the agent ran `bd close` from another directory.
func detectRepo(beadsDir string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "remote", "get-url", "origin")
	if beadsDir != "" {
		cmd.Dir = beadsDir
	}
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	repo := extractRepoFromURL(strings.TrimSpace(string(out)))
	if repo == "" {
		return "", fmt.Errorf("cannot parse GitHub repo from remote URL: %q", strings.TrimSpace(string(out)))
	}
	return repo, nil
}

func extractRepoFromURL(rawURL string) string {
	rawURL = strings.TrimSuffix(rawURL, ".git")
	if idx := strings.LastIndex(rawURL, "github.com/"); idx >= 0 {
		return rawURL[idx+len("github.com/"):]
	}
	if idx := strings.LastIndex(rawURL, "github.com:"); idx >= 0 {
		return rawURL[idx+len("github.com:"):]
	}
	return ""
}
