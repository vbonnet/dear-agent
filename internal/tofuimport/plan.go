package tofuimport

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Verb is what the script should do with a step.
type Verb string

const (
	// VerbImport means run `tofu import <address> <import_id>`.
	VerbImport Verb = "import"
	// VerbSkip means the address is already in state and, for rulesets, bound
	// to the object the plan expects.
	VerbSkip Verb = "skip"
	// VerbCreate means the remote object provably does not exist, so `tofu
	// plan` will propose creating it. No state is mutated.
	VerbCreate Verb = "create"
)

// Step is one line of the import plan.
type Step struct {
	Verb     Verb   `json:"verb"`
	Address  string `json:"address"`
	ImportID string `json:"import_id,omitempty"`
	Reason   string `json:"reason"`
}

// Evidence is everything the planner reads. It is gathered by the script and
// passed in, so planning itself touches no network and no state.
type Evidence struct {
	Inventory Inventory
	State     State
	// CanonicalRulesetName comes from .github/rulesets/main.json.
	CanonicalRulesetName string
	// RulesetPages maps an active repository to the raw
	// `gh api --paginate --slurp .../rulesets` body. A repository missing from
	// this map is an error: an unlisted repository is not evidence of absence.
	RulesetPages map[string][]byte
}

// BuildPlan resolves every repository's identity before proposing a single
// mutation.
//
// Resolving the whole fleet up front is the point. A listing failure or an
// ambiguous match discovered halfway through would land after earlier
// repositories were already imported, leaving a partial state that the next
// run has to reason about. Here, any such failure happens before step one.
func BuildPlan(evidence Evidence) ([]Step, error) {
	if evidence.CanonicalRulesetName == "" {
		return nil, fmt.Errorf("canonical ruleset name is required")
	}

	// Phase one: resolve identities. Nothing is emitted yet.
	rulesetIDs := map[string]int{}
	rulesetAbsent := map[string]bool{}
	for _, repo := range evidence.Inventory.Active {
		raw, listed := evidence.RulesetPages[repo]
		if !listed {
			return nil, fmt.Errorf("no ruleset listing was collected for %s; refusing to infer absence", repo)
		}
		summaries, err := ParseRulesetPages(raw)
		if err != nil {
			return nil, fmt.Errorf("ruleset listing for %s: %w", repo, err)
		}
		id, present, err := SelectRulesetID(repo, evidence.CanonicalRulesetName, summaries)
		if err != nil {
			return nil, err
		}
		if present {
			rulesetIDs[repo] = id
		} else {
			rulesetAbsent[repo] = true
		}
	}

	// Phase two: emit steps. Every decision above already succeeded.
	var steps []Step
	for _, repo := range evidence.Inventory.Active {
		steps = append(steps,
			simpleStep(evidence.State, ModuleAddress(repo, "github_repository.this"), repo),
			simpleStep(evidence.State, ModuleAddress(repo, "github_repository_dependabot_security_updates.this"), repo),
		)

		address := ModuleAddress(repo, "github_repository_ruleset.branch_protection")
		if rulesetAbsent[repo] {
			steps = append(steps, Step{
				Verb:    VerbCreate,
				Address: address,
				Reason:  "repository has no ruleset yet; plan will propose creating it",
			})
			continue
		}

		expected := fmt.Sprintf("%s:%d", repo, rulesetIDs[repo])
		if !evidence.State.Has(address) {
			steps = append(steps, Step{
				Verb:     VerbImport,
				Address:  address,
				ImportID: expected,
				Reason:   "ruleset resolved to a single provider object",
			})
			continue
		}
		actual, err := evidence.State.RulesetBinding(address)
		if err != nil {
			return nil, err
		}
		if actual != expected {
			return nil, fmt.Errorf(
				"stale ruleset state binding for %s: found %s, expected %s", address, actual, expected)
		}
		steps = append(steps, Step{
			Verb:    VerbSkip,
			Address: address,
			Reason:  "already imported and verified against " + actual,
		})
	}

	for _, repo := range evidence.Inventory.Archived {
		steps = append(steps, simpleStep(evidence.State, fmt.Sprintf("github_repository.archived[%q]", repo), repo))
	}
	return steps, nil
}

// simpleStep covers the resources whose import ID is just the repository name
// and whose only state question is presence.
func simpleStep(state State, address, importID string) Step {
	if state.Has(address) {
		return Step{Verb: VerbSkip, Address: address, Reason: "already imported"}
	}
	return Step{Verb: VerbImport, Address: address, ImportID: importID, Reason: "not yet in state"}
}

// ModuleAddress builds a managed-repo module address. The active fleet is
// managed by ./modules/managed-repo, instantiated per repository via for_each,
// so imports must land on the module-qualified address the configuration
// expects rather than on a bare root-level one.
func ModuleAddress(repo, resource string) string {
	return fmt.Sprintf("module.managed_repos[%q].%s", repo, resource)
}

// EncodePlan renders a plan as one tab-separated record per line, which is what
// infra/import.sh reads. Tabs, not spaces: an address contains quotes and
// brackets but never a tab, so the script can split on it without quoting
// rules of its own.
func EncodePlan(steps []Step) string {
	var out strings.Builder
	for _, step := range steps {
		fmt.Fprintf(&out, "%s\t%s\t%s\t%s\n", step.Verb, step.Address, step.ImportID, step.Reason)
	}
	return out.String()
}

// EncodePlanJSON renders a plan as JSON, for a caller that wants to assert on
// it rather than execute it.
func EncodePlanJSON(steps []Step) ([]byte, error) {
	if steps == nil {
		steps = []Step{}
	}
	return json.MarshalIndent(steps, "", "  ")
}
