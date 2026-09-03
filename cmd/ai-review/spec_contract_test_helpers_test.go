package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func writeReviewFile(t *testing.T, repo, relative, contents string) {
	t.Helper()
	path := filepath.Join(repo, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

const testSpecAuthoringPolicy = "# Authoring behavioral specifications\n\nThis protected-base guide is the canonical SPEC policy owner for tests.\n"
const testActiveHarnessRegistry = "package agent\n\nvar activeHarnesses = []string{\"claude-code\", \"codex-cli\", \"agy\", \"opencode-cli\", \"pi-cli\"}\n"

func newReviewRepo(t *testing.T) string {
	t.Helper()
	repo := gittest.NewRepo(t)
	writeReviewFile(t, repo, specAuthoringPolicyPath, testSpecAuthoringPolicy)
	writeReviewFile(t, repo, activeHarnessRegistryPath, testActiveHarnessRegistry)
	gittest.Run(t, repo, "add", specAuthoringPolicyPath, activeHarnessRegistryPath)
	gittest.Run(t, repo, "commit", "-m", "add protected-base SPEC policy")
	return repo
}

func specDocument(id, body, feature string) string {
	return fmt.Sprintf("# Contract\n\n**%s** %s\n\n## BDD Traceability\n\n- Feature: `%s`\n", id, body, feature)
}

func specWithoutTrace(id, body string) string {
	return fmt.Sprintf("# Contract\n\n**%s** %s\n", id, body)
}

func featureDocument(backlinks, name string) string {
	return fmt.Sprintf("%sFeature: %s\n\n  Scenario: exercise the contract\n    Given the contract behavior is exercised\n", backlinks, name)
}

func applicabilityReviewsJSON(t *testing.T, plan reviewPlan, disposition string) string {
	t.Helper()
	reviews := make([]specApplicabilityReview, 0, len(plan.Applicability))
	for _, evidence := range plan.Applicability {
		reviews = append(reviews, specApplicabilityReview{
			Path:          evidence.Path,
			RequirementID: evidence.RequirementID,
			Harness:       evidence.Harness,
			Disposition:   disposition,
			Rationale:     "The stable promise applies.",
		})
	}
	raw, err := json.Marshal(reviews)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func contractFormReviewsJSON(t *testing.T, plan reviewPlan, disposition string) string {
	t.Helper()
	reviews := make([]specContractFormReview, 0, len(plan.ContractForms))
	for _, evidence := range plan.ContractForms {
		reviews = append(reviews, specContractFormReview{
			Path:                  evidence.Path,
			VisibleContractDigest: evidence.VisibleContractDigest,
			Disposition:           disposition,
			Rationale:             "All promises map to stable IDs.",
		})
	}
	raw, err := json.Marshal(reviews)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func reviewableSemanticPlan(t *testing.T, candidateCount int) reviewPlan {
	t.Helper()
	base := strings.Repeat("a", 40)
	head := strings.Repeat("b", 40)
	contractContent := specWithoutTrace("SESSION-01", "When a session completes, the system shall retain its final outcome.")
	parsed, err := parseRequirements(contractContent)
	if err != nil || len(parsed) != 1 {
		t.Fatalf("parse test contract = %#v, %v", parsed, err)
	}
	members := make([]activeHarnessMemberEvidence, 0, 5)
	for _, name := range []string{"claude-code", "codex-cli", "agy", "opencode-cli", "pi-cli"} {
		member, ok := activeHarnessMember(name)
		if !ok {
			t.Fatalf("missing trusted harness member %s", name)
		}
		members = append(members, member)
	}
	plan := reviewPlan{
		Version:      specContractVersion,
		BaseSHA:      base,
		MergeBaseSHA: base,
		HeadSHA:      head,
		Policy:       specPolicyEvidence{Path: specAuthoringPolicyPath, Revision: base, Content: testSpecAuthoringPolicy},
		ActiveHarnessInventory: activeHarnessInventoryEvidence{
			Path:     activeHarnessRegistryPath,
			Revision: base,
			Members:  members,
		},
		Changes: []specChange{{Path: "domains/session/SPEC.md", Status: "modified"}},
		Contracts: []changedSpecContract{{
			Path:               "domains/session/SPEC.md",
			Status:             "modified",
			Content:            contractContent,
			FeaturePaths:       []string{},
			Features:           []bddFeatureEvidence{},
			RequirementChanges: []specRequirementDelta{},
		}},
		ContractForms: []specContractFormEvidence{{
			Path:                  "domains/session/SPEC.md",
			VisibleContractDigest: visibleContractDigest(contractContent),
			StableRequirementIDs:  []string{parsed[0].ID},
		}},
		Applicability:      []specApplicabilityEvidence{},
		OwnerIndex:         []semanticOwnerCandidate{},
		OwnerShards:        []semanticOwnerShard{},
		ReviewNeeded:       true,
		ReviewRelevant:     true,
		EscalationTriggers: []string{},
		HumanReasons:       []string{},
	}
	for _, member := range members {
		plan.Applicability = append(plan.Applicability, specApplicabilityEvidence{
			Path:          "domains/session/SPEC.md",
			RequirementID: parsed[0].ID,
			Promise:       parsed[0].Body,
			Harness:       member.Name,
		})
	}
	for ordinal := range candidateCount {
		plan.OwnerIndex = append(plan.OwnerIndex, semanticOwnerCandidate{
			Ordinal:         ordinal,
			Path:            fmt.Sprintf("domains/candidate-%03d/SPEC.md", ordinal),
			RequirementIDs:  []string{fmt.Sprintf("CAND-%03d", ordinal)},
			VisibleContract: fmt.Sprintf("**CAND-%03d** When storage bucket %03d fills, the system shall reject a new upload.", ordinal, ordinal),
			FeaturePaths:    []string{},
			Signals:         []string{},
		})
	}
	digest, shards, reasons, err := buildSemanticOwnerShards(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(reasons) != 0 {
		t.Fatalf("test plan owner bounds: %v", reasons)
	}
	plan.OwnerIndexDigest = digest
	plan.OwnerShards = shards
	plan.OwnerIndexComplete = true
	return plan
}

func completeOwnerSearchForTest(t *testing.T, plan *reviewPlan) {
	t.Helper()
	digest, shards, reasons, err := buildSemanticOwnerShards(*plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(reasons) != 0 {
		t.Fatalf("test owner index exceeds bounds: %v", reasons)
	}
	plan.OwnerIndexDigest = digest
	plan.OwnerShards = shards
	plan.OwnerIndexComplete = true
	receipts := make([]semanticOwnerShardReceipt, 0, len(shards))
	for index, shard := range shards {
		receipts = append(receipts, semanticOwnerShardReceipt{
			Ordinal:        index,
			ShardDigest:    shard.Digest,
			CandidateCount: len(shard.Candidates),
			VerdictDigest:  strings.Repeat("a", sha256.Size*2),
		})
	}
	plan.OwnerSearch = &semanticOwnerSearchEvidence{
		IndexDigest:    digest,
		CandidateCount: len(plan.OwnerIndex),
		ShardCount:     len(shards),
		Receipts:       receipts,
		Complete:       true,
	}
}
