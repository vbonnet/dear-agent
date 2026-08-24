package workflow

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Constitutional declares "humans set the rules; agents implement them"
// mode for a workflow. When enabled, workflow validation fails unless
// the workflow ships a non-empty Invariants block. The block is
// declarative: each invariant names a target output and either a JSON
// Schema the output must satisfy, a regex the output must match, or a
// numeric floor the output's confidence must clear.
//
// The point is to separate two concerns a workflow lifecycle can otherwise
// conflate: a human author writes down the contract once (the invariants), and
// downstream Audit / Enforce phases — eventually
// adversarial review per ROADMAP §6.5 — verify the run-time artifact
// against that contract. This file ships the schema + the definition
// validation; verification lives in pkg/audit.
type Constitutional struct {
	// Enforce flips the workflow into constitutional mode. When true,
	// Validate requires Invariants to be non-empty and well-shaped.
	// When false (the default), the block is informational only — a
	// workflow may ship invariants without enforcement, useful for
	// migration.
	Enforce bool `yaml:"enforce,omitempty"`

	// Description is an optional free-text field documenting what
	// invariants this workflow's authors committed to. Not interpreted
	// by the runner.
	Description string `yaml:"description,omitempty"`
}

// InvariantKind is the dispatcher that decides which verifier the
// Audit phase will run against an invariant. The four kinds line up
// 1:1 with the kinds an ExitGate already understands; the contrast is
// that ExitGate evaluates per-node and gates the node, while an
// Invariant is workflow-level and feeds the adversarial-review
// pipeline.
type InvariantKind string

const (
	// InvariantJSONSchema validates Target against a JSON Schema file
	// (Schema). The schema path is resolved relative to the workflow
	// file's directory at run time.
	InvariantJSONSchema InvariantKind = "json_schema"

	// InvariantRegexMatch checks that Target matches Pattern. Case-
	// sensitive unless the pattern carries an (?i) flag.
	InvariantRegexMatch InvariantKind = "regex_match"

	// InvariantConfidenceFloor reads a numeric value from Target and
	// requires it to be >= Min. Used to pin the minimum reviewer
	// confidence the run must achieve before being trusted.
	InvariantConfidenceFloor InvariantKind = "confidence_floor"

	// InvariantPredicate is the free-form text kind: the invariant is
	// documented for humans but has no automatic verifier. Predicate
	// holds the natural-language statement. These show up in audit
	// reports so a reviewer (human or LLM) sees what the workflow
	// claimed.
	InvariantPredicate InvariantKind = "predicate"
)

// Invariant is one declarative claim about what the workflow must
// produce. Exactly one of Schema, Pattern, Min, or Predicate must be
// populated, matching Kind.
type Invariant struct {
	// ID is a short stable identifier (kebab-case, alphanumeric +
	// dashes). Used by audit findings to point back at the invariant
	// they verified; must be unique within a workflow.
	ID string `yaml:"id"`

	// Description is the one-line human-readable statement of what
	// the invariant guarantees. Required: the whole point of
	// constitutional mode is that a human committed to this in
	// writing, so an empty description is rejected.
	Description string `yaml:"description"`

	// Kind selects the verifier. See InvariantKind for the menu.
	Kind InvariantKind `yaml:"kind"`

	// Target is a dotted path expression rooted at outputs (e.g.
	// "outputs.report.path", "outputs.report.frontmatter.confidence").
	// Required for all kinds except Predicate.
	Target string `yaml:"target,omitempty"`

	// Schema is the path to a JSON Schema file for Kind=json_schema.
	// Resolved relative to the workflow file's directory at run time.
	Schema string `yaml:"schema,omitempty"`

	// Pattern is the regex for Kind=regex_match.
	Pattern string `yaml:"pattern,omitempty"`

	// Min is the inclusive lower bound for Kind=confidence_floor.
	Min float64 `yaml:"min,omitempty"`

	// Predicate is the natural-language claim for Kind=predicate.
	Predicate string `yaml:"predicate,omitempty"`

	// VerifierRole hints at which model family should adversarially
	// verify this invariant. Empty means "use the workflow's default
	// reviewer-cross binding". Plays into ROADMAP §6.5 (trust
	// inversion): an invariant verified by a different family from
	// the implementer earns higher trust.
	VerifierRole string `yaml:"verifier_role,omitempty"`
}

var invariantIDRegex = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Validate enforces shape invariants on a single Invariant entry.
// The runner calls this from Workflow.Validate so YAML authors see a
// clear error at load time rather than a runtime "unknown field".
func (inv *Invariant) Validate() error {
	if inv.ID == "" {
		return fmt.Errorf("invariant: id is required")
	}
	if !invariantIDRegex.MatchString(inv.ID) {
		return fmt.Errorf("invariant %q: id must match %s", inv.ID, invariantIDRegex.String())
	}
	if strings.TrimSpace(inv.Description) == "" {
		return fmt.Errorf("invariant %q: description is required", inv.ID)
	}
	return inv.validateKind()
}

// validateKind enforces the per-kind shape requirements for an Invariant.
// It is split out of Validate to keep each method's cyclomatic complexity
// within the gocyclo budget.
func (inv *Invariant) validateKind() error {
	switch inv.Kind {
	case InvariantJSONSchema:
		if inv.Target == "" || inv.Schema == "" {
			return fmt.Errorf("invariant %q: kind=json_schema requires target and schema", inv.ID)
		}
	case InvariantRegexMatch:
		if inv.Target == "" || inv.Pattern == "" {
			return fmt.Errorf("invariant %q: kind=regex_match requires target and pattern", inv.ID)
		}
		if _, err := regexp.Compile(inv.Pattern); err != nil {
			return fmt.Errorf("invariant %q: invalid pattern: %w", inv.ID, err)
		}
	case InvariantConfidenceFloor:
		if inv.Target == "" {
			return fmt.Errorf("invariant %q: kind=confidence_floor requires target", inv.ID)
		}
		if inv.Min <= 0 || inv.Min > 1 {
			return fmt.Errorf("invariant %q: kind=confidence_floor min must be in (0,1], got %v", inv.ID, inv.Min)
		}
	case InvariantPredicate:
		if strings.TrimSpace(inv.Predicate) == "" {
			return fmt.Errorf("invariant %q: kind=predicate requires predicate", inv.ID)
		}
	default:
		return fmt.Errorf("invariant %q: unknown kind %q", inv.ID, inv.Kind)
	}
	return nil
}

// validateConstitutional is the single validation seam for the workflow-level
// constitutional contract. Presence and entry shape belong together here so
// every caller of Workflow.Validate inherits the same fail-closed behavior.
func validateConstitutional(c *Constitutional, invs []Invariant) error {
	if err := validateConstitutionalPresence(c, invs); err != nil {
		return err
	}

	seen := make(map[string]struct{}, len(invs))
	for i := range invs {
		inv := &invs[i]
		if err := inv.Validate(); err != nil {
			return fmt.Errorf("invariants[%d]: %w", i, err)
		}
		if _, dup := seen[inv.ID]; dup {
			return fmt.Errorf("invariants[%d]: duplicate id %q", i, inv.ID)
		}
		seen[inv.ID] = struct{}{}
	}
	return nil
}

func validateConstitutionalPresence(c *Constitutional, invs []Invariant) error {
	if c != nil && c.Enforce && len(invs) == 0 {
		return errors.New("constitutional mode is on but declares no invariants")
	}
	return nil
}
