package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// NeedResult is one entry of GitHub's `needs` context.
type NeedResult struct {
	Result  string            `json:"result"`
	Outputs map[string]string `json:"outputs"`
}

// AuditInput is everything the gateway's skip audit reasons over.
type AuditInput struct {
	Needs     map[string]NeedResult
	EventName string
}

// AuditReport is the outcome of one gateway evaluation.
type AuditReport struct {
	Log        []string
	Failures   []string
	Violations []string
}

// Failed reports whether the gateway must exit non-zero.
func (r AuditReport) Failed() bool { return len(r.Failures) > 0 || len(r.Violations) > 0 }

// ParseNeeds decodes `toJSON(needs)`.
func ParseNeeds(raw string) (map[string]NeedResult, error) {
	needs := map[string]NeedResult{}
	if strings.TrimSpace(raw) == "" {
		return needs, nil
	}
	if err := json.Unmarshal([]byte(raw), &needs); err != nil {
		return nil, fmt.Errorf("parse needs context: %w", err)
	}
	return needs, nil
}

// detectorJob is the reusable change-detection job every scoped consumer
// depends on. Its name must match the job id in the calling workflow.
const detectorJob = "changes"

// Audit implements both halves of the CI Gateway.
//
// Half one is the ordinary aggregate: any upstream failure or cancellation
// fails the gateway.
//
// Half two is the part most published versions of this pattern omit. Because
// GitHub reports a job skipped by an `if:` condition as `skipped`, and branch
// protection accepts `skipped` as satisfying a required status check, a filter
// that wrongly excludes a relevant PR does not turn CI red — it turns CI
// *absent*, and the PR merges green with the gate disabled. So the gateway
// re-derives which jobs should have run from the same selection the jobs
// consumed, and fails if any of them reports `skipped`.
func Audit(in AuditInput) AuditReport {
	var rep AuditReport

	names := make([]string, 0, len(in.Needs))
	for name := range in.Needs {
		names = append(names, name)
	}
	sort.Strings(names)

	rep.Log = append(rep.Log, "Upstream job results:")
	for _, name := range names {
		rep.Log = append(rep.Log, fmt.Sprintf("  %s: %s", name, in.Needs[name].Result))
		switch in.Needs[name].Result {
		case "failure", "cancelled":
			rep.Failures = append(rep.Failures, fmt.Sprintf("%s -> %s", name, in.Needs[name].Result))
		}
	}

	detector := in.Needs[detectorJob]
	// A detector that did not succeed publishes no outputs. Treating that as
	// "nothing is relevant" would let a checkout or runner failure disable
	// every scoped gate at once, so it means the opposite: everything was
	// relevant, and every consumer was required to run.
	detectorOK := detector.Result == "success"
	sel := func(key string) bool {
		if !detectorOK {
			return true
		}
		return detector.Outputs[key] == "true"
	}
	selGo, selAGM, selEngram := sel("go"), sel("agm"), sel("engram")

	rep.Log = append(rep.Log, fmt.Sprintf(
		"Selection: detector=%s go=%v agm=%v engram=%v event=%s",
		detector.Result, selGo, selAGM, selEngram, in.EventName))

	// Expectations are re-derived here rather than shared with the workflow so
	// that editing a job's `if:` without editing this table makes the two
	// disagree and the gateway fail loudly. That divergence is the signal.
	expect := []struct {
		job string
		run bool
	}{
		// `ci` carries no selection in its job-level `if:`; it always runs and
		// gates its own steps, because a skipped matrix job never emits the
		// expanded "Build & Test (<os>)" contexts branch protection requires.
		{"ci", true},
		{"integration-tests", in.EventName == "pull_request" && selGo},
		{"agm-codex-contracts", selGo && selAGM},
		{"engram-storage-hardening", selGo && selEngram},
	}

	rep.Log = append(rep.Log, "Skip audit:")
	for _, e := range expect {
		got, ok := in.Needs[e.job]
		result := "missing"
		if ok {
			result = got.Result
		}
		rep.Log = append(rep.Log, fmt.Sprintf("  %s: result=%s expected_to_run=%v", e.job, result, e.run))
		if !e.run {
			continue
		}
		switch result {
		case "skipped":
			rep.Violations = append(rep.Violations, fmt.Sprintf(
				"%s was SKIPPED but the change set says it was relevant", e.job))
		case "missing":
			rep.Violations = append(rep.Violations, fmt.Sprintf(
				"%s is not in the gateway's `needs` but was expected to run", e.job))
		}
	}
	return rep
}
