// safe-merge lands a feature branch into a golden ~/src checkout as one atomic,
// audited unit, so the dangerous raw form `git -C ~/src/* merge *` never needs
// to be allow-listed.
//
// Usage:
//
//	safe-merge --repo <dir> --branch <feature> [--mode squash|no-ff] [--message <msg>]
//	           (--wayfinder <project-dir> | --emergency --reason "<why>") [--timeout <dur>]
//
// Examples:
//
//	safe-merge --repo ~/src/dear-agent --branch feat/foo --wayfinder ~/src/engram-research/projects/foo
//	safe-merge --repo ~/src/dear-agent --branch hotfix --emergency --reason "prod down, no session"
//
// safe-merge verifies the target is a clean git tree on its default branch,
// folds the branch in (squash+commit by default, keeping history linear), and
// rolls the tree back to its pre-merge state on any conflict. It never pushes —
// pushing the landed commit stays safe-push's job — and never force-anything.
// Every invocation is recorded in ~/.local/state/dear-agent/safe-merge.log.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/internal/safemerge"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "\nsafe-merge: %v\n", err)
		os.Exit(1)
	}
}

// options holds the parsed command line.
type options struct {
	repo, branch, mode, message, reason, wayfinder string
	emergency                                      bool
	timeout                                        time.Duration
	help                                           bool
}

// parseArgs turns argv into options. String-valued flags are table-driven to
// keep the parser flat; --emergency, --timeout, and --help are the only
// special forms.
func parseArgs(argv []string) (options, error) {
	opts := options{mode: string(safemerge.ModeSquash), timeout: safemerge.DefaultTimeout}
	strFlags := map[string]*string{
		"--repo": &opts.repo, "--branch": &opts.branch, "--mode": &opts.mode,
		"--message": &opts.message, "-m": &opts.message,
		"--wayfinder": &opts.wayfinder, "--reason": &opts.reason,
	}
	for i := 0; i < len(argv); i++ {
		arg := argv[i]
		value := func() (string, bool) {
			if i+1 >= len(argv) {
				return "", false
			}
			i++
			return argv[i], true
		}
		switch arg {
		case "-h", "--help":
			opts.help = true
			return opts, nil
		case "--emergency":
			opts.emergency = true
		case "--timeout":
			v, ok := value()
			if !ok {
				return opts, fmt.Errorf("--timeout requires a value")
			}
			d, err := time.ParseDuration(v)
			if err != nil {
				return opts, fmt.Errorf("invalid --timeout %q: %w", v, err)
			}
			opts.timeout = d
		default:
			dst, known := strFlags[arg]
			if !known {
				return opts, fmt.Errorf("unknown argument %q (see --help)", arg)
			}
			v, ok := value()
			if !ok {
				return opts, fmt.Errorf("%s requires a value", arg)
			}
			*dst = v
		}
	}
	return opts, nil
}

func run(argv []string) error {
	opts, err := parseArgs(argv)
	if err != nil {
		return err
	}
	if opts.help {
		fmt.Print(usage)
		return nil
	}

	req := &safemerge.Request{
		Repo:      opts.repo,
		Branch:    opts.branch,
		Mode:      safemerge.Mode(opts.mode),
		Message:   opts.message,
		Emergency: opts.emergency,
		Reason:    opts.reason,
	}

	// Resolve the wayfinder session unless this is an audited emergency.
	if !opts.emergency {
		dir, err := safemerge.ResolveSessionDir(opts.wayfinder)
		if err != nil {
			return err
		}
		sess, err := safemerge.LoadSession(dir)
		if err != nil {
			return err
		}
		req.Session = &sess
	}

	if err := req.Validate(); err != nil {
		return err
	}

	rec := safemerge.AuditRecord{
		Time:      time.Now().UTC().Format(time.RFC3339),
		Repo:      req.Repo,
		Branch:    req.Branch,
		Mode:      string(req.Mode),
		Emergency: req.Emergency,
		Reason:    req.Reason,
	}
	if req.Session != nil {
		rec.SessionID = req.Session.ID
	}

	res, mergeErr := safemerge.NewMerger(opts.timeout).Run(req)
	if res != nil {
		rec.RolledBack = res.RolledBack
	}
	if mergeErr != nil {
		rec.ExitCode = 1
		rec.Error = mergeErr.Error()
	}
	if auditErr := safemerge.AppendAudit(os.Getenv("HOME"), rec); auditErr != nil {
		// Audit is best-effort: warn but do not mask the merge outcome.
		fmt.Fprintf(os.Stderr, "safe-merge: warning: audit log not written: %v\n", auditErr)
	}
	if mergeErr != nil {
		return mergeErr
	}

	fmt.Printf("Landed %q into %s (%s mode). Commit message:\n\n%s\n\n"+
		"Next: push with safe-push (safe-merge never pushes).\n",
		req.Branch, req.Repo, req.Mode, res.CommitMessage)
	return nil
}

const usage = `safe-merge — land a feature branch into a golden ~/src checkout, atomically and audited.

Usage:
  safe-merge --repo <dir> --branch <feature> [--mode squash|no-ff] [--message <msg>]
             (--wayfinder <project-dir> | --emergency --reason "<why>") [--timeout <dur>]

Flags:
  --repo <dir>            golden checkout to merge into (the destination)
  --branch <name>        feature branch to land
  --mode squash|no-ff    squash (default, linear) or an explicit merge commit
  --message, -m <msg>    commit/merge headline (a default is derived if omitted)
  --wayfinder <dir>      wayfinder project dir whose WAYFINDER-STATUS.md is in_progress
  --emergency            land without a session (requires --reason; audited)
  --reason <why>         why no wayfinder session exists (with --emergency)
  --timeout <dur>        kill the merge after this long (default 60s)
  -h, --help             show this help

Why this exists:
  The golden-tree landing step is ` + "`git -C ~/src/<repo> merge --squash <branch>`" + ` then a
  commit — an all-or-nothing chain whose raw allow-list pattern
  (git -C ~/src/* merge *) also permits dangerous merges and caused a 5.3h
  deploy stall (bead ce-f87f). safe-merge runs the merge+commit as one unit,
  refuses a dirty or off-default-branch tree, rolls back on conflict, requires
  a wayfinder trace, and never pushes or force-anything.
`
