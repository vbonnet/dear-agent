package tofuimport

import (
	"strings"
	"testing"
)

// rulesetListing renders one paginated ruleset page.
func rulesetListing(entries ...string) []byte {
	return []byte("[[" + strings.Join(entries, ",") + "]]")
}

func canonicalListing() []byte {
	return rulesetListing(`{"id":18061003,"name":"main-zero-bypass"}`)
}

// stateWithRuleset builds a `tofu show -json` document binding the dear-agent
// ruleset address to a repository and provider ID.
func stateWithRuleset(t *testing.T, repository, id string) State {
	t.Helper()
	raw := `{"values":{"root_module":{"child_modules":[{"resources":[{"address":` +
		`"module.managed_repos[\"dear-agent\"].github_repository_ruleset.branch_protection",` +
		`"values":{"repository":"` + repository + `","id":"` + id + `"}}]}]}}}`
	state, err := ParseState([]byte(raw))
	if err != nil {
		t.Fatalf("ParseState: %v", err)
	}
	return state
}

func baseEvidence() Evidence {
	return Evidence{
		Inventory:            Inventory{Active: []string{"dear-agent"}},
		CanonicalRulesetName: canonicalName,
		RulesetPages:         map[string][]byte{"dear-agent": canonicalListing()},
	}
}

func TestBuildPlanImportsEveryManagedResource(t *testing.T) {
	evidence := baseEvidence()
	evidence.Inventory.Archived = []string{"old-thing"}

	steps, err := BuildPlan(evidence)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if len(steps) != 4 {
		t.Fatalf("expected 4 steps, got %d: %+v", len(steps), steps)
	}

	want := []Step{
		{Verb: VerbImport, Address: `module.managed_repos["dear-agent"].github_repository.this`, ImportID: "dear-agent"},
		{Verb: VerbImport, Address: `module.managed_repos["dear-agent"].github_repository_dependabot_security_updates.this`, ImportID: "dear-agent"},
		{Verb: VerbImport, Address: `module.managed_repos["dear-agent"].github_repository_ruleset.branch_protection`, ImportID: "dear-agent:18061003"},
		{Verb: VerbImport, Address: `github_repository.archived["old-thing"]`, ImportID: "old-thing"},
	}
	for i, w := range want {
		if steps[i].Verb != w.Verb || steps[i].Address != w.Address || steps[i].ImportID != w.ImportID {
			t.Errorf("step %d = %+v, want verb %s address %s id %s", i, steps[i], w.Verb, w.Address, w.ImportID)
		}
	}
}

func TestBuildPlanSkipsAVerifiedExistingBinding(t *testing.T) {
	evidence := baseEvidence()
	evidence.State = stateWithRuleset(t, "dear-agent", "18061003")

	steps, err := BuildPlan(evidence)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	rulesetStep := steps[2]
	if rulesetStep.Verb != VerbSkip {
		t.Fatalf("expected the verified ruleset to be skipped, got %+v", rulesetStep)
	}
	if !strings.Contains(rulesetStep.Reason, "dear-agent:18061003") {
		t.Fatalf("skip reason does not name the verified binding: %q", rulesetStep.Reason)
	}
}

func TestBuildPlanRefusesAStaleRulesetBinding(t *testing.T) {
	// A stale address is not an "already imported" success: skipping it would
	// let a later plan act on the wrong remote ruleset.
	tests := []struct {
		name       string
		repository string
		id         string
		wantErr    string
	}{
		{name: "wrong provider id", repository: "dear-agent", id: "99", wantErr: "found dear-agent:99"},
		{name: "wrong repository", repository: "other", id: "18061003", wantErr: "found other:18061003"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evidence := baseEvidence()
			evidence.State = stateWithRuleset(t, tt.repository, tt.id)

			_, err := BuildPlan(evidence)
			if err == nil {
				t.Fatal("BuildPlan unexpectedly accepted a stale binding")
			}
			if !strings.Contains(err.Error(), tt.wantErr) ||
				!strings.Contains(err.Error(), "expected dear-agent:18061003") {
				t.Fatalf("error %q does not report both bindings", err)
			}
		})
	}
}

func TestBuildPlanProposesCreationOnlyForAProvableAbsence(t *testing.T) {
	evidence := baseEvidence()
	evidence.Inventory.Active = []string{"dear-agent", "engram-research"}
	evidence.RulesetPages["engram-research"] = rulesetListing()

	steps, err := BuildPlan(evidence)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	var created []string
	for _, step := range steps {
		if step.Verb == VerbCreate {
			created = append(created, step.Address)
			if step.ImportID != "" {
				t.Errorf("a create step must carry no import id: %+v", step)
			}
		}
	}
	if len(created) != 1 || !strings.Contains(created[0], "engram-research") {
		t.Fatalf("expected only engram-research's ruleset to be created, got %v", created)
	}
}

func TestBuildPlanResolvesEveryIdentityBeforeEmittingAnyStep(t *testing.T) {
	// The failure below concerns the second repository. If planning were
	// incremental, the first repository's steps would already have been
	// emitted and executed by the time it surfaced, leaving a partial state.
	evidence := baseEvidence()
	evidence.Inventory.Active = []string{"dear-agent", "engram-research"}
	evidence.RulesetPages["engram-research"] = rulesetListing(
		`{"id":1,"name":"branch-protection"}`, `{"id":2,"name":"branch-protection"}`)

	steps, err := BuildPlan(evidence)
	if err == nil {
		t.Fatal("BuildPlan unexpectedly succeeded")
	}
	if steps != nil {
		t.Fatalf("a failed plan must emit no steps, got %+v", steps)
	}
	if !strings.Contains(err.Error(), "refusing an ambiguous import") {
		t.Fatalf("error %q does not explain the ambiguity", err)
	}
}

func TestBuildPlanRefusesToInferAbsenceFromAMissingListing(t *testing.T) {
	evidence := baseEvidence()
	evidence.Inventory.Active = []string{"dear-agent", "engram-research"}

	_, err := BuildPlan(evidence)
	if err == nil || !strings.Contains(err.Error(), "refusing to infer absence") {
		t.Fatalf("expected a refusal to infer absence, got %v", err)
	}
}

func TestBuildPlanRequiresACanonicalRulesetName(t *testing.T) {
	evidence := baseEvidence()
	evidence.CanonicalRulesetName = ""
	if _, err := BuildPlan(evidence); err == nil {
		t.Fatal("BuildPlan unexpectedly accepted an empty canonical ruleset name")
	}
}

func TestEncodePlanIsTabSeparatedAndUnambiguous(t *testing.T) {
	steps := []Step{{Verb: VerbImport, Address: `module.managed_repos["a"].x`, ImportID: "a", Reason: "not yet in state"}}
	encoded := EncodePlan(steps)
	fields := strings.Split(strings.TrimSuffix(encoded, "\n"), "\t")
	if len(fields) != 4 {
		t.Fatalf("expected 4 tab-separated fields, got %d: %q", len(fields), encoded)
	}
	// An address carries quotes and brackets but never a tab, which is why the
	// script can split on tabs without quoting rules of its own.
	if strings.Contains(fields[1], "\t") || fields[1] != `module.managed_repos["a"].x` {
		t.Fatalf("address field is not intact: %q", fields[1])
	}
}

func TestParseStateTreatsEmptyInputAsAnEmptyState(t *testing.T) {
	state, err := ParseState(nil)
	if err != nil {
		t.Fatalf("ParseState(nil): %v", err)
	}
	if state.Has("anything") {
		t.Fatal("an empty state must contain no addresses")
	}
	if _, err := ParseState([]byte("not json")); err == nil {
		t.Fatal("ParseState unexpectedly accepted unreadable state")
	}
}

func TestRulesetBindingRequiresExactlyOneResolvableResource(t *testing.T) {
	state, err := ParseState([]byte(`{"values":{"root_module":{"resources":[
		{"address":"a","values":{"repository":"r"}}]}}}`))
	if err != nil {
		t.Fatalf("ParseState: %v", err)
	}
	if _, err := state.RulesetBinding("a"); err == nil {
		t.Fatal("a resource without an id must not resolve to a binding")
	}
	if _, err := state.RulesetBinding("missing"); err == nil {
		t.Fatal("an absent address must not resolve to a binding")
	}
}

func TestIsBenignImportFailure(t *testing.T) {
	benign := []string{
		"Error: Not Found",
		"GET https://api.github.com/repos/x: 404",
		"no security configuration associated with this repository",
	}
	for _, message := range benign {
		if !IsBenignImportFailure(message) {
			t.Errorf("%q should be recognized as an absent object", message)
		}
	}

	// Anything unrecognized must stop the run rather than be treated as
	// absence, which would leave a partially imported state.
	fatal := []string{
		"Error: 403 Forbidden",
		"Error: rate limit exceeded",
		"Error: invalid credentials",
		"",
	}
	for _, message := range fatal {
		if IsBenignImportFailure(message) {
			t.Errorf("%q must not be treated as an absent object", message)
		}
	}
}
