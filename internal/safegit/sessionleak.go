package safegit

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/internal/sessionid"
)

// commitRecordSep and commitFieldSep delimit `git log` output. ASCII record
// (0x1e) and unit (0x1f) separators are used rather than newlines because a
// commit message is itself multi-line; a newline-delimited format could not be
// parsed unambiguously.
const (
	commitRecordSep = "\x1e"
	commitFieldSep  = "\x1f"
)

// leakScanTimeout bounds the local `git log`. It is generous for a local
// object walk and short enough that a wedged repository cannot delay a push.
const leakScanTimeout = 15 * time.Second

// UnpushedCommit is one commit that is about to reach the remote.
type UnpushedCommit struct {
	SHA     string
	Subject string
	Message string
}

// UnpushedCommits lists commits reachable from HEAD but not from any origin
// remote-tracking ref — that is, the commits this push would publish.
//
// The range is `HEAD --not --remotes=origin` rather than the exact refspec
// being pushed: computing the true set would require asking the remote
// (`--dry-run` costs a network round trip on every push), while the local
// remote-tracking refs answer it offline. The difference is conservative in
// the right direction — a stale origin ref can only include a commit that was
// already published, never exclude one that is about to be.
func UnpushedCommits(repoDir string) ([]UnpushedCommit, error) {
	ctx, cancel := context.WithTimeout(context.Background(), leakScanTimeout)
	defer cancel()

	argv := []string{}
	if repoDir != "" {
		argv = append(argv, "-C", repoDir)
	}
	argv = append(argv,
		"log", "--no-color", "--no-notes",
		"--format=%H"+commitFieldSep+"%s"+commitFieldSep+"%B"+commitRecordSep,
		"HEAD", "--not", "--remotes=origin",
	)
	// No shell: argv entries are compile-time literals plus the caller's
	// repository directory.
	cmd := exec.CommandContext(ctx, "git", argv...)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("cannot list unpushed commits: %w", err)
	}

	var commits []UnpushedCommit
	for rec := range strings.SplitSeq(string(out), commitRecordSep) {
		rec = strings.TrimLeft(rec, "\n")
		if strings.TrimSpace(rec) == "" {
			continue
		}
		fields := strings.SplitN(rec, commitFieldSep, 3)
		if len(fields) != 3 {
			continue
		}
		commits = append(commits, UnpushedCommit{
			SHA:     fields[0],
			Subject: fields[1],
			Message: fields[2],
		})
	}
	return commits, nil
}

// SessionLeakError describes commit messages that carry a Claude Code session
// reference, and teaches the two rewrites that clear them.
type SessionLeakError struct {
	Offenders []UnpushedCommit
	// BaseHint is the first offender's parent, used in the reset --soft recipe.
	BaseHint string
}

func (e *SessionLeakError) Error() string {
	var b strings.Builder
	b.WriteString("refusing to push: ")
	if len(e.Offenders) == 1 {
		b.WriteString("1 unpushed commit carries")
	} else {
		fmt.Fprintf(&b, "%d unpushed commits carry", len(e.Offenders))
	}
	b.WriteString(" a private Claude Code session reference in its message:\n\n")
	for _, c := range e.Offenders {
		short := c.SHA
		if len(short) > 9 {
			short = short[:9]
		}
		fmt.Fprintf(&b, "  %s %s\n", short, c.Subject)
		b.WriteString(sessionid.Describe(sessionid.Scan(c.Message)))
	}
	b.WriteString("\nA session reference addresses a private transcript and gives reviewers nothing. " +
		"Once pushed it is permanent: squash-merge folds every commit message into main's history, " +
		"which is how 160 session ids reached this repository's main branch.\n\n")
	b.WriteString("Rewrite the messages before pushing:\n")
	b.WriteString("  - tip commit only:  git commit --amend        (delete the offending line)\n")
	if e.BaseHint != "" {
		fmt.Fprintf(&b, "  - several commits:  git reset --soft %s && git commit\n", e.BaseHint)
	} else {
		b.WriteString("  - several commits:  git reset --soft <base> && git commit\n")
	}
	b.WriteString("\nsafe-push refuses rather than editing the messages itself: rewriting history " +
		"is the author's decision, not a wrapper's.")
	return b.String()
}

// CheckSessionLeaks refuses the push when any commit it would publish carries a
// session reference.
//
// It fails OPEN when the commit list cannot be read at all — an unreadable
// repository, a missing origin, a shallow clone — printing a warning to stderr
// rather than blocking. That mirrors .claude/hooks/pretool-pr-guard, which
// fails open for the same reason: this is a best-effort hygiene net over a
// cooperative path, and a net that wedges pushes on unrelated git conditions
// gets routed around, which prevents nothing. It fails CLOSED whenever it did
// read the messages and did find a reference.
func CheckSessionLeaks(repoDir string) error {
	commits, err := UnpushedCommits(repoDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "safe-push: skipping the session-reference scan: %v\n", err)
		return nil
	}
	var offenders []UnpushedCommit
	for _, c := range commits {
		if sessionid.Has(c.Message) {
			offenders = append(offenders, c)
		}
	}
	if len(offenders) == 0 {
		return nil
	}
	// git log lists newest first, so the last offender is the oldest; its
	// parent is the cut point for the reset --soft recipe.
	oldest := offenders[len(offenders)-1]
	return &SessionLeakError{Offenders: offenders, BaseHint: oldest.SHA[:min(9, len(oldest.SHA))] + "^"}
}
