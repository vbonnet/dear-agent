package tofuimport

import (
	"encoding/json"
	"fmt"
)

// State is the subset of `tofu show -json` this package reads.
type State struct {
	Values *struct {
		RootModule module `json:"root_module"`
	} `json:"values"`
}

type module struct {
	Resources    []resource `json:"resources"`
	ChildModules []module   `json:"child_modules"`
}

type resource struct {
	Address string `json:"address"`
	Values  struct {
		Repository *string `json:"repository"`
		ID         *string `json:"id"`
	} `json:"values"`
}

// ParseState reads `tofu show -json`. Empty input is an empty state, which is
// the normal first-run case, not an error.
func ParseState(raw []byte) (State, error) {
	if len(raw) == 0 {
		return State{}, nil
	}
	var state State
	if err := json.Unmarshal(raw, &state); err != nil {
		return State{}, fmt.Errorf("OpenTofu state JSON is unreadable: %w", err)
	}
	return state, nil
}

// resourcesAt returns every state resource at an address. More than one is an
// error at the call site, never a "pick the first" situation.
func (s State) resourcesAt(address string) []resource {
	if s.Values == nil {
		return nil
	}
	var found []resource
	var walk func(m module)
	walk = func(m module) {
		for _, r := range m.Resources {
			if r.Address == address {
				found = append(found, r)
			}
		}
		for _, child := range m.ChildModules {
			walk(child)
		}
	}
	walk(s.Values.RootModule)
	return found
}

// Has reports whether the state already tracks an address.
func (s State) Has(address string) bool {
	return len(s.resourcesAt(address)) > 0
}

// RulesetBinding returns the "<repository>:<id>" a state address is bound to.
//
// A stale address is not an "already imported" success. Skipping it would let
// a later plan act on the wrong remote ruleset, which is exactly the failure
// this whole importer exists to avoid, so anything other than one resolvable
// resource is an error.
func (s State) RulesetBinding(address string) (string, error) {
	found := s.resourcesAt(address)
	if len(found) != 1 {
		return "", fmt.Errorf("expected exactly one state resource at %s, found %d", address, len(found))
	}
	values := found[0].Values
	if values.Repository == nil || values.ID == nil {
		return "", fmt.Errorf("state resource at %s has no repository or id to verify", address)
	}
	return fmt.Sprintf("%s:%s", *values.Repository, *values.ID), nil
}
