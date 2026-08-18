package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// PreMergeCapable reports whether the named check could have run before the
// merge. Two things can rule that out: the workflow has no pull_request
// trigger at all, or the job producing the check is guarded to events a pull
// request never fires.
//
// Without this, a schedule-only workflow going red on main is classified as a
// check that "never reported on the PR", and the retro sends the reader off to
// widen a path filter that was never involved. Drift detectors, dependency
// audits, and infrastructure reconciliation are not escapes; they are
// post-merge detectors doing their job.
//
// Unknown workflows and unknown jobs default to true, which is the conservative
// direction: a real escape misreported as post-merge-only would be quietly
// dropped, whereas a post-merge job misreported as an escape is merely noisy.
func (o sweepOptions) PreMergeCapable(workflowName, checkName string) bool {
	wf, known := workflowIndex()[workflowName]
	if !known {
		return true
	}
	if !wf.PreMerge {
		return false
	}
	// The workflow runs on pull requests, but this particular job may not.
	// `AGM Tagged Sweep` inside CI is the standing example: CI is
	// pull-request-capable, that job is guarded to schedule and dispatch, and
	// treating its failure as an escape sends the reader after a path filter
	// that was never in play.
	if capable, seen := wf.Jobs[checkName]; seen {
		return capable
	}
	return true
}

type workflowInfo struct {
	// PreMerge is whether the workflow's `on:` block names a pull_request
	// trigger.
	PreMerge bool
	// Jobs maps a job's display name to whether its `if:` condition permits a
	// pull-request event. Jobs with no `if:` are absent — absent means "no
	// job-level restriction", which the lookup reads as capable.
	Jobs map[string]bool
}

var (
	triggersOnce    sync.Once
	workflowsByName map[string]workflowInfo
)

// workflowIndex maps a workflow's display name to its trigger facts. Parsed
// from the checked-out .github/workflows rather than the API so it reflects the
// tree under test.
//
// Deliberately a text scan rather than a YAML parse: the questions are whether
// a `pull_request` key appears in the `on:` block and whether a job's `if:`
// excludes pull-request events. Pulling a YAML dependency into a tool that
// otherwise shells out to gh is not worth it.
func workflowIndex() map[string]workflowInfo {
	triggersOnce.Do(func() {
		workflowsByName = map[string]workflowInfo{}
		// Both extensions: GitHub honours .yaml as well as .yml, and a glob on
		// one of them silently drops half the possible workflows.
		entries, err := os.ReadDir(".github/workflows")
		if err != nil {
			return
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if ext := filepath.Ext(entry.Name()); ext != ".yml" && ext != ".yaml" {
				continue
			}
			data, err := os.ReadFile(filepath.Join(".github/workflows", entry.Name()))
			if err != nil {
				continue
			}
			name, info, ok := parseWorkflow(string(data))
			if ok {
				workflowsByName[name] = info
			}
		}
	})
	return workflowsByName
}

// pullRequestTriggers are the only two `on:` keys that make a workflow run
// before a merge. Matched as whole keys: `pull_request_review` and
// `pull_request_review_comment` fire on review activity, not on the pull
// request itself, and a prefix test folds them in — which marks the Claude Code
// workflow pre-merge capable when it has neither trigger.
var pullRequestTriggers = []string{"pull_request", "pull_request_target"}

// parseWorkflow extracts the workflow's display name, whether its trigger block
// names a pull request, and which named jobs are guarded away from pull
// requests. Returns ok=false when there is no name to key on.
func parseWorkflow(content string) (string, workflowInfo, bool) {
	p := workflowParser{
		info:         workflowInfo{Jobs: map[string]bool{}},
		jobKeyIndent: jobIndent(content),
	}

	for line := range strings.SplitSeq(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Top-level keys are unindented and close whatever block was open.
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			p.topLevel(line)
			continue
		}
		p.nested(line, trimmed)
	}
	p.flushJob()

	return p.name, p.info, p.name != ""
}

// workflowParser carries the line-scanner's position through a workflow file:
// which top-level block is open, and which job block is being read.
type workflowParser struct {
	info         workflowInfo
	jobKeyIndent int

	name       string
	inTriggers bool
	inJobs     bool
	// jobName is the display name of the job block currently open; jobIf is
	// its `if:` condition. Both are flushed into info.Jobs when the block ends.
	jobName string
	jobIf   string
}

func (p *workflowParser) flushJob() {
	if p.jobName != "" && p.jobIf != "" {
		p.info.Jobs[p.jobName] = ifPermitsPullRequest(p.jobIf)
	}
	p.jobName, p.jobIf = "", ""
}

func (p *workflowParser) topLevel(line string) {
	p.flushJob()
	p.inTriggers, p.inJobs = false, false

	switch {
	case p.name == "" && strings.HasPrefix(line, "name:"):
		p.name = unquote(strings.TrimPrefix(line, "name:"))
	case strings.HasPrefix(line, "on:"):
		p.inTriggers = true
		// Inline forms: `on: pull_request` or `on: [push, pull_request]`.
		if namesPullRequestTrigger(strings.TrimPrefix(line, "on:")) {
			p.info.PreMerge = true
		}
	case strings.HasPrefix(line, "jobs:"):
		p.inJobs = true
	}
}

func (p *workflowParser) nested(line, trimmed string) {
	if p.inTriggers {
		if isPullRequestTriggerKey(trimmed) {
			p.info.PreMerge = true
		}
		return
	}
	if !p.inJobs {
		return
	}

	// Job ids sit at exactly one indent level under `jobs:`; anything deeper
	// belongs to the job currently open.
	if indentOf(line) == p.jobKeyIndent {
		p.flushJob()
		return
	}
	if rest, found := strings.CutPrefix(trimmed, "name:"); found && p.jobName == "" {
		p.jobName = unquote(rest)
		return
	}
	if rest, found := strings.CutPrefix(trimmed, "if:"); found && p.jobIf == "" {
		p.jobIf = strings.TrimSpace(rest)
	}
}

// isPullRequestTriggerKey matches a whole `pull_request:` / `pull_request_target:`
// mapping key, with or without quotes, and the bare list-item forms.
func isPullRequestTriggerKey(trimmed string) bool {
	candidate := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
	candidate = strings.TrimSuffix(candidate, ":")
	candidate = strings.Trim(candidate, `"'`)
	return slices.Contains(pullRequestTriggers, candidate)
}

// namesPullRequestTrigger handles the inline `on:` forms, where the triggers sit
// on the same line as the key.
func namesPullRequestTrigger(rest string) bool {
	rest = strings.Trim(strings.TrimSpace(rest), "[]")
	for field := range strings.SplitSeq(rest, ",") {
		if isPullRequestTriggerKey(strings.TrimSpace(field)) {
			return true
		}
	}
	return false
}

// ifPermitsPullRequest reports whether a job-level `if:` could ever be true for
// a pull-request event. Only conditions that pin `github.event_name` to a fixed
// set are decidable by inspection; anything else is treated as permitting, which
// keeps the default on the conservative side.
func ifPermitsPullRequest(condition string) bool {
	if !strings.Contains(condition, "github.event_name") {
		return true
	}
	// A condition that names event_name at all but never names a pull-request
	// event can only be satisfied by some other event.
	for _, trigger := range pullRequestTriggers {
		if strings.Contains(condition, trigger) {
			return true
		}
	}
	// `github.event_name != 'schedule'` is a negative test and still admits
	// pull requests.
	return strings.Contains(condition, "!=")
}

// jobIndent finds the indent width used for job ids, so nested keys can be told
// apart from a new job. Falls back to two spaces, which is what every workflow
// in this repository uses.
func jobIndent(content string) int {
	inJobs := false
	for line := range strings.SplitSeq(content, "\n") {
		if strings.HasPrefix(line, "jobs:") {
			inJobs = true
			continue
		}
		if !inJobs {
			continue
		}
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if indent := indentOf(line); indent > 0 {
			return indent
		}
		return 2
	}
	return 2
}

func indentOf(line string) int {
	return len(line) - len(strings.TrimLeft(line, " \t"))
}

func unquote(s string) string {
	return strings.Trim(strings.TrimSpace(s), `"'`)
}
