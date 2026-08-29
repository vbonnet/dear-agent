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
	ruleName          = "bash-20-line-limit"
	lineLimit         = 20
	sunsetWarningDays = 30
	storePath         = ".github/language-policy/exceptions.jsonl"
	baselinePath      = ".github/language-policy/baseline.json"
	storeDir          = ".github/language-policy"
	usageString       = `language-policy — enforce the repository shell-script policy.

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
                 waiver, waivers whose sunset date has passed, and waivers
                 expiring within 30 UTC calendar days.
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
	store, err := LoadStore(f)
	if cerr := f.Close(); cerr != nil && err == nil {
		err = fmt.Errorf("closing waiver store: %w", cerr)
	}
	return store, err
}

// readNULList parses a NUL-delimited pathname list. NUL delimiting is what lets
// paths containing spaces or newlines survive; a newline-delimited list would
// split such a path in half and silently skip the script.
func readNULList(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
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

	tests, err := BuildTestIndex(*repoRoot)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	res, err := scan(*repoRoot, store, tests, paths, now)
	if err != nil {
		return err
	}

	fmt.Printf("rule=%s limit=%d checked=%d compliant=%d tested=%d waived=%d violations=%d\n",
		ruleName, lineLimit, res.checked, res.compliant, res.tested, res.waived, len(res.violations))
	return report(res.violations, *github)
}

// scanResult is the outcome of checking a set of paths.
type scanResult struct {
	checked    int
	compliant  int
	tested     int
	waived     int
	violations []violation
}

// scan checks each in-scope path against the limit and the waiver store.
func scan(repoRoot string, store *Store, tests *TestIndex, paths []string, now time.Time) (scanResult, error) {
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
		closeErr := f.Close()
		if cerr != nil {
			return res, fmt.Errorf("counting %s: %w", p, cerr)
		}
		if closeErr != nil {
			return res, fmt.Errorf("closing %s: %w", full, closeErr)
		}
		res.checked++
		switch {
		case n <= lineLimit:
			res.compliant++
		case tests.Covered(p):
			// A long script that is tested is not the risk this rule exists
			// to catch. Crediting a test makes the escape hatch productive:
			// the way out is to cover the script, not to file a waiver.
			res.tested++
		case store.Active(ruleName, p, now):
			res.waived++
		default:
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
			fmt.Println(githubAnnotation("error", v.path, fmt.Sprintf(
				"%s is %d countable lines (limit %d), untested, and has no active waiver. Add a test under %s/, shorten it, or file a waiver.",
				v.path, v.count, lineLimit, testDir)))
		} else {
			fmt.Printf("  %-60s %3d lines (+%d over limit, untested)\n", v.path, v.count, v.count-lineLimit)
		}
	}
	if len(violations) > 0 {
		return fmt.Errorf("%d shell script(s) exceed the %d-line limit while untested and unwaived", len(violations), lineLimit)
	}
	return nil
}

func runSweep(args []string) error {
	return runSweepAt(args, time.Now().UTC())
}

func runSweepAt(args []string, now time.Time) error {
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
	expired := store.Expired(now)
	for _, e := range expired {
		sunset := ""
		if e.Sunset != nil {
			sunset = *e.Sunset
		}
		if *github {
			fmt.Println(githubAnnotation("warning", e.Path,
				fmt.Sprintf("waiver for %s expired on %s", e.Path, sunset)))
		} else {
			fmt.Printf("expired waiver: %s (sunset %s)\n", e.Path, sunset)
		}
	}
	expiring := store.ExpiringWithin(now, sunsetWarningDays)
	for _, e := range expiring {
		fmt.Println(formatExpiringWarning(e, sunsetWarningDays, *github))
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
	scanned := 0
	defer func() {
		fmt.Printf("sweep: %d waiver(s) total, %d expired, %d expiring within %d days, %d script(s) scanned\n",
			len(store.All), len(expired), len(expiring), sunsetWarningDays, scanned)
	}()
	tests, terr := BuildTestIndex(*repoRoot)
	if terr != nil {
		return terr
	}
	res, err := scan(*repoRoot, store, tests, paths, now)
	scanned = res.checked
	if err != nil {
		return err
	}

	dead := reportDeadWaivers(*repoRoot, store, *github)
	if err := report(res.violations, *github); err != nil {
		return err
	}
	calibration(res, len(store.All), dead, *github)
	return nil
}

func formatExpiringWarning(e Exception, days int, github bool) string {
	sunset := ""
	if e.Sunset != nil {
		sunset = *e.Sunset
	}
	action := fmt.Sprintf(
		"remove it if no longer needed, otherwise shorten the script, add a test under %s/, or renew it with explicit owner approval before expiry",
		testDir)
	if github {
		return githubAnnotation("warning", e.Path, fmt.Sprintf(
			"%s waiver expires on %s (within %d days); %s",
			e.Rule, sunset, days, action))
	}
	return fmt.Sprintf("expiring waiver: %s (rule %s, sunset %s, within %d days); %s",
		e.Path, e.Rule, sunset, days, action)
}

func githubAnnotation(level, path, message string) string {
	properties := ""
	if path != "" {
		properties = " file=" + escapeWorkflowCommandProperty(path)
	}
	return "::" + level + properties + "::" + escapeWorkflowCommandData(message)
}

func escapeWorkflowCommandData(s string) string {
	return strings.NewReplacer(
		"%", "%25",
		"\r", "%0D",
		"\n", "%0A",
	).Replace(s)
}

func escapeWorkflowCommandProperty(s string) string {
	return strings.NewReplacer(
		":", "%3A",
		",", "%2C",
	).Replace(escapeWorkflowCommandData(s))
}

// reportDeadWaivers names waivers whose target no longer exists and returns how
// many there were. Reporting them is what lets the store shrink instead of only
// ever growing: 26 of the original 110 pointed at deleted files.
func reportDeadWaivers(repoRoot string, store *Store, github bool) int {
	n := 0
	for _, e := range store.All {
		st, serr := os.Stat(filepath.Join(repoRoot, e.Path))
		if serr == nil && st.Mode().IsRegular() {
			continue
		}
		n++
		if github {
			fmt.Println(githubAnnotation("warning", "",
				fmt.Sprintf("waiver for %s references a file that no longer exists; remove it", e.Path)))
		} else {
			fmt.Printf("dead waiver (file gone): %s\n", e.Path)
		}
	}
	return n
}

// calibration is the policy-agnostic canary this rule lacked.
//
// A rule whose waiver count exceeds the number of files that actually comply
// is not enforcing anything; it is a list of exemptions with a rule attached.
// This repository reached 110 waivers against 22 compliant scripts before
// anyone noticed, because nothing was watching the ratio. Any future rule with
// an exemption list gets the same canary for free by reusing this check.
func calibration(res scanResult, waiverCount, deadCount int, github bool) {
	passing := res.compliant + res.tested
	fmt.Printf("calibration: compliant=%d tested=%d passing=%d waivers=%d dead_waivers=%d\n",
		res.compliant, res.tested, passing, waiverCount, deadCount)
	if waiverCount > passing {
		// Reported, not failed. The ratchet in verify-store is what enforces
		// (the count may not grow); failing here as well would leave the
		// nightly permanently red with no path to green except clearing the
		// whole backlog at once, and a permanently red signal is a muted one.
		msg := fmt.Sprintf(
			"policy %s is still inverted: %d waivers against %d passing scripts. "+
				"Each waiver clears by adding a test under %s/ or shortening the script. "+
				"Lower max_waivers in %s as they clear.",
			ruleName, waiverCount, passing, testDir, baselinePath)
		if github {
			fmt.Println(githubAnnotation("warning", "", msg))
		} else {
			fmt.Printf("WARNING: %s\n", msg)
		}
	}
}

// binaryStoreExtensions are the shapes a resurrected binary waiver store would
// most plausibly take. The policy store must stay diffable and blameable, so a
// binary sibling is a hard failure rather than a warning.
var binaryStoreExtensions = []string{".db", ".sqlite", ".sqlite3", ".db3"}

// binaryStoreOffenders returns store-directory entries whose extension
// matches a known binary format, which would defeat the waiver store's
// git-blame attributability requirement.
func binaryStoreOffenders(entries []os.DirEntry) []string {
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
	return offenders
}

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
	offenders := binaryStoreOffenders(entries)
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
	bf, err := os.Open(filepath.Join(*repoRoot, baselinePath))
	if err != nil {
		return fmt.Errorf("opening %s: %w", baselinePath, err)
	}
	baseline, berr := LoadBaseline(bf)
	if cerr := bf.Close(); cerr != nil && berr == nil {
		berr = cerr
	}
	if berr != nil {
		return fmt.Errorf("%s: %w", baselinePath, berr)
	}
	if err := baseline.CheckRatchet(ruleName, len(store.All)); err != nil {
		return err
	}

	fmt.Printf("verify-store: %s OK (%d waivers, text, sorted, no duplicates, within ratchet)\n", storePath, len(store.All))
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
		return fmt.Errorf("stating %s: %w", target, err)
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
		return fmt.Errorf("writing %s: %w", target, err)
	}
	fmt.Printf("format: rewrote %s (%d waivers)\n", storePath, len(store.All))
	return nil
}
