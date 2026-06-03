package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"
	"time"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("agm-worktree-audit", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var (
		root         = fs.String("root", defaultRoot(), "directory whose child repos are scanned")
		jsonOut      = fs.Bool("json", false, "emit findings as JSON instead of a text report")
		worktreeDays = fs.Int("worktree-days", 7, "worktree HEAD older than this many days is 'abandoned'")
		branchDays   = fs.Int("branch-days", 14, "unmerged branch untouched this many days is 'stale'")
	)
	fs.Usage = func() {
		fmt.Fprintf(stderr, "usage: agm-worktree-audit [flags]\n\n"+
			"Scans every git repo under --root and reports reclaimable worktrees\n"+
			"and branches. Read-only: never removes anything.\n\nflags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}

	th := Thresholds{
		WorktreeStale: time.Duration(*worktreeDays) * 24 * time.Hour,
		BranchStale:   time.Duration(*branchDays) * 24 * time.Hour,
	}

	repoPaths, err := DiscoverRepos(*root)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if len(repoPaths) == 0 {
		fmt.Fprintf(stderr, "no git repositories found under %s\n", *root)
		return 0
	}

	repos := make([]RepoData, 0, len(repoPaths))
	for _, p := range repoPaths {
		repos = append(repos, CollectRepo(p))
	}

	now := time.Now()
	findings := Categorize(repos, now, th)

	if *jsonOut {
		return emitJSON(stdout, stderr, *root, now, th, repos, findings)
	}
	emitText(stdout, *root, now, th, repos, findings)
	return 0
}

func defaultRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "src"
	}
	return filepath.Join(home, "src")
}

// jsonReport is the top-level JSON document.
type jsonReport struct {
	Root          string         `json:"root"`
	GeneratedAt   time.Time      `json:"generated_at"`
	WorktreeDays  float64        `json:"worktree_stale_days"`
	BranchDays    float64        `json:"branch_stale_days"`
	ReposScanned  int            `json:"repos_scanned"`
	FindingCounts map[string]int `json:"finding_counts"`
	Findings      []Finding      `json:"findings"`
}

func emitJSON(stdout, stderr *os.File, root string, now time.Time, th Thresholds, repos []RepoData, findings []Finding) int {
	rep := jsonReport{
		Root:          root,
		GeneratedAt:   now,
		WorktreeDays:  th.WorktreeStale.Hours() / 24,
		BranchDays:    th.BranchStale.Hours() / 24,
		ReposScanned:  len(repos),
		FindingCounts: countByKind(findings),
		Findings:      findings,
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rep); err != nil {
		fmt.Fprintf(stderr, "error encoding json: %v\n", err)
		return 1
	}
	return 0
}

func countByKind(findings []Finding) map[string]int {
	m := map[string]int{}
	for _, f := range findings {
		m[string(f.Kind)]++
	}
	return m
}

// kindOrder controls the section order and supplies human-readable headers.
var kindOrder = []struct {
	kind   FindingKind
	header string
}{
	{KindAbandonedWorktree, "Abandoned worktrees (no recent commit)"},
	{KindWorktreeNoRemote, "Worktrees with no remote branch (local-only work)"},
	{KindMergedNotDeleted, "Merged branches not deleted"},
	{KindStaleUnmerged, "Stale unmerged branches"},
}

func emitText(w *os.File, root string, now time.Time, th Thresholds, repos []RepoData, findings []Finding) {
	fmt.Fprintf(w, "Worktree & branch abandonment audit\n")
	fmt.Fprintf(w, "  root:            %s\n", root)
	fmt.Fprintf(w, "  generated:       %s\n", now.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(w, "  repos scanned:   %d\n", len(repos))
	fmt.Fprintf(w, "  worktree stale:  >= %.0f days   branch stale: >= %.0f days\n\n",
		th.WorktreeStale.Hours()/24, th.BranchStale.Hours()/24)

	counts := countByKind(findings)
	if len(findings) == 0 {
		fmt.Fprintf(w, "✓ nothing to clean up — no abandoned worktrees or branches found.\n")
		warnUnresolved(w, repos)
		return
	}

	byKind := map[FindingKind][]Finding{}
	for _, f := range findings {
		byKind[f.Kind] = append(byKind[f.Kind], f)
	}

	for _, k := range kindOrder {
		group := byKind[k.kind]
		if len(group) == 0 {
			continue
		}
		fmt.Fprintf(w, "── %s (%d) ──\n", k.header, len(group))
		tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
		fmt.Fprintf(tw, "  REPO\tBRANCH\tLAST COMMIT\tAGE\tA/B\tMERGED\tDETAIL\n")
		for _, f := range group {
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				f.Repo, branchLabel(f), lastCommitLabel(f.LastCommit),
				ageLabel(now, f.LastCommit), abLabel(f.Ahead, f.Behind),
				mergedLabel(f.Merged), detail(f))
		}
		tw.Flush()
		fmt.Fprintln(w)
	}

	fmt.Fprintf(w, "Summary: %d findings across %d repos — ", len(findings), len(repos))
	parts := make([]string, 0, len(kindOrder))
	for _, k := range kindOrder {
		if c := counts[string(k.kind)]; c > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", c, k.kind))
		}
	}
	fmt.Fprintf(w, "%s\n", joinComma(parts))
	warnUnresolved(w, repos)
}

// warnUnresolved flags repos where the base ref could not be resolved, since
// their branch-level findings (merged/ahead/behind) are unreliable.
func warnUnresolved(w *os.File, repos []RepoData) {
	var bad []string
	for _, r := range repos {
		if r.BaseRef == "" {
			bad = append(bad, r.Name)
		}
	}
	if len(bad) == 0 {
		return
	}
	sort.Strings(bad)
	fmt.Fprintf(w, "\n⚠ no base ref (main/master) resolved for: %s\n", joinComma(bad))
	fmt.Fprintf(w, "  branch merge/ahead-behind data for these repos is unavailable.\n")
}

func branchLabel(f Finding) string {
	if f.Branch == "" {
		return "(detached)"
	}
	return f.Branch
}

func lastCommitLabel(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	return t.Format("2006-01-02")
}

func ageLabel(now, t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	days := int(now.Sub(t).Hours() / 24)
	return fmt.Sprintf("%dd", days)
}

func abLabel(ahead, behind int) string {
	if ahead < 0 || behind < 0 {
		return "?"
	}
	return fmt.Sprintf("+%d/-%d", ahead, behind)
}

func mergedLabel(merged bool) string {
	if merged {
		return "yes"
	}
	return "no"
}

func detail(f Finding) string {
	switch f.Kind {
	case KindWorktreeNoRemote, KindAbandonedWorktree:
		return f.Path
	case KindMergedNotDeleted, KindStaleUnmerged:
		return f.Reason
	default:
		return f.Reason
	}
}

func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}
