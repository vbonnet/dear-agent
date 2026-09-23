package retrolint

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Options specifies configuration options for evaluating retrospectives.
type Options struct {
	RepoRoot        string
	RetrosDir       string
	BaselinePath    string
	Ratchet         bool
	AbsenceLookback time.Duration
	Files           []string
	Now             time.Time
}

// Run executes retrospective guard evaluation against configured targets.
func Run(ctx context.Context, opts Options) (*Report, error) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	repoRoot := opts.RepoRoot
	if repoRoot == "" {
		repoRoot = "."
	}
	absRepoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("resolving repo root: %w", err)
	}

	baseline, err := loadBaseline(opts.BaselinePath)
	if err != nil {
		return nil, err
	}

	report := &Report{
		Status: StatusPass,
	}

	if err := checkRatchetIfEnabled(ctx, absRepoRoot, opts.RetrosDir, opts.Ratchet, baseline, report); err != nil {
		return nil, err
	}

	files, err := discoverFiles(opts)
	if err != nil {
		return nil, err
	}

	hasAbsent, err := evaluateRetrospectives(ctx, absRepoRoot, files, baseline, opts, report)
	if err != nil {
		return nil, err
	}

	finalizeReportStatus(report, opts, hasAbsent)
	return report, nil
}

func loadBaseline(path string) (*Baseline, error) {
	if path == "" {
		return nil, nil
	}
	b, err := LoadBaselineFile(path)
	if err != nil {
		return nil, fmt.Errorf("loading baseline: %w", err)
	}
	return b, nil
}

func checkRatchetIfEnabled(ctx context.Context, repoRoot, retrosDir string, ratchet bool, baseline *Baseline, report *Report) error {
	if !ratchet || baseline == nil {
		return nil
	}
	ratchetErrors, err := CheckRatchet(ctx, repoRoot, retrosDir, baseline)
	if err != nil {
		return fmt.Errorf("checking ratchet: %w", err)
	}
	report.RatchetErrors = ratchetErrors
	return nil
}

func evaluateRetrospectives(ctx context.Context, repoRoot string, files []string, baseline *Baseline, opts Options, report *Report) (bool, error) {
	refTime := opts.Now
	if refTime.IsZero() {
		refTime = time.Now()
	}

	hasAbsent := false
	for _, file := range files {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		res, evalErr := EvaluateRetrospectiveFile(ctx, repoRoot, file, baseline)
		if evalErr != nil {
			res = &RetroResult{
				Path:   file,
				Status: StatusFail,
				Errors: []string{evalErr.Error()},
			}
		}

		if opts.AbsenceLookback > 0 {
			checkAbsence(res, file, refTime, opts.AbsenceLookback, &hasAbsent)
		}

		updateReportCounts(report, res)
		report.Results = append(report.Results, *res)
	}
	return hasAbsent, nil
}

func discoverFiles(opts Options) ([]string, error) {
	if len(opts.Files) > 0 {
		return opts.Files, nil
	}
	if opts.RetrosDir == "" {
		return nil, nil
	}
	var files []string
	err := filepath.WalkDir(opts.RetrosDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".md") {
			return nil
		}
		lower := strings.ToLower(name)
		if lower == "readme.md" || lower == "template.md" || strings.HasPrefix(lower, "status-") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking retrospectives directory: %w", err)
	}
	sort.Strings(files)
	return files, nil
}

func checkAbsence(res *RetroResult, file string, refTime time.Time, lookback time.Duration, hasAbsent *bool) {
	isRecent := isWithinWindow(res, file, refTime, lookback)
	if isRecent && res.Status != StatusPass && !res.Waived {
		res.Status = StatusAbsent
		res.Errors = append(res.Errors, fmt.Sprintf("retrospective added within %s absence lookback window lacks valid guards (RLINT-07)", lookback))
		*hasAbsent = true
	}
}

func isWithinWindow(res *RetroResult, file string, refTime time.Time, lookback time.Duration) bool {
	cutoff := refTime.Add(-lookback)
	if res.Date != "" {
		if t, err := time.Parse("2006-01-02", res.Date); err == nil {
			return !t.Before(cutoff)
		}
	}
	if fi, err := os.Stat(file); err == nil {
		return !fi.ModTime().Before(cutoff)
	}
	return false
}

func updateReportCounts(report *Report, res *RetroResult) {
	report.Evaluated++
	switch res.Status {
	case StatusPass, StatusPresent:
		report.Passed++
	case StatusWaived:
		report.Waived++
	case StatusFail, StatusAbsent:
		report.Failed++
	}
}

func finalizeReportStatus(report *Report, opts Options, hasAbsent bool) {
	if opts.AbsenceLookback > 0 {
		if hasAbsent {
			report.Status = StatusAbsent
		} else {
			report.Status = StatusPresent
		}
		return
	}

	if len(report.RatchetErrors) > 0 || report.Failed > 0 {
		report.Status = StatusFail
	} else {
		report.Status = StatusPass
	}
}
