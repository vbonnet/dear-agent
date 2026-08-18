package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

// These tests guard the failure mode that makes path-scoped CI dangerous: a
// mistake here does not turn a required check red, it makes the check
// *absent*, and GitHub treats a skipped or never-reported required context as
// satisfying branch protection just as well as a passing one. None of it is
// visible in a workflow log, so it is asserted here instead.

type workflowJob struct {
	Name     string    `yaml:"name"`
	If       string    `yaml:"if"`
	Needs    yaml.Node `yaml:"needs"`
	Uses     string    `yaml:"uses"`
	Strategy struct {
		Matrix map[string]yaml.Node `yaml:"matrix"`
	} `yaml:"strategy"`
	Steps []struct {
		Name string `yaml:"name"`
		If   string `yaml:"if"`
	} `yaml:"steps"`
}

type workflow struct {
	Name string                 `yaml:"name"`
	On   yaml.Node              `yaml:"on"`
	Jobs map[string]workflowJob `yaml:"jobs"`
}

func repoRoot() string { return filepath.Join("..", "..") }

func loadWorkflows(t *testing.T) map[string]workflow {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(repoRoot(), ".github", "workflows", "*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]workflow{}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var wf workflow
		if err := yaml.Unmarshal(raw, &wf); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		out[filepath.Base(p)] = wf
	}
	if len(out) == 0 {
		t.Fatal("no workflows found")
	}
	return out
}

func requiredContexts(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot(), ".github", "rulesets", "main.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ruleset struct {
		Rules []struct {
			Type       string `json:"type"`
			Parameters struct {
				RequiredStatusChecks []struct {
					Context string `json:"context"`
				} `json:"required_status_checks"`
			} `json:"parameters"`
		} `json:"rules"`
	}
	if err := json.Unmarshal(raw, &ruleset); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range ruleset.Rules {
		if r.Type != "required_status_checks" {
			continue
		}
		for _, c := range r.Parameters.RequiredStatusChecks {
			out = append(out, c.Context)
		}
	}
	if len(out) == 0 {
		t.Fatal("ruleset declares no required status checks")
	}
	return out
}

// matrixValues returns the scalar values of a matrix axis, ignoring the
// include/exclude keys, which do not create new axes.
func matrixValues(node yaml.Node) []string {
	var vals []string
	if node.Kind != yaml.SequenceNode {
		return nil
	}
	for _, item := range node.Content {
		if item.Kind == yaml.ScalarNode {
			vals = append(vals, item.Value)
		}
	}
	return vals
}

// contextNames returns every check-run name a job can report. GitHub expands a
// matrix into the job name when the name interpolates a matrix value, and
// otherwise appends the matrix values in parentheses.
func contextNames(id string, job workflowJob) []string {
	name := job.Name
	if name == "" {
		name = id
	}
	axes := map[string][]string{}
	for k, v := range job.Strategy.Matrix {
		if k == "include" || k == "exclude" {
			continue
		}
		if vals := matrixValues(v); len(vals) > 0 {
			axes[k] = vals
		}
	}
	if len(axes) == 0 {
		return []string{name}
	}
	keys := make([]string, 0, len(axes))
	for k := range axes {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	if strings.Contains(name, "${{ matrix.") {
		names := []string{name}
		for _, k := range keys {
			var next []string
			for _, n := range names {
				for _, v := range axes[k] {
					next = append(next, strings.ReplaceAll(n, "${{ matrix."+k+" }}", v))
				}
			}
			names = next
		}
		return names
	}
	// Single-axis, non-interpolated names get " (value)" appended.
	if len(keys) == 1 {
		var names []string
		for _, v := range axes[keys[0]] {
			names = append(names, name+" ("+v+")")
		}
		return names
	}
	return []string{name}
}

func needsList(job workflowJob) []string {
	switch job.Needs.Kind {
	case yaml.ScalarNode:
		return []string{job.Needs.Value}
	case yaml.SequenceNode:
		var out []string
		for _, n := range job.Needs.Content {
			out = append(out, n.Value)
		}
		return out
	}
	return nil
}

func hasStatusFunction(cond string) bool {
	return strings.Contains(cond, "always()") || strings.Contains(cond, "!cancelled()") ||
		strings.Contains(cond, "! cancelled()")
}

// TestEveryRequiredContextIsProducedByAJob catches a required context that no
// job can ever report, which leaves the PR at "Expected — Waiting for status to
// be reported" forever.
func TestEveryRequiredContextIsProducedByAJob(t *testing.T) {
	produced := map[string]string{}
	for file, wf := range loadWorkflows(t) {
		for id, job := range wf.Jobs {
			for _, n := range contextNames(id, job) {
				produced[n] = file
			}
		}
	}
	for _, ctx := range requiredContexts(t) {
		if _, ok := produced[ctx]; !ok {
			t.Errorf("required status check %q is not produced by any job in .github/workflows", ctx)
		}
	}
}

// TestCIGatewayIsRequired is the whole point of the gateway. Its skip audit is
// advisory unless branch protection enforces it: the scoped jobs it watches
// report `skipped`, GitHub accepts a skipped required context, and a red
// gateway that is not required blocks nothing.
func TestCIGatewayIsRequired(t *testing.T) {
	if slices.Contains(requiredContexts(t), "CI Gateway") {
		return
	}
	t.Fatal("`CI Gateway` must be a required status check in .github/rulesets/main.json, " +
		"otherwise its skip audit cannot block a merge")
}

// TestRequiredMatrixJobsAreScopedAtStepLevel is the regression test for the
// sharpest edge of this design. GitHub does not expand `strategy.matrix` for a
// job whose job-level `if:` is false: the skipped check run carries the
// literal, unexpanded name (`AGM E2E Install (${{ matrix.distro }})` is what
// this repo's own PR runs report), so the required expanded contexts are never
// emitted at all.
func TestRequiredMatrixJobsAreScopedAtStepLevel(t *testing.T) {
	required := map[string]bool{}
	for _, ctx := range requiredContexts(t) {
		required[ctx] = true
	}
	for file, wf := range loadWorkflows(t) {
		for id, job := range wf.Jobs {
			names := contextNames(id, job)
			if len(names) == 0 || len(job.Strategy.Matrix) == 0 {
				continue
			}
			isRequired := false
			for _, n := range names {
				if required[n] {
					isRequired = true
				}
			}
			if !isRequired || job.If == "" {
				continue
			}
			if strings.Contains(job.If, "needs.") && strings.Contains(job.If, ".outputs.") {
				t.Errorf("%s: job %q produces a required matrix context but gates on a "+
					"`needs.*.outputs` value at the JOB level (%q). A skipped matrix job "+
					"never emits its expanded contexts — scope the steps instead.",
					file, id, job.If)
			}
			if !hasStatusFunction(job.If) {
				t.Errorf("%s: job %q produces a required matrix context and has a job-level "+
					"`if:` (%q) with no status function, so it inherits an implicit "+
					"`success()` and a failed dependency would skip it.", file, id, job.If)
			}
		}
	}
}

// TestRequiredJobsSurviveAFailedDependency covers the non-matrix half: an `if:`
// with no status function inherits `success()` over `needs`, so a detector that
// dies on checkout silently skips the consumer, and a skipped required check
// satisfies branch protection.
func TestRequiredJobsSurviveAFailedDependency(t *testing.T) {
	required := map[string]bool{}
	for _, ctx := range requiredContexts(t) {
		required[ctx] = true
	}
	for file, wf := range loadWorkflows(t) {
		for id, job := range wf.Jobs {
			isRequired := false
			for _, n := range contextNames(id, job) {
				if required[n] {
					isRequired = true
				}
			}
			if !isRequired || len(needsList(job)) == 0 {
				continue
			}
			if !hasStatusFunction(job.If) {
				t.Errorf("%s: job %q produces a required status check and depends on %v, but "+
					"its `if:` (%q) has no `always()`/`!cancelled()`. A failed dependency "+
					"would skip it, and a skipped required check satisfies branch protection.",
					file, id, needsList(job), job.If)
			}
		}
	}
}

// TestScopedJobsTreatADetectorFailureAsRelevant covers the jobs that are not
// themselves required but are audited by the gateway.
func TestScopedJobsTreatADetectorFailureAsRelevant(t *testing.T) {
	for file, wf := range loadWorkflows(t) {
		for id, job := range wf.Jobs {
			conds := []string{job.If}
			for _, s := range job.Steps {
				conds = append(conds, s.If)
			}
			for _, cond := range conds {
				if !strings.Contains(cond, "needs."+detectorJob+".outputs.") {
					continue
				}
				if strings.Contains(cond, "needs."+detectorJob+".result") || hasStatusFunction(cond) {
					continue
				}
				t.Errorf("%s: job %q reads `needs.%s.outputs.*` in %q without a "+
					"`needs.%s.result != 'success'` clause or a status function. A failed "+
					"detector publishes empty outputs, which must mean \"run everything\".",
					file, id, detectorJob, cond, detectorJob)
			}
		}
	}
}

// TestNoPathFiltersOnRequiredWorkflows: a workflow dropped by `on.<event>.paths`
// creates no check run at all, so the required context never reports and the PR
// is unmergeable forever. This is the trap ADR-038 exists to avoid.
func TestNoPathFiltersOnRequiredWorkflows(t *testing.T) {
	required := map[string]bool{}
	for _, ctx := range requiredContexts(t) {
		required[ctx] = true
	}
	for file, wf := range loadWorkflows(t) {
		produces := false
		for id, job := range wf.Jobs {
			for _, n := range contextNames(id, job) {
				if required[n] {
					produces = true
				}
			}
		}
		if !produces {
			continue
		}
		var on map[string]yaml.Node
		if err := wf.On.Decode(&on); err != nil {
			continue // `on: [push]` shorthand carries no filters
		}
		for event, node := range on {
			// Only the pull_request triggers matter: a PR's required contexts
			// come from the runs on the PR itself. A `paths` filter on `push`
			// changes what runs after merge, not what gates the merge.
			if event != "pull_request" && event != "pull_request_target" {
				continue
			}
			var spec map[string]yaml.Node
			if node.Decode(&spec) != nil {
				continue
			}
			for _, key := range []string{"paths", "paths-ignore"} {
				if _, bad := spec[key]; bad {
					t.Errorf("%s: event %q uses `%s`, but this workflow produces a required "+
						"status check. A workflow dropped by a path filter reports no check "+
						"run, so the required context never arrives. Scope with a job- or "+
						"step-level `if:` instead (ADR-038).", file, event, key)
				}
			}
		}
	}
}

// TestGatewayWatchesEveryScopedJob keeps the gateway's `needs` in step with the
// jobs that carry a selection condition. A scoped job outside the gateway's
// `needs` can be skipped with nothing auditing the skip.
func TestGatewayWatchesEveryScopedJob(t *testing.T) {
	wf := loadWorkflows(t)["ci.yml"]
	gateway, ok := wf.Jobs["ci-gateway"]
	if !ok {
		t.Fatal("ci.yml has no ci-gateway job")
	}
	if !strings.Contains(gateway.If, "always()") {
		t.Fatalf("ci-gateway must use `if: always()`, got %q — a gateway that skips is a "+
			"gateway that passes", gateway.If)
	}
	watched := map[string]bool{}
	for _, n := range needsList(gateway) {
		watched[n] = true
	}
	for id, job := range wf.Jobs {
		if id == "ci-gateway" || id == detectorJob {
			continue
		}
		if !strings.Contains(job.If, "needs."+detectorJob+".outputs.") {
			continue
		}
		if !watched[id] {
			t.Errorf("ci.yml: job %q is path-scoped but is not in ci-gateway's `needs`, so a "+
				"wrong skip would go unaudited", id)
		}
	}
}

// TestEmbeddedAssetsAreDiscoverableFromTheTree pins the mechanism the `go`
// selector relies on. If //go:embed discovery silently returns nothing, every
// embedded `.sql`, `.yaml` and Markdown asset falls back to the extension
// denylist, and embedded Markdown would be misread as documentation.
func TestEmbeddedAssetsAreDiscoverableFromTheTree(t *testing.T) {
	roots, err := DiscoverEmbedRoots(repoRoot())
	if err != nil {
		t.Fatalf("embed discovery: %v", err)
	}
	set := map[string]bool{}
	for _, r := range roots {
		set[r] = true
	}
	for _, want := range []string{
		"pkg/source/sqlite/schema.sql",
		"agm/internal/contracts/slo-contracts.yaml",
		"agm/internal/dolt/migrations/001_initial_schema.sql",
	} {
		if !set[want] {
			t.Errorf("//go:embed discovery missed %q — the `go` selector would treat it as "+
				"an unrelated asset (found %d roots)", want, len(roots))
		}
	}

	// And the classifier must act on them.
	c := &Classifier{EmbedRoots: roots}
	if sel := c.Classify([]string{"pkg/source/sqlite/schema.sql"}); !sel.Values["go"] {
		t.Error("an embedded schema.sql must set go=true")
	}
}
