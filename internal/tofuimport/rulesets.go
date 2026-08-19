package tofuimport

import (
	"encoding/json"
	"fmt"
)

// CanonicalRulesetID is the immutable provider ID of the live dear-agent
// repository ruleset.
//
// dear-agent is anchored by ID as well as by name because a name-only lookup
// cannot tell "the ruleset was renamed" from "somebody created a second one".
// Guessing wrong there creates a parallel active ruleset beside the real one.
const CanonicalRulesetID = 18061003

// LegacyRulesetName is the name the fleet's rulesets carry, and the name
// dear-agent's own ruleset carried before it was renamed. Accepting both names
// for dear-agent lets the importer run before or after the rename.
const LegacyRulesetName = "branch-protection"

// RulesetSummary is one entry of GET /repos/{owner}/{repo}/rulesets.
type RulesetSummary struct {
	ID   *int    `json:"id"`
	Name *string `json:"name"`
}

// ParseRulesetPages flattens the array-of-pages `gh api --paginate --slurp`
// produces and proves every entry carries a usable identity.
//
// Absence has to be proven, not inferred. A response such as [null] or [{}] is
// a JSON array that matches no name; reading it as "no ruleset exists" would
// let the next plan create a second active ruleset beside the real one. A
// matching object with a null id would import the literal string "null" as a
// provider ID.
func ParseRulesetPages(raw []byte) ([]RulesetSummary, error) {
	var pages [][]RulesetSummary
	if err := json.Unmarshal(raw, &pages); err != nil {
		return nil, fmt.Errorf("ruleset listing is not a paginated array of arrays: %w", err)
	}

	var summaries []RulesetSummary
	for _, page := range pages {
		for _, summary := range page {
			if summary.ID == nil || summary.Name == nil {
				return nil, fmt.Errorf("ruleset listing contains an entry without an id or a name")
			}
			if *summary.ID <= 0 {
				return nil, fmt.Errorf("ruleset listing contains a non-positive ruleset id %d", *summary.ID)
			}
			if *summary.Name == "" {
				return nil, fmt.Errorf("ruleset listing contains a ruleset with an empty name")
			}
			summaries = append(summaries, summary)
		}
	}
	return summaries, nil
}

// SelectRulesetID picks the one ruleset it is safe to import for a repository.
//
// It returns (id, true, nil) when exactly one ruleset was identified,
// (0, false, nil) when the repository provably has none yet, and an error
// whenever the evidence is ambiguous. An ambiguous lookup is never evidence
// that creating another ruleset is safe.
func SelectRulesetID(repo, canonicalName string, summaries []RulesetSummary) (int, bool, error) {
	if canonicalName == "" {
		return 0, false, fmt.Errorf("canonical ruleset name is empty")
	}
	if repo == canonicalRepository {
		return selectCanonicalRulesetID(canonicalName, summaries)
	}

	var matches []RulesetSummary
	for _, summary := range summaries {
		if *summary.Name == LegacyRulesetName {
			matches = append(matches, summary)
		}
	}
	switch len(matches) {
	case 0:
		return 0, false, nil
	case 1:
		return *matches[0].ID, true, nil
	default:
		return 0, false, fmt.Errorf(
			"found %d rulesets named %s on %s; refusing an ambiguous import", len(matches), LegacyRulesetName, repo)
	}
}

// selectCanonicalRulesetID resolves dear-agent's ruleset, which is pinned to
// its immutable provider ID. A duplicate, a replacement ID, or an unexpected
// rename all fail closed, so a recovery run can never turn an uncertain lookup
// into a second active ruleset.
func selectCanonicalRulesetID(canonicalName string, summaries []RulesetSummary) (int, bool, error) {
	var matches []RulesetSummary
	for _, summary := range summaries {
		if *summary.ID == CanonicalRulesetID || *summary.Name == canonicalName || *summary.Name == LegacyRulesetName {
			matches = append(matches, summary)
		}
	}
	if len(matches) != 1 {
		return 0, false, fmt.Errorf(
			"expected exactly one %s ruleset matching ID %d or name %s/%s; found %d",
			canonicalRepository, CanonicalRulesetID, canonicalName, LegacyRulesetName, len(matches))
	}

	selected := matches[0]
	if *selected.ID != CanonicalRulesetID {
		return 0, false, fmt.Errorf(
			"%s ruleset %s has replacement ID %d; expected canonical ID %d",
			canonicalRepository, *selected.Name, *selected.ID, CanonicalRulesetID)
	}
	if *selected.Name != canonicalName && *selected.Name != LegacyRulesetName {
		return 0, false, fmt.Errorf(
			"%s ruleset ID %d has unexpected name %s", canonicalRepository, CanonicalRulesetID, *selected.Name)
	}
	return *selected.ID, true, nil
}

// CanonicalRulesetName reads the ruleset name out of .github/rulesets/main.json,
// the checked-in document that owns it.
func CanonicalRulesetName(raw []byte) (string, error) {
	var document struct {
		Name *string `json:"name"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return "", fmt.Errorf("canonical ruleset document is not valid JSON: %w", err)
	}
	if document.Name == nil || *document.Name == "" {
		return "", fmt.Errorf("canonical ruleset document has no name")
	}
	return *document.Name, nil
}
