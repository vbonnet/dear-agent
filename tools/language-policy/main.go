package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	ruleName    = "bash-20-line-limit"
	lineLimit   = 20
	storePath   = ".github/language-policy/exceptions.jsonl"
	storeDir    = ".github/language-policy"
	usageString = `language-policy — enforce the repository shell-script policy.

Usage:
  language-policy check [--files-from <file>] [--all] [--github]
  language-policy sweep [--files-from <file>] [--github]
  language-policy verify-store
  language-policy format

Commands:
  check          Check scripts against the line limit and the waiver store.
                 --files-from reads a NUL-delimited path list (as written by
                 'git diff -z --name-only'); --all checks every path given on
                 the command line instead.
  sweep          Whole-repo scan: report over-limit scripts with no active
                 waiver, plus waivers whose sunset date has passed.
  format         Rewrite the waiver store in canonical form (sorted, compact).
                 Run this after hand-editing it.
  verify-store   Validate the waiver store: text-only, parseable, sorted, no
                 duplicates, and no binary store reintroduced alongside it.
`
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usageString)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "check":
		err = runCheck(os.Args[2:])
	case "sweep":
		err = runSweep(os.Args[2:])
	case "verify-store":
		err = runVerifyStore(os.Args[2:])
	case "format":
		err = runFormat(os.Args[2:])
	case "-h", "--help", "help":
		fmt.Print(usageString)
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", os.Args[1], usageString)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "language-policy: %v\n", err)
		os.Exit(1)
	}
}

func loadStore(repoRoot string) (*Store, error) {
	f, err := os.Open(filepath.Join(repoRoot, storePath))
	if err != nil {
		return nil, fmt.Errorf("opening waiver store: %w", err)
	}
	defer func() { _ = f.Close() }()
	return LoadStore(f)
}

// readNULList parses a NUL-delimited pathname list. NUL delimiting is what lets
// paths containing spaces or newlines survive; a newline-delimited list would
// split such a path in half and silently skip the script.
func readNULList(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []string
	for part := range bytes.SplitSeq(b, []byte{0}) {
		if s := strings.TrimSpace(string(part)); s != "" {
			out = append(out, s)
		}
	}
	return out, nil
}

type violation struct {
	path  string
	count int
}

func runCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	filesFrom := fs.String("files-from", "", "read NUL-delimited paths from this file")
	all := fs.Bool("all", false, "check the paths given as arguments")
	github := fs.Bool("github", false, "emit GitHub Actions annotations")
	repoRoot := fs.String("repo", ".", "repository root")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var paths []string
	switch {
	case *filesFrom != "":
		p, err := readNULList(*filesFrom)
		if err != nil {
			return fmt.Errorf("reading --files-from: %w", err)
		}
		paths = p
	case *all:
		paths = fs.Args()
	default:
		return fmt.Errorf("one of --files-from or --all is required")
	}

	store, err := loadStore(*repoRoot)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	res, err := scan(*repoRoot, store, paths, now)
	if err != nil {
		return err
	}

	fmt.Printf("rule=%s limit=%d checked=%d compliant=%d waived=%d violations=%d\n",
		ruleName, lineLimit, len(paths), res.compliant, res.waived, len(res.violations))
	return report(res.violations, *github)
}

// scanResult is the outcome of checking a set of paths.
type scanResult struct {
	compliant  int
	waived     int
	violations []violation
}

// scan checks each in-scope path against the limit and the waiver store.
func scan(repoRoot string, store *Store, paths []string, now time.Time) (scanResult, error) {
	var res scanResult
	for _, p := range paths {
		if !InScope(p) {
			continue
		}
		// A path can be listed in a diff and then deleted or renamed away by
		// a later commit in the same range; that is not a violation.
		full := filepath.Join(repoRoot, normalizePath(p))
		// Skip anything that is not a regular file: a directory would fail
		// the read below with EISDIR and abort the whole scan, and a dangling
		// symlink is not a script to check.
		if st, serr := os.Stat(full); serr != nil || !st.Mode().IsRegular() {
			continue
		}
		f, err := os.Open(full)
		if err != nil {
			continue
		}
		n, cerr := CountLines(f)
		_ = f.Close()
		if cerr != nil {
			return res, fmt.Errorf("counting %s: %w", p, cerr)
		}
		switch {
		case n <= lineLimit:
			res.compliant++
		default:
			if store.Active(ruleName, p, now) {
				res.waived++
				continue
			}
			res.violations = append(res.violations, violation{path: normalizePath(p), count: n})
		}
	}
	return res, nil
}

// report prints violations and returns a non-nil error when there are any, so
// the process exit code carries the verdict.
func report(violations []violation, github bool) error {
	for _, v := range violations {
		if github {
			fmt.Printf("::error file=%s::%s is %d countable lines (limit %d) with no active waiver\n",
				v.path, v.path, v.count, lineLimit)
		} else {
			fmt.Printf("  %-60s %3d lines (+%d over limit)\n", v.path, v.count, v.count-lineLimit)
		}
	}
	if len(violations) > 0 {
		return fmt.Errorf("%d shell script(s) exceed the %d-line limit without an active waiver", len(violations), lineLimit)
	}
	return nil
}

func runSweep(args []string) error {
	fs := flag.NewFlagSet("sweep", flag.ExitOnError)
	github := fs.Bool("github", false, "emit GitHub Actions annotations")
	repoRoot := fs.String("repo", ".", "repository root")
	filesFrom := fs.String("files-from", "", "read NUL-delimited paths from this file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	store, err := loadStore(*repoRoot)
	if err != nil {
		return err
	}
	now := time.Now().UTC()

	expired := store.Expired(now)
	for _, e := range expired {
		sunset := ""
		if e.Sunset != nil {
			sunset = *e.Sunset
		}
		if *github {
			fmt.Printf("::warning file=%s::waiver for %s expired on %s\n", e.Path, e.Path, sunset)
		} else {
			fmt.Printf("expired waiver: %s (sunset %s)\n", e.Path, sunset)
		}
	}

	paths := fs.Args()
	if *filesFrom != "" {
		p, ferr := readNULList(*filesFrom)
		if ferr != nil {
			return fmt.Errorf("reading --files-from: %w", ferr)
		}
		paths = p
	}
	// Report the waiver census before the error return below, so a failing
	// sweep still tells the reader how large the backlog is.
	defer fmt.Printf("sweep: %d waiver(s) total, %d expired, %d script(s) scanned\n", len(store.All), len(expired), len(paths))
	res, err := scan(*repoRoot, store, paths, now)
	if err != nil {
		return err
	}
	return report(res.violations, *github)
}

// binaryStoreExtensions are the shapes a resurrected binary waiver store would
// most plausibly take. The policy store must stay diffable and blameable, so a
// binary sibling is a hard failure rather than a warning.
var binaryStoreExtensions = []string{".db", ".sqlite", ".sqlite3", ".db3"}

func runVerifyStore(args []string) error {
	fs := flag.NewFlagSet("verify-store", flag.ExitOnError)
	repoRoot := fs.String("repo", ".", "repository root")
	if err := fs.Parse(args); err != nil {
		return err
	}

	entries, err := os.ReadDir(filepath.Join(*repoRoot, storeDir))
	if err != nil {
		return fmt.Errorf("reading %s: %w", storeDir, err)
	}
	var offenders []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		for _, bad := range binaryStoreExtensions {
			if ext == bad {
				offenders = append(offenders, filepath.Join(storeDir, e.Name()))
			}
		}
	}
	if len(offenders) > 0 {
		return fmt.Errorf("binary waiver store(s) present: %s\n"+
			"The waiver store must stay a text format so each waiver is attributable with 'git blame'.\n"+
			"Add waivers to %s instead", strings.Join(offenders, ", "), storePath)
	}

	raw, err := os.ReadFile(filepath.Join(*repoRoot, storePath))
	if err != nil {
		return fmt.Errorf("reading %s: %w", storePath, err)
	}
	// A NUL byte means something wrote a binary blob to the .jsonl path, which
	// would defeat the guard above by keeping the text extension.
	if bytes.IndexByte(raw, 0) >= 0 {
		return fmt.Errorf("%s contains NUL bytes; the waiver store must be text", storePath)
	}

	store, err := LoadStore(bytes.NewReader(raw))
	if err != nil {
		return fmt.Errorf("%s: %w", storePath, err)
	}
	if err := CheckSorted(store.All); err != nil {
		return fmt.Errorf("%s: %w", storePath, err)
	}
	fmt.Printf("verify-store: %s OK (%d waivers, text, sorted, no duplicates)\n", storePath, len(store.All))
	return nil
}

// runFormat rewrites the store in canonical form. Hand-editing a JSONL file is
// expected and fine; this is how an edited store is normalised back to sorted,
// compact lines so the next machine write produces no incidental diff.
func runFormat(args []string) error {
	fs := flag.NewFlagSet("format", flag.ExitOnError)
	repoRoot := fs.String("repo", ".", "repository root")
	if err := fs.Parse(args); err != nil {
		return err
	}
	target := filepath.Join(*repoRoot, storePath)
	// Reuse the file's current mode rather than imposing one: format rewrites
	// a tracked file in place and has no business changing its permissions.
	st, err := os.Stat(target)
	if err != nil {
		return err
	}
	f, err := os.Open(target)
	if err != nil {
		return err
	}
	store, err := LoadStore(f)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	var buf bytes.Buffer
	if err := WriteStore(&buf, store.All); err != nil {
		return err
	}
	if err := os.WriteFile(target, buf.Bytes(), st.Mode().Perm()); err != nil {
		return err
	}
	fmt.Printf("format: rewrote %s (%d waivers)\n", storePath, len(store.All))
	return nil
}
