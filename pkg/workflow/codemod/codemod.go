// Package codemod transforms workflow YAML files between schema
// generations. The Phase 4 use case is migrating v0.1 workflows
// (`model:` hardcoded, no budget, no outputs[]) to v0.2 (`role:`
// resolved through the registry, default budget, declared outputs).
//
// The transformer operates on yaml.v3's Node tree rather than
// (un)marshalling through structs. That preserves comments, key order,
// and original formatting — crucial for a codemod whose diff a human
// will review. Round-tripping through Workflow would discard all that
// context and rewrite the file in canonical-Go-struct order.
//
// Two modes ship today:
//
//   - UpgradeV01ToV02: in-place upgrade of an existing workflow YAML.
//     Adds `schema_version: "1"` if missing; promotes `model:` →
//     `role:` for AI nodes that match a known mapping; inserts a
//     default `budget:` block when none is present.
//
//   - FromWayfinder: synthesises a new workflow YAML from a Wayfinder
//     session file. Each waypoint becomes a node, with bash bodies that
//     print the waypoint id (placeholder until Wayfinder ships its own
//     phase-as-workflow integration in Phase 4.2).
//
// Both modes return a Result describing what changed; the CLI uses that
// to print a per-file summary so dry-runs are diff-friendly.
package codemod

import (
	"bytes"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// Result records what a codemod did to a single file. Empty Changes
// means the input was already in the target form (no-op). Errors are
// returned separately — Result is only populated on success.
type Result struct {
	// Path is the source file path (informational; the codemod never
	// writes to disk itself — the caller decides where the bytes go).
	Path string

	// Changes is the human-readable list of transformations applied.
	// Each entry is a single line, suitable for stdout printing.
	Changes []string

	// Output is the transformed YAML bytes. Identical to the input when
	// no changes were applied (a no-op codemod still returns the input
	// unmodified, so callers can write unconditionally).
	Output []byte
}

// Changed reports whether any transformation was applied. False on a
// no-op so the CLI can skip writing.
func (r Result) Changed() bool { return len(r.Changes) > 0 }

// UpgradeOptions tunes the v0.1 → v0.2 upgrade. The zero value is the
// safe default: only mappings that have a known role are promoted; no
// stub fields (budget, outputs) are inserted.
type UpgradeOptions struct {
	// AddDefaultBudget, when true, adds a `budget:` block to every AI
	// node that doesn't already declare one. The default budget caps
	// max_tokens=50000 and max_dollars=1.00 — conservative seed values
	// the operator is expected to tune. Recommended for new code that
	// lacks any budget framing today.
	AddDefaultBudget bool

	// DropModelOnRolePromotion removes `model:` from AI nodes once
	// `role:` is added. When false (the default), both fields remain
	// and the runner prefers role; when true, the node only carries
	// role: and the codemod is a true upgrade rather than additive.
	// Defaults to false because a paired role+model pair is safer
	// during a multi-PR migration: callers that bypass the registry
	// still see a model id, and the lint will flag the model later.
	DropModelOnRolePromotion bool

	// ModelToRole overrides the built-in mapping. Keys are model ids
	// (e.g. "claude-opus-4-7"); values are role names. A non-nil map
	// replaces the built-in entirely; nil falls back to BuiltinModelToRole.
	ModelToRole map[string]string
}

// UpgradeV01ToV02 reads a workflow YAML and returns a transformed
// version that uses the Phase 1+ schema. The transformation is a
// best-effort upgrade — fields that have no defensible default
// (permissions, exit_gate, hitl) are left absent rather than guessed.
//
// The function never returns a partial Result on error: if any step
// fails, the caller gets the original bytes back via the error.
func UpgradeV01ToV02(in []byte, opts UpgradeOptions) (Result, error) {
	mapping := opts.ModelToRole
	if mapping == nil {
		mapping = BuiltinModelToRole()
	}
	root, err := parseDoc(in)
	if err != nil {
		return Result{}, err
	}
	doc := docNode(root)
	if doc == nil || doc.Kind != yaml.MappingNode {
		return Result{}, fmt.Errorf("codemod: top-level YAML must be a mapping, got %v", kindName(doc))
	}
	var changes []string

	// 1. Ensure schema_version is set. Older workflows omit it; the
	// runner accepts the absence today, but downstream tooling is
	// easier when every file carries an explicit version.
	if added := ensureScalarField(doc, "schema_version", "1", "!!str"); added {
		changes = append(changes, "added schema_version: \"1\"")
	}

	// 2. Walk every AI node — top-level and inside loop bodies — and
	// apply the role/budget upgrades.
	walkNodes(doc, func(parent string, nodeMap *yaml.Node) {
		nodeID := scalarField(nodeMap, "id")
		ai := mappingField(nodeMap, "ai")
		if ai == nil {
			return
		}
		ref := nodeRef(parent, nodeID)
		if changed := promoteModelToRole(ai, mapping, opts.DropModelOnRolePromotion); changed != "" {
			changes = append(changes, fmt.Sprintf("%s: %s", ref, changed))
		}
		if opts.AddDefaultBudget {
			if added := ensureBudget(nodeMap); added {
				changes = append(changes, fmt.Sprintf("%s: added default budget block", ref))
			}
		}
	})

	if len(changes) == 0 {
		// No transformations — return the original bytes unchanged.
		// Re-serialising would re-format quoting/whitespace, which is
		// noise in the diff a human reviews.
		return Result{Output: in}, nil
	}

	out, err := serialiseDoc(root)
	if err != nil {
		return Result{}, err
	}
	return Result{Changes: changes, Output: out}, nil
}

// BuiltinModelToRole is the default model → role mapping used by
// UpgradeV01ToV02 when the caller doesn't supply one. Keys are the
// canonical model ids; values are roles defined in the built-in
// registry (`research`, `implementer`, `reviewer`).
//
// The mapping is deliberately small: only models the registry knows
// about are promoted. An unmapped model is left alone (the lint then
// flags it via --check-deprecated-models).
func BuiltinModelToRole() map[string]string {
	return map[string]string{
		// Long-context analysis → research tier.
		"claude-opus-4-7":  "research",
		"claude-opus-4-5":  "research",
		"claude-opus-3":    "research",
		"gemini-3.1-pro":   "research",
		"gemini-2.5-pro":   "research",
		"gpt-5.5-pro":      "research",

		// Code synthesis → implementer tier.
		"claude-sonnet-4-6": "implementer",
		"claude-sonnet-4-5": "implementer",
		"claude-sonnet-3-5": "implementer",
		"claude-haiku-4-5":  "implementer",
		"gpt-4-turbo":       "implementer",
		"gpt-4o":            "implementer",
	}
}

// promoteModelToRole inspects an AI body and adds `role:` based on the
// model id when missing. Returns a one-line description of what
// changed, or "" if no change was made.
//
// drop=true removes the model field entirely; drop=false preserves it
// alongside the new role for back-compat (the runner prefers role and
// the lint will eventually nudge the user to drop the model).
func promoteModelToRole(ai *yaml.Node, mapping map[string]string, drop bool) string {
	model := scalarField(ai, "model")
	role := scalarField(ai, "role")
	if role != "" {
		// Already declares a role; nothing to do.
		return ""
	}
	if model == "" {
		return ""
	}
	mappedRole, ok := mapping[model]
	if !ok {
		return ""
	}
	insertScalarField(ai, "role", mappedRole, "!!str", "model")
	if drop {
		removeField(ai, "model")
		return fmt.Sprintf("model %q → role %q (model dropped)", model, mappedRole)
	}
	return fmt.Sprintf("model %q → role %q (model retained for back-compat)", model, mappedRole)
}

// ensureBudget inserts a default budget block on a node mapping when
// none is present. Returns true iff a block was added.
func ensureBudget(nodeMap *yaml.Node) bool {
	if mappingField(nodeMap, "budget") != nil {
		return false
	}
	budget := &yaml.Node{
		Kind: yaml.MappingNode,
		Tag:  "!!map",
		Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "max_tokens"},
			{Kind: yaml.ScalarNode, Tag: "!!int", Value: "50000"},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "max_dollars"},
			{Kind: yaml.ScalarNode, Tag: "!!float", Value: "1.00"},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "on_overrun"},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "fail"},
		},
	}
	key := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "budget"}
	nodeMap.Content = append(nodeMap.Content, key, budget)
	return true
}

// parseDoc parses YAML bytes into a yaml.v3 Node document. We keep the
// document node (rather than reaching straight to its content) so
// re-serialising preserves the document marker if the input had one.
func parseDoc(in []byte) (*yaml.Node, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(in, &root); err != nil {
		return nil, fmt.Errorf("codemod: parse yaml: %w", err)
	}
	return &root, nil
}

// docNode returns the inner mapping node of a document, descending
// past the document wrapper. Returns nil for empty inputs.
func docNode(root *yaml.Node) *yaml.Node {
	if root == nil || root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return nil
	}
	return root.Content[0]
}

// serialiseDoc encodes a node back to bytes, preserving comments and
// (mostly) formatting. yaml.v3's default indent of 4 reads odd against
// the existing 2-space style in this repo, so we pin indent=2.
func serialiseDoc(root *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return nil, fmt.Errorf("codemod: encode yaml: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("codemod: close encoder: %w", err)
	}
	return buf.Bytes(), nil
}

// walkNodes invokes fn for every node mapping in the workflow's
// `nodes:` list, recursing into loop bodies. parent is "" at the root
// and the loop node id inside a loop body — used to qualify diagnostic
// references.
func walkNodes(doc *yaml.Node, fn func(parent string, nodeMap *yaml.Node)) {
	nodes := sequenceField(doc, "nodes")
	if nodes == nil {
		return
	}
	for _, n := range nodes.Content {
		if n.Kind != yaml.MappingNode {
			continue
		}
		fn("", n)
		// Recurse into loop bodies. Loop child nodes live under
		// loop.nodes (matching the Go struct).
		loop := mappingField(n, "loop")
		if loop == nil {
			continue
		}
		loopID := scalarField(n, "id")
		childSeq := sequenceField(loop, "nodes")
		if childSeq == nil {
			continue
		}
		for _, child := range childSeq.Content {
			if child.Kind != yaml.MappingNode {
				continue
			}
			fn(loopID, child)
		}
	}
}

// scalarField returns the string value of a scalar field on a mapping
// node, or "" if the field is absent or not a scalar.
func scalarField(m *yaml.Node, key string) string {
	if m == nil || m.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key && m.Content[i+1].Kind == yaml.ScalarNode {
			return m.Content[i+1].Value
		}
	}
	return ""
}

// mappingField returns the mapping-typed value of a field, or nil if
// the field is absent or not a mapping.
func mappingField(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key && m.Content[i+1].Kind == yaml.MappingNode {
			return m.Content[i+1]
		}
	}
	return nil
}

// sequenceField returns the sequence-typed value of a field, or nil if
// the field is absent or not a sequence.
func sequenceField(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key && m.Content[i+1].Kind == yaml.SequenceNode {
			return m.Content[i+1]
		}
	}
	return nil
}

// ensureScalarField sets a scalar field on a mapping node when absent.
// Returns true iff the field was added (false when the field already
// existed, regardless of its value — the codemod doesn't overwrite).
func ensureScalarField(m *yaml.Node, key, value, tag string) bool {
	if scalarField(m, key) != "" {
		return false
	}
	// Insert at the top so schema_version reads first in the file —
	// matches the canonical example in ROADMAP.md.
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	valNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value}
	m.Content = append([]*yaml.Node{keyNode, valNode}, m.Content...)
	return true
}

// insertScalarField adds a scalar field immediately before the field
// named anchor (for grouping role: alongside model:). If anchor is "",
// or absent from the mapping, the field is appended.
func insertScalarField(m *yaml.Node, key, value, tag, anchor string) {
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	valNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == anchor {
			m.Content = append(m.Content[:i],
				append([]*yaml.Node{keyNode, valNode}, m.Content[i:]...)...)
			return
		}
	}
	m.Content = append(m.Content, keyNode, valNode)
}

// removeField deletes a key/value pair from a mapping node. Silently
// no-ops when the key is absent.
func removeField(m *yaml.Node, key string) {
	if m == nil {
		return
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return
		}
	}
}

// kindName renders a yaml.Kind for diagnostics ("mapping", "sequence",
// etc.). yaml.v3 doesn't ship a Stringer.
func kindName(n *yaml.Node) string {
	if n == nil {
		return "nil"
	}
	switch n.Kind {
	case yaml.DocumentNode:
		return "document"
	case yaml.MappingNode:
		return "mapping"
	case yaml.SequenceNode:
		return "sequence"
	case yaml.ScalarNode:
		return "scalar"
	case yaml.AliasNode:
		return "alias"
	default:
		return "unknown"
	}
}

// nodeRef formats a diagnostic node reference (loopID/nodeID or just
// nodeID at the top level).
func nodeRef(parent, id string) string {
	if parent == "" {
		return id
	}
	return parent + "/" + id
}

// WriteResult emits a Result's diff-friendly summary to w. Used by the
// CLI for both the apply and dry-run paths.
func WriteResult(w io.Writer, r Result) error {
	if !r.Changed() {
		_, err := fmt.Fprintf(w, "no-op  %s\n", r.Path)
		return err
	}
	if _, err := fmt.Fprintf(w, "changed %s (%d transformation%s)\n", r.Path, len(r.Changes), pluralS(len(r.Changes))); err != nil {
		return err
	}
	for _, c := range r.Changes {
		if _, err := fmt.Fprintf(w, "  - %s\n", c); err != nil {
			return err
		}
	}
	return nil
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
