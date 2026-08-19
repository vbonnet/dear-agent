// Package tofuimport holds the decisions infra/import.sh used to make in Bash.
//
// Importing existing GitHub objects into OpenTofu state is a sequence of
// irreversible, partially-ordered state mutations. Getting one wrong does not
// fail loudly: it binds a state address to the wrong remote object, and the
// next plan then proposes changes to a ruleset nobody meant to touch. Every
// such decision therefore has to fail closed, and every fail-closed rule needs
// a test.
//
// Bash is a poor host for that. The repository caps shell scripts at 20 lines
// for this reason, and infra/import.sh was already running on a waiver at 103.
// So the decisions live here, as pure functions over recorded evidence, and
// the script keeps only what a script is good at: running commands and moving
// their output around.
package tofuimport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// identitySegment is the grammar for a GitHub repository or owner name: ASCII
// alphanumerics, ".", "-" and "_", starting with an alphanumeric.
//
// This mirrors .github/infra/identity-segment.jq, which the workflows use for
// the same purpose. The rule is not cosmetic. A segment carrying a newline
// splits one record into two repositories downstream, a "/" produces a bogus
// owner/name slug, and a quote produces an invalid OpenTofu state address or
// provider import ID. Because the import loop mutates state per repository,
// any of those would surface only after a partial import.
var identitySegment = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// canonicalRepository is the repository whose checked-in ruleset is the source
// of authority. An inventory that omits it would reconcile the fleet against a
// policy that never mentions its own source of truth.
const canonicalRepository = "dear-agent"

// Inventory is the set of repositories the import acts on, as evaluated from
// the OpenTofu input variables rather than from a second hard-coded fleet list.
type Inventory struct {
	Active   []string `json:"active"`
	Archived []string `json:"archived"`
}

// ParseInventory validates the JSON emitted by
//
//	tofu console <<< 'jsonencode({active = sort(keys(var.active_repos)), ...})'
//
// It rejects anything it cannot act on safely rather than importing a subset.
func ParseInventory(raw []byte) (Inventory, error) {
	var inventory Inventory
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&inventory); err != nil {
		return Inventory{}, fmt.Errorf("evaluated OpenTofu inventory is not the expected {active, archived} object: %w", err)
	}

	for _, group := range []struct {
		label string
		names []string
	}{{"active", inventory.Active}, {"archived", inventory.Archived}} {
		for _, name := range group.names {
			if !identitySegment.MatchString(name) {
				return Inventory{}, fmt.Errorf("%s repository %q is not a valid GitHub identity segment", group.label, name)
			}
		}
	}

	// GitHub repository names are case-insensitive, so two entries differing
	// only in case are one repository declared twice, and a repository in both
	// lists would be managed and ignored at the same time.
	seen := map[string]string{}
	for _, group := range []struct {
		label string
		names []string
	}{{"active", inventory.Active}, {"archived", inventory.Archived}} {
		for _, name := range group.names {
			key := strings.ToLower(name)
			if previous, duplicate := seen[key]; duplicate {
				return Inventory{}, fmt.Errorf(
					"repository %q duplicates or overlaps %q under case-insensitive comparison", name, previous)
			}
			seen[key] = name
		}
	}

	if _, present := seen[canonicalRepository]; !present {
		return Inventory{}, fmt.Errorf("inventory omits %s, the repository whose checked-in ruleset is canonical", canonicalRepository)
	}

	// Sorted so the import order, and therefore any partial-failure point, is
	// reproducible between runs.
	sort.Strings(inventory.Active)
	sort.Strings(inventory.Archived)
	return inventory, nil
}
