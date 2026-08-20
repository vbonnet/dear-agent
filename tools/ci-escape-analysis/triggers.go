package main

import (
	"os"
	"path/filepath"
	"regexp"
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
	// MainTriggered is whether the workflow still has any trigger that runs it
	// against main: a push, a schedule, a manual dispatch, or a merge queue.
	// A workflow that has none cannot be red on main today whatever its run
	// history says — Routing Enforcement dropped its push trigger and its last
	// main run is a failure from a tree that no longer exists.
	MainTriggered bool
	// Jobs maps a job's display name to whether its `if:` condition permits a
	// pull-request event. Jobs with no `if:` are absent — absent means "no
	// job-level restriction", which the lookup reads as capable.
	Jobs map[string]bool
}

var (
	triggersOnce    sync.Once
	workflowsByName map[string]workflowInfo
	workflowsByPath map[string]workflowInfo
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
		workflowsByPath = map[string]workflowInfo{}
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
				workflowsByPath[filepath.ToSlash(filepath.Join(".github/workflows", entry.Name()))] = info
			}
		}
	})
	return workflowsByName
}

// workflowPathIndex maps a workflow's repository path to its trigger facts.
// The sweep enumerates workflows by path (that is what the API returns and what
// `gh run list --workflow` takes), and needs the trigger facts BEFORE it has a
// run to read a display name from.
func workflowPathIndex() map[string]workflowInfo {
	workflowIndex()
	return workflowsByPath
}

// MainEvaluatingFile reports whether the workflow file at path can run against
// main at all. Unknown paths report true: GitHub can list a workflow whose file
// is no longer in the tree, and treating that as "cannot be red" would hide a
// real failure.
func (o sweepOptions) MainEvaluatingFile(workflowPath string) bool {
	wf, known := workflowPathIndex()[workflowPath]
	if !known {
		return true
	}
	return wf.MainTriggered
}

// pullRequestTriggers are the only two `on:` keys that make a workflow run
// before a merge. Matched as whole keys: `pull_request_review` and
// `pull_request_review_comment` fire on review activity, not on the pull
// request itself, and a prefix test folds them in — which marks the Claude Code
// workflow pre-merge capable when it has neither trigger.
var pullRequestTriggers = []string{"pull_request", "pull_request_target"}

// mainTriggers are the `on:` keys that can run a workflow against main.
// Everything else — issue_comment, issues, the pull_request_review family —
// fires on human interaction and says nothing about whether main builds.
var mainTriggers = []string{"push", "schedule", "workflow_dispatch", "merge_group", "repository_dispatch"}

// mainEvaluatingEvents are the run `event` values that actually evaluate main.
// `Claude Code` runs on the default branch for issue and review events, so a
// failed `@claude` invocation would otherwise be read as main being broken.
var mainEvaluatingEvents = map[string]bool{
	"push":                true,
	"schedule":            true,
	"workflow_dispatch":   true,
	"merge_group":         true,
	"repository_dispatch": true,
	"dynamic":             true,
}

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
		inline := strings.TrimPrefix(line, "on:")
		if namesTrigger(inline, pullRequestTriggers) {
			p.info.PreMerge = true
		}
		if namesTrigger(inline, mainTriggers) {
			p.info.MainTriggered = true
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
		if isTriggerKey(trimmed, mainTriggers) {
			p.info.MainTriggered = true
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
	return isTriggerKey(trimmed, pullRequestTriggers)
}

// isTriggerKey matches a whole `<trigger>:` mapping key, with or without quotes,
// and the bare list-item forms.
func isTriggerKey(trimmed string, triggers []string) bool {
	candidate := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
	candidate = strings.TrimSuffix(candidate, ":")
	candidate = strings.Trim(candidate, `"'`)
	return slices.Contains(triggers, candidate)
}

// namesTrigger handles the inline `on:` forms, where the triggers sit on the
// same line as the key.
func namesTrigger(rest string, triggers []string) bool {
	rest = strings.Trim(strings.TrimSpace(rest), "[]")
	for field := range strings.SplitSeq(rest, ",") {
		if isTriggerKey(strings.TrimSpace(field), triggers) {
			return true
		}
	}
	return false
}

// EvaluatesMain reports whether the named workflow still has a trigger that
// runs it against main. Unknown workflows default to true.
func (o sweepOptions) EvaluatesMain(workflowName string) bool {
	wf, known := workflowIndex()[workflowName]
	if !known {
		return true
	}
	return wf.MainTriggered
}

// eventNameComparison matches a `github.event_name == 'x'` or `!= 'x'` test,
// in either quote style.
var eventNameComparison = regexp.MustCompile(`github\.event_name\s*(==|!=)\s*['"]([^'"]*)['"]`)

// ifPermitsPullRequest reports whether a job-level `if:` could ever be true for
// a pull-request event. Only conditions that pin `github.event_name` to a fixed
// set are decidable by inspection; anything else is treated as permitting, which
// keeps the default on the conservative side.
//
// The operator has to be parsed, not just the event name. A substring test
// reads `github.event_name != 'pull_request'` as permission because the string
// contains "pull_request" — exactly inverting the condition. `Generate SBOM`
// and `Go Vulnerability Check` both use that guard, so the substring version
// classified their push failures as `never-ran` with path-filter advice.
func ifPermitsPullRequest(condition string) bool {
	matches := eventNameComparison.FindAllStringSubmatch(condition, -1)
	if len(matches) == 0 {
		// Either no event gate at all, or one written in a form this does not
		// decide (a `contains()` call, a matrix expression). Permit.
		return true
	}

	var equalsPR, notEqualsPR, equalsOther, notEqualsOther bool
	for _, m := range matches {
		isPR := slices.Contains(pullRequestTriggers, m[2])
		switch {
		case m[1] == "==" && isPR:
			equalsPR = true
		case m[1] == "!=" && isPR:
			notEqualsPR = true
		case m[1] == "==":
			equalsOther = true
		default:
			notEqualsOther = true
		}
	}

	switch {
	case equalsPR:
		return true // some branch of the condition admits a pull request
	case notEqualsPR:
		return false // explicitly prohibited on pull requests
	case equalsOther:
		return false // pinned to events a pull request never fires
	case notEqualsOther:
		return true // excludes some other event; pull requests still pass
	default:
		return true
	}
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
