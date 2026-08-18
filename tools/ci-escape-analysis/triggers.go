package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// PreMergeCapable reports whether the named workflow has a pull_request or
// pull_request_target trigger — that is, whether it ever had a chance to run
// before the merge.
//
// Without this, a schedule-only workflow going red on main is classified as a
// check that "never reported on the PR", and the retro sends the reader off to
// widen a path filter that was never involved. Drift detectors, dependency
// audits, and infrastructure reconciliation are not escapes; they are
// post-merge detectors doing their job.
//
// Unknown workflows default to true, which is the conservative direction: a
// real escape misreported as post-merge-only would be quietly dropped, whereas
// a post-merge job misreported as an escape is merely noisy.
func (o sweepOptions) PreMergeCapable(workflowName string) bool {
	capable, known := workflowTriggers()[workflowName]
	if !known {
		return true
	}
	return capable
}

var (
	triggersOnce sync.Once
	triggersByWF map[string]bool
)

// workflowTriggers maps a workflow's display name to whether it triggers on a
// pull request. Parsed from the checked-out .github/workflows rather than the
// API so it reflects the tree under test.
//
// Deliberately a text scan rather than a YAML parse: the only question is
// whether a `pull_request` key appears in the `on:` block, and pulling a YAML
// dependency into a tool that otherwise shells out to gh is not worth it.
func workflowTriggers() map[string]bool {
	triggersOnce.Do(func() {
		triggersByWF = map[string]bool{}
		paths, err := filepath.Glob(".github/workflows/*.yml")
		if err != nil {
			return
		}
		for _, path := range paths {
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			name, capable, ok := parseWorkflow(string(data))
			if ok {
				triggersByWF[name] = capable
			}
		}
	})
	return triggersByWF
}

// parseWorkflow extracts the workflow's display name and whether its trigger
// block mentions a pull request. Returns ok=false when there is no name to key
// on.
func parseWorkflow(content string) (name string, preMerge bool, ok bool) {
	inTriggers := false
	for line := range strings.SplitSeq(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if rest, found := strings.CutPrefix(line, "name:"); found && name == "" {
			name = strings.Trim(strings.TrimSpace(rest), `"'`)
			continue
		}

		// Top-level keys are unindented. `on:` opens the trigger block; the
		// next unindented key closes it.
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			if strings.HasPrefix(line, "on:") {
				inTriggers = true
				// Inline form, e.g. `on: pull_request`.
				if strings.Contains(line, "pull_request") {
					preMerge = true
				}
				continue
			}
			inTriggers = false
			continue
		}

		if inTriggers && strings.HasPrefix(trimmed, "pull_request") {
			preMerge = true
		}
	}
	return name, preMerge, name != ""
}
