package main

import (
	"context"
	"fmt"
	"sort"
)

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
		equal, err := semanticallyEqualWorkflowEvidence(basePeers, headPeers)
		if err != nil {
			triggers = append(triggers, fmt.Sprintf("privileged workflow evidence is malformed or ambiguous (%s)", paths[0]))
			continue
		}
		if equal {
			continue
		}
		for _, peer := range peers {
			reason, privileged := workflowPrivilegeReason(peer.blob)
			if !privileged {
				continue
			}
			triggers = append(triggers, fmt.Sprintf("privileged workflow authority change (%s; %s in %s)", paths[0], reason, peer.path))
			break
		}
	}
	return triggers
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
