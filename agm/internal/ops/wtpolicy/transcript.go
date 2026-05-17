package wtpolicy

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// QuestionPhrases are the lowercase substrings that, when the final
// assistant turn contains one, mark a session as waiting for the user.
// Intentionally conservative (a false "awaiting" permanently blocks
// legitimate cleanup, which is worse here than a miss) and exported so it
// can be tuned without touching the matcher.
var QuestionPhrases = []string{
	"would you like",
	"should i",
	"what would you prefer",
}

// transcriptLine is the minimal shape of one Claude Code transcript event.
// Transcripts are heterogeneous (summary/system/assistant/user/…); only
// lines that carry a message with a user/assistant role are conversation
// turns, and isSidechain marks sub-agent traffic that is not the
// user-facing thread.
type transcriptLine struct {
	Type        string `json:"type"`
	IsSidechain bool   `json:"isSidechain"`
	Message     *struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
	Name string `json:"name"`
}

// projectSlug maps an absolute worktree path to Claude Code's projects
// directory name. CC slugifies the cwd by replacing the path separator
// (and '.') with '-', e.g. /Users/v/worktrees/dear-agent/foo →
// -Users-v-worktrees-dear-agent-foo.
func projectSlug(worktreePath string) string {
	return strings.NewReplacer("/", "-", ".", "-").Replace(worktreePath)
}

// TranscriptPathForWorktree returns the most recently modified transcript
// .jsonl for the session that ran in worktreePath, and ok=false when none
// can be located (no project dir, empty dir, unreadable). It tries the
// path as given and, as a fallback, its symlink-resolved form (so a
// symlinked worktrees root still maps correctly).
func TranscriptPathForWorktree(worktreePath string) (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	candidates := []string{worktreePath}
	if resolved, rerr := filepath.EvalSymlinks(worktreePath); rerr == nil && resolved != worktreePath {
		candidates = append(candidates, resolved)
	}
	for _, c := range candidates {
		dir := filepath.Join(home, ".claude", "projects", projectSlug(c))
		if p, ok := mostRecentJSONL(dir); ok {
			return p, true
		}
	}
	return "", false
}

func mostRecentJSONL(dir string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	type cand struct {
		path string
		mod  int64
	}
	var cands []cand
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		cands = append(cands, cand{filepath.Join(dir, e.Name()), info.ModTime().UnixNano()})
	}
	if len(cands) == 0 {
		return "", false
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].mod > cands[j].mod })
	return cands[0].path, true
}

// AwaitingInput reports whether the session that ran in worktreePath ended
// with the assistant waiting on the user, plus a short detail for the
// report.
//
// Fail-safe direction is deliberate and the OPPOSITE of the merge/dirty
// oracles: a missing/unreadable transcript, or a final turn that is not an
// unanswered assistant question, returns (false, ""). A false positive
// here would pin a genuinely-merged husk forever (the exact sprawl this
// tooling exists to clear); a false negative only risks retiring a
// merged-AND-clean worktree, and the transcript itself lives under
// ~/.claude/projects independent of the worktree, so no conversation is
// lost. Only a POSITIVE detection blocks retirement.
func AwaitingInput(worktreePath string) (awaiting bool, detail string) {
	path, ok := TranscriptPathForWorktree(worktreePath)
	if !ok {
		return false, ""
	}
	f, err := os.Open(path) //nolint:gosec // path derived from $HOME/.claude/projects + slug
	if err != nil {
		return false, ""
	}
	defer f.Close()

	lastRole := ""
	lastAssistantText := ""
	lastAssistantToolAsk := false

	// bufio.Reader (not Scanner): transcript lines embed whole tool
	// results/file contents and routinely exceed Scanner's token cap.
	r := bufio.NewReader(f)
	for {
		line, rerr := r.ReadBytes('\n')
		if len(line) > 0 {
			if role, text, toolAsk, okLine := parseTurn(line); okLine {
				lastRole = role
				if role == "assistant" {
					lastAssistantText = text
					lastAssistantToolAsk = toolAsk
				}
			}
		}
		if rerr != nil {
			break // EOF or a read error: classify on what we have
		}
	}

	if lastRole != "assistant" {
		return false, "" // user/harness replied after, or no turns
	}
	if lastAssistantToolAsk {
		return true, "AskUserQuestion"
	}
	if endsWithQuestion(lastAssistantText) {
		return true, "trailing-question"
	}
	if matchesQuestionPhrase(lastAssistantText) {
		return true, "question-phrase"
	}
	return false, ""
}

// parseTurn extracts the role and (for assistant turns) the concatenated
// text plus whether an AskUserQuestion tool was invoked. okLine is false
// for non-conversation lines (summaries, system, sidechain, parse errors)
// so the caller's last-turn tracking is not perturbed by them.
func parseTurn(raw []byte) (role, text string, askedViaTool, ok bool) {
	var tl transcriptLine
	if err := json.Unmarshal(raw, &tl); err != nil {
		return "", "", false, false
	}
	if tl.IsSidechain || tl.Message == nil {
		return "", "", false, false
	}
	role = tl.Message.Role
	if role != "user" && role != "assistant" {
		return "", "", false, false
	}
	if role != "assistant" {
		return role, "", false, true
	}

	// Assistant content is normally a block array; tolerate a bare string.
	var blocks []contentBlock
	if err := json.Unmarshal(tl.Message.Content, &blocks); err != nil {
		var s string
		if json.Unmarshal(tl.Message.Content, &s) == nil {
			return role, s, false, true
		}
		return role, "", false, true
	}
	var b strings.Builder
	for _, blk := range blocks {
		switch blk.Type {
		case "text":
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(blk.Text)
		case "tool_use":
			if blk.Name == "AskUserQuestion" {
				askedViaTool = true
			}
		}
	}
	return role, b.String(), askedViaTool, true
}

func endsWithQuestion(text string) bool {
	t := strings.TrimRight(text, " \t\r\n")
	return strings.HasSuffix(t, "?")
}

func matchesQuestionPhrase(text string) bool {
	lt := strings.ToLower(text)
	for _, p := range QuestionPhrases {
		if strings.Contains(lt, p) {
			return true
		}
	}
	return false
}
