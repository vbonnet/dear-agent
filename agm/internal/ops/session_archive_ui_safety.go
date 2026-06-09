package ops

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/vbonnet/dear-agent/agm/internal/claudeui"
	gitpkg "github.com/vbonnet/dear-agent/agm/internal/git"
)

// SafetyWarning is a reason a session may NOT be safe to archive even though it
// passed the idle/age/live gates. Mirrors the checks Claude Desktop performs
// before letting the user archive: don't bury sessions whose worktree still has
// work, that own an open PR, or that look like they are waiting on the user.
type SafetyWarning struct {
	Kind   string `json:"kind"`   // "uncommitted-work" | "open-pr" | "awaiting-input"
	Detail string `json:"detail"` // human-readable specifics
}

const (
	warnUncommitted   = "uncommitted-work"
	warnOpenPR        = "open-pr"
	warnAwaitingInput = "awaiting-input"
)

// wtInspector abstracts the git/gh probes so the safety logic is unit-testable
// without a real repo or network. The real implementation reuses the tested
// internal/git primitives.
type wtInspector interface {
	Worktrees(repoPath string) ([]gitpkg.Worktree, error)
	Dirty(path string) (bool, error)
	Unmerged(path string) (commits bool, detail string)
	OpenPR(repoPath, branch string) (number int, known bool)
}

// transcriptScanner returns the last assistant text message for a CLI session
// id, if a transcript exists. The bool reports whether a transcript was found.
type transcriptScanner func(cliSessionID string) (lastAssistantText string, found bool)

// evalSafetyWarnings computes the warnings for a single (archive-eligible)
// session. It is called only for the archive direction and only after the
// idle/live gates, so the git/gh cost is paid for a handful of sessions, not
// the whole store.
func evalSafetyWarnings(s *claudeui.Session, insp wtInspector, scan transcriptScanner) []SafetyWarning {
	var warns []SafetyWarning

	// (1)+(2) Worktree association: the session's cwd/originCwd if they still
	// exist and are git worktrees, plus (req. 4) any worktree whose directory
	// name or branch contains the session identifier.
	for _, w := range associatedWorktrees(s, insp) {
		if dirty, err := insp.Dirty(w.Path); err != nil || dirty {
			detail := w.Path + " has uncommitted changes"
			if err != nil {
				detail = w.Path + " status unknown (treated as dirty): " + err.Error()
			}
			warns = append(warns, SafetyWarning{Kind: warnUncommitted, Detail: detail})
		} else if unmerged, d := insp.Unmerged(w.Path); unmerged {
			warns = append(warns, SafetyWarning{Kind: warnUncommitted, Detail: w.Path + ": " + d})
		}
		if w.Branch != "" {
			if num, known := insp.OpenPR(w.Path, w.Branch); known {
				warns = append(warns, SafetyWarning{
					Kind:   warnOpenPR,
					Detail: "branch " + w.Branch + " has open PR #" + strconv.Itoa(num),
				})
			}
		}
	}

	// (3) Transcript heuristic for "waiting on the user" — AskUserQuestion is
	// invisible to hooks, so we approximate via the last assistant message.
	if scan != nil {
		if text, found := scan(s.CliSessionID); found {
			if reason, waiting := awaitingInputReason(text); waiting {
				warns = append(warns, SafetyWarning{Kind: warnAwaitingInput, Detail: reason})
			}
		}
	}

	return warns
}

// associatedWorktrees resolves the worktrees linked to a session: the cwd /
// originCwd directly, plus any worktree whose basename or branch contains the
// session's cliSessionId or sessionId (requirement 4).
func associatedWorktrees(s *claudeui.Session, insp wtInspector) []gitpkg.Worktree {
	repoHint := firstExistingDir(s.Cwd, s.OriginCwd)
	if repoHint == "" {
		return nil // worktree gone (likely already cleaned/merged) — nothing to inspect
	}

	wts, err := insp.Worktrees(repoHint)
	if err != nil {
		return nil
	}

	ids := []string{s.CliSessionID, s.SessionID}
	matched := map[string]gitpkg.Worktree{}
	for _, w := range wts {
		switch {
		case w.Path == s.Cwd || w.Path == s.OriginCwd,
			containsAny(filepath.Base(w.Path), ids),
			containsAny(w.Branch, ids):
			matched[w.Path] = w
		}
	}
	// If the cwd is itself a repo/worktree git didn't list as a separate entry
	// (e.g. the main checkout), still inspect it directly.
	if len(matched) == 0 && dirExists(s.Cwd) {
		matched[s.Cwd] = gitpkg.Worktree{Path: s.Cwd}
	}

	out := make([]gitpkg.Worktree, 0, len(matched))
	for _, w := range matched {
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// awaitingInputReason applies the "waiting for user input" heuristic to the
// last assistant message: a trailing question mark, or one of the canonical
// clarifying-question lead-ins.
func awaitingInputReason(text string) (reason string, waiting bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", false
	}
	if strings.HasSuffix(trimmed, "?") {
		return "last assistant message ends with a question", true
	}
	lower := strings.ToLower(trimmed)
	for _, p := range []string{"would you like", "should i", "what would you prefer"} {
		if strings.Contains(lower, p) {
			return "last assistant message contains " + strconv.Quote(p), true
		}
	}
	return "", false
}

// assistantTextOf parses one transcript JSONL line and, if it is an assistant
// message, returns the concatenation of its text blocks (empty for tool-only
// or non-assistant lines).
func assistantTextOf(line string) string {
	line = strings.TrimSpace(line)
	if line == "" || !strings.Contains(line, `"assistant"`) {
		return ""
	}
	var rec struct {
		Type    string `json:"type"`
		Message struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal([]byte(line), &rec) != nil || rec.Type != "assistant" {
		return ""
	}
	// content may be a plain string or an array of typed blocks.
	var asString string
	if json.Unmarshal(rec.Message.Content, &asString) == nil {
		return asString
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(rec.Message.Content, &blocks) != nil {
		return ""
	}
	var b strings.Builder
	for _, bl := range blocks {
		if bl.Type == "text" {
			b.WriteString(bl.Text)
		}
	}
	return b.String()
}

// --- real implementations (used when the request leaves them nil) ---

type realInspector struct{}

func (realInspector) Worktrees(repoPath string) ([]gitpkg.Worktree, error) {
	return gitpkg.ListWorktrees(repoPath)
}

func (realInspector) Dirty(path string) (bool, error) {
	return gitpkg.HasUncommittedChanges(path)
}

// Unmerged reports whether the worktree branch holds commits not in the
// resolved base ref. If the base cannot be resolved we report "not unmerged"
// rather than guessing — the dirty and open-PR checks still apply.
func (realInspector) Unmerged(path string) (bool, string) {
	base := gitpkg.ResolveBaseRef(path)
	if base == "" {
		return false, ""
	}
	out, err := exec.Command("git", "-C", path, "rev-list", "--count", base+"..HEAD").Output()
	if err != nil {
		return false, ""
	}
	n := strings.TrimSpace(string(out))
	if n == "" || n == "0" {
		return false, ""
	}
	return true, n + " commit(s) not in " + base
}

func (realInspector) OpenPR(repoPath, branch string) (int, bool) {
	return gitpkg.OpenPRForBranch(repoPath, branch)
}

// realTranscriptScanner returns a scanner over ~/.claude/projects/*/<cli>.jsonl.
func realTranscriptScanner(projectsDir string) transcriptScanner {
	return func(cli string) (string, bool) {
		if cli == "" {
			return "", false
		}
		matches, _ := filepath.Glob(filepath.Join(projectsDir, "*", cli+".jsonl"))
		if len(matches) == 0 {
			return "", false
		}
		// Most-recently-modified wins if a session ran in multiple cwds.
		best, bestMod := "", int64(-1)
		for _, m := range matches {
			if fi, err := os.Stat(m); err == nil && fi.ModTime().UnixNano() > bestMod {
				best, bestMod = m, fi.ModTime().UnixNano()
			}
		}
		if best == "" {
			return "", false
		}
		return lastAssistantText(best), true
	}
}

// lastAssistantText reads the tail of a transcript and returns the text of the
// last assistant message that carried any text (skipping tool-only turns).
func lastAssistantText(path string) string {
	const tailBytes = 256 * 1024
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return ""
	}
	start := int64(0)
	if fi.Size() > tailBytes {
		start = fi.Size() - tailBytes
	}
	buf := make([]byte, fi.Size()-start)
	if _, err := f.ReadAt(buf, start); err != nil && len(buf) == 0 {
		return ""
	}

	lines := strings.Split(string(buf), "\n")
	if start > 0 && len(lines) > 0 {
		lines = lines[1:] // drop the partial first line
	}
	for i := len(lines) - 1; i >= 0; i-- {
		if t := assistantTextOf(lines[i]); t != "" {
			return t
		}
	}
	return ""
}

func dirExists(p string) bool {
	if p == "" {
		return false
	}
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func firstExistingDir(paths ...string) string {
	for _, p := range paths {
		if dirExists(p) {
			return p
		}
	}
	return ""
}

func containsAny(hay string, needles []string) bool {
	for _, n := range needles {
		if n != "" && strings.Contains(hay, n) {
			return true
		}
	}
	return false
}
