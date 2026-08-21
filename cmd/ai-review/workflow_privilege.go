package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// requiredCheckOwningWorkflows are workflow files whose job `name:` is a
// provider-required branch-protection status context in
// .github/rulesets/main.json (audited 2026-08-20, PR #1205 review). Ownership
// alone makes a content change to the file security relevant, independent of
// the permission scopes or trigger events workflowPrivilegeReason and
// changedWorkflowSchedule inspect: a job can be gutted into an unconditional
// pass without ever touching a write scope, secret, or privileged event,
// silently neutralizing the merge gate the context enforces.
// structural-health.yml is the concrete case that motivated this rule: it
// declares only `contents: read` on ordinary push/pull_request triggers, so
// none of the other privileged-workflow rules fire for it, yet it is the sole
// source of the required `Structural Health (baselined)` context. The other
// entries are already privileged through an existing rule (workflow_call
// delegation or a schedule trigger) today, but are listed here too so this
// invariant does not silently lapse if that incidental coverage changes.
var requiredCheckOwningWorkflows = map[string]bool{
	".github/workflows/ci.yml":                true,
	".github/workflows/codeql.yml":            true,
	".github/workflows/language-policy.yml":   true,
	".github/workflows/sbom-scan.yml":         true,
	".github/workflows/adr-integrity.yml":     true,
	".github/workflows/doc-header-lint.yml":   true,
	".github/workflows/structural-health.yml": true,
}

// requiredCheckOwnerReason reports the escalation reason, if any, for a
// changed workflow identity that owns a provider-required status context.
func requiredCheckOwnerReason(path string) (string, bool) {
	if requiredCheckOwningWorkflows[normalizedPathIdentity(path)] {
		return "workflow owns a provider-required branch-protection status check", true
	}
	return "", false
}

func privilegedWorkflowEscalationTriggers(ctx context.Context, base, head string, changedPaths []string) []string {
	trees, err := loadTreeIdentityEvidence(ctx, base, head)
	if err != nil {
		changed := changedWorkflowIdentities(changedPaths)
		if len(changed) == 0 {
			return nil
		}
		return []string{workflowEvidenceFailure(changed)}
	}
	return privilegedWorkflowEscalationTriggersWithEvidence(ctx, trees, changedPaths)
}

func privilegedWorkflowEscalationTriggersWithEvidence(ctx context.Context, trees treeIdentityEvidence, changedPaths []string) []string {
	changed := changedWorkflowIdentities(changedPaths)
	if len(changed) == 0 {
		return nil
	}
	if len(changed) > maxWorkflowEvidenceFiles {
		return []string{workflowEvidenceFailure(changed)}
	}

	baseEvidence, err := loadWorkflowEvidence(ctx, trees.base, changed)
	if err != nil {
		return []string{workflowEvidenceFailure(changed)}
	}
	headEvidence, err := loadWorkflowEvidence(ctx, trees.head, changed)
	if err != nil {
		return []string{workflowEvidenceFailure(changed)}
	}

	identities := make([]string, 0, len(changed))
	for identity := range changed {
		identities = append(identities, identity)
	}
	sort.Strings(identities)

	var triggers []string
	for _, identity := range identities {
		paths := changed[identity]
		basePeers := baseEvidence[identity]
		headPeers := headEvidence[identity]
		peers := append(append([]workflowBlobEvidence{}, basePeers...), headPeers...)
		if len(peers) == 0 {
			triggers = append(triggers, fmt.Sprintf("privileged workflow evidence cannot be authenticated within bounds (%s)", paths[0]))
			continue
		}
		if trigger, changed := changedWorkflowTrigger(paths[0], basePeers, headPeers, peers); changed {
			triggers = append(triggers, trigger)
		}
	}
	return triggers
}

// changedWorkflowTrigger reports the escalation trigger, if any, for one
// changed workflow identity. A workflow whose canonical content is unchanged
// across the revisions carries no trigger at all.
func changedWorkflowTrigger(path string, basePeers, headPeers, peers []workflowBlobEvidence) (string, bool) {
	equal, err := semanticallyEqualWorkflowEvidence(basePeers, headPeers)
	if err != nil {
		return fmt.Sprintf("privileged workflow evidence is malformed or ambiguous (%s)", path), true
	}
	if equal {
		return "", false
	}
	if reason, owns := requiredCheckOwnerReason(path); owns {
		return fmt.Sprintf("privileged workflow authority change (%s; %s)", path, reason), true
	}
	for _, peer := range peers {
		reason, privileged := workflowPrivilegeReason(peer.blob)
		if !privileged {
			continue
		}
		return fmt.Sprintf("privileged workflow authority change (%s; %s in %s)", path, reason, peer.path), true
	}
	scheduleChanged, err := changedWorkflowSchedule(basePeers, headPeers)
	if err != nil {
		return fmt.Sprintf("privileged workflow evidence is malformed or ambiguous (%s)", path), true
	}
	if scheduleChanged {
		return fmt.Sprintf("workflow schedule change alters unattended, billed execution (%s)", path), true
	}
	return "", false
}

// changedWorkflowSchedule reports whether a workflow's `on: schedule` block
// differs across the revisions. Schedule frequency is not authority the
// permission classifier can see: an otherwise read-only workflow moved from
// weekly to every few minutes still holds no write scope, yet it multiplies
// billed runner minutes and unattended executions. Only workflows whose
// canonical content already changed reach this check, so an unrelated edit to
// a long-standing scheduled workflow stays on the automated path.
func changedWorkflowSchedule(base, head []workflowBlobEvidence) (bool, error) {
	baseSchedules, err := canonicalWorkflowSchedules(base)
	if err != nil {
		return false, err
	}
	headSchedules, err := canonicalWorkflowSchedules(head)
	if err != nil {
		return false, err
	}
	return !slices.Equal(baseSchedules, headSchedules), nil
}

// canonicalWorkflowSchedules returns the declared schedules only, independent
// of each peer's raw path spelling. Peers with no schedule contribute
// nothing, so adding or deleting an unscheduled workflow compares equal and
// stays on the automated path, while adding, retiming, or removing a cron
// entry does not. Comparing schedule content alone, rather than
// path-prefixed content, keeps a pure case- or Unicode-normalization-only
// rename of an already-scheduled workflow (same folded workflowIdentity, but
// a base peer path spelled differently from its head peer) from reporting a
// schedule change that never happened: changedWorkflowTrigger only reaches
// this check once semanticallyEqualWorkflowEvidence has already established
// that *something* changed, and that separate, path-inclusive comparison is
// the one responsible for detecting the rename itself.
func canonicalWorkflowSchedules(peers []workflowBlobEvidence) ([]string, error) {
	values := make([]string, 0, len(peers))
	for _, peer := range peers {
		schedule, err := canonicalWorkflowSchedule(peer.blob)
		if err != nil {
			return nil, err
		}
		if schedule == "" {
			continue
		}
		values = append(values, schedule)
	}
	sort.Strings(values)
	return values, nil
}

func canonicalWorkflowSchedule(blob []byte) (string, error) {
	root, _, err := parseWorkflowYAML(blob)
	if err != nil {
		return "", err
	}
	on, ok := mappingNodeValue(root, "on")
	if !ok || on.Kind != yaml.MappingNode {
		// A scalar or sequence `on` cannot carry cron entries, and a missing
		// `on` is already an authority reason upstream.
		return "", nil
	}
	// Fold the key for the same reason workflow_call does: the provider treats
	// authority-bearing event identifiers as one equivalence class, so folded
	// duplicates are ambiguous rather than benign.
	schedule, ok, ambiguous := mappingNodeValueASCIIFold(on, "schedule")
	if ambiguous {
		return "", errors.New("workflow schedule evidence is ambiguous")
	}
	if !ok {
		return "", nil
	}
	var canonical strings.Builder
	appendCanonicalWorkflowNode(&canonical, schedule)
	return canonical.String(), nil
}

func semanticallyEqualWorkflowEvidence(base, head []workflowBlobEvidence) (bool, error) {
	canonical := func(peers []workflowBlobEvidence) ([]string, error) {
		values := make([]string, 0, len(peers))
		for _, peer := range peers {
			_, encoded, err := parseWorkflowYAML(peer.blob)
			if err != nil {
				return nil, err
			}
			values = append(values, peer.path+"\x00"+encoded)
		}
		sort.Strings(values)
		return values, nil
	}
	baseCanonical, err := canonical(base)
	if err != nil {
		return false, err
	}
	headCanonical, err := canonical(head)
	if err != nil {
		return false, err
	}
	if len(baseCanonical) != len(headCanonical) {
		return false, nil
	}
	for i := range baseCanonical {
		if baseCanonical[i] != headCanonical[i] {
			return false, nil
		}
	}
	return true, nil
}
