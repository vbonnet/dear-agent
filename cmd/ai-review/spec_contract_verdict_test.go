package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/internal/gittest"
)

func TestParseSpecContractVerdict_RejectsUnknownFieldsAndUnauthenticatedEvidence(t *testing.T) {
	plan := reviewPlan{
		Version:      specContractVersion,
		BaseSHA:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		MergeBaseSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		HeadSHA:      "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		ReviewNeeded: true,
		Changes:      []specChange{{Path: "SPEC.md", Status: "modified"}},
	}
	valid := fmt.Sprintf(`{"version":%q,"base_sha":%q,"merge_base_sha":%q,"head_sha":%q,"changes":[{"path":"SPEC.md","status":"modified"}],"status":"approved","summary":"Contract has one owner and reciprocal traceability.","deletion_reviews":[],"contract_form_reviews":[],"applicability_reviews":[],"findings":[]}`, specContractVersion, plan.BaseSHA, plan.MergeBaseSHA, plan.HeadSHA)
	if verdict, err := parseSpecContractVerdict([]byte(valid), plan); err != nil || verdict.Status != Approved {
		t.Fatalf("valid verdict = %#v, %v", verdict, err)
	}
	for _, raw := range []string{
		valid[:len(valid)-1] + `,"override":true}`,
		fmt.Sprintf(`{"version":%q,"base_sha":"different","merge_base_sha":%q,"head_sha":%q,"changes":[{"path":"SPEC.md","status":"modified"}],"status":"approved","summary":"Contract has one owner and reciprocal traceability.","deletion_reviews":[],"contract_form_reviews":[],"applicability_reviews":[],"findings":[]}`, specContractVersion, plan.MergeBaseSHA, plan.HeadSHA),
		fmt.Sprintf(`{"version":%q,"base_sha":%q,"merge_base_sha":"different","head_sha":%q,"changes":[{"path":"SPEC.md","status":"modified"}],"status":"approved","summary":"Contract has one owner and reciprocal traceability.","deletion_reviews":[],"contract_form_reviews":[],"applicability_reviews":[],"findings":[]}`, specContractVersion, plan.BaseSHA, plan.HeadSHA),
		fmt.Sprintf(`{"version":%q,"base_sha":%q,"merge_base_sha":%q,"head_sha":%q,"changes":[{"path":"other/SPEC.md","status":"modified"}],"status":"approved","summary":"Contract has one owner and reciprocal traceability.","deletion_reviews":[],"contract_form_reviews":[],"applicability_reviews":[],"findings":[]}`, specContractVersion, plan.BaseSHA, plan.MergeBaseSHA, plan.HeadSHA),
		fmt.Sprintf(`{"version":%q,"base_sha":%q,"merge_base_sha":%q,"head_sha":%q,"changes":null,"status":"approved","summary":"Contract has one owner and reciprocal traceability.","deletion_reviews":[],"contract_form_reviews":[],"applicability_reviews":[],"findings":[]}`, specContractVersion, plan.BaseSHA, plan.MergeBaseSHA, plan.HeadSHA),
		fmt.Sprintf(`{"version":%q,"base_sha":%q,"merge_base_sha":%q,"head_sha":%q,"changes":[{"path":"SPEC.md","status":"modified"}],"status":"approved","summary":"Contract has one owner and reciprocal traceability.","deletion_reviews":null,"contract_form_reviews":[],"applicability_reviews":[],"findings":[]}`, specContractVersion, plan.BaseSHA, plan.MergeBaseSHA, plan.HeadSHA),
		fmt.Sprintf(`{"version":%q,"base_sha":%q,"merge_base_sha":%q,"head_sha":%q,"changes":[{"path":"SPEC.md","status":"modified"}],"status":"approved","summary":"Contract has one owner and reciprocal traceability.","deletion_reviews":[],"contract_form_reviews":[],"applicability_reviews":[],"findings":null}`, specContractVersion, plan.BaseSHA, plan.MergeBaseSHA, plan.HeadSHA),
	} {
		if _, err := parseSpecContractVerdict([]byte(raw), plan); err == nil {
			t.Fatalf("accepted unsafe verdict %s", raw)
		}
	}
}

func TestParseSpecContractVerdict_RequiresEveryAuthenticatedApplicabilityDisposition(t *testing.T) {
	plan := reviewPlan{
		Version:      specContractVersion,
		BaseSHA:      "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		MergeBaseSHA: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		HeadSHA:      "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Changes:      []specChange{{Path: "domains/session/SPEC.md", Status: "modified"}},
		Applicability: []specApplicabilityEvidence{
			{Path: "domains/session/SPEC.md", RequirementID: "SESSION-01", Promise: "When a session completes, the system shall retain it.", Harness: "claude-code"},
			{Path: "domains/session/SPEC.md", RequirementID: "SESSION-01", Promise: "When a session completes, the system shall retain it.", Harness: "codex-cli"},
		},
	}
	validReviews := `[{"path":"domains/session/SPEC.md","requirement_id":"SESSION-01","harness":"claude-code","disposition":"supported","rationale":"The stable promise applies."},{"path":"domains/session/SPEC.md","requirement_id":"SESSION-01","harness":"codex-cli","disposition":"adapted","rationale":"The scoped adaptation applies."}]`
	verdictJSON := func(reviews string) string {
		return fmt.Sprintf(`{"version":%q,"base_sha":%q,"merge_base_sha":%q,"head_sha":%q,"changes":[{"path":"domains/session/SPEC.md","status":"modified"}],"status":"approved","summary":"Every active member has a final disposition.","deletion_reviews":[],"contract_form_reviews":[],"applicability_reviews":%s,"findings":[]}`, specContractVersion, plan.BaseSHA, plan.MergeBaseSHA, plan.HeadSHA, reviews)
	}
	if verdict, err := parseSpecContractVerdict([]byte(verdictJSON(validReviews)), plan); err != nil || len(verdict.ApplicabilityReviews) != 2 {
		t.Fatalf("complete applicability verdict = %#v, %v", verdict, err)
	}
	for _, reviews := range []string{
		`[]`,
		`null`,
		`[{"path":"domains/session/SPEC.md","requirement_id":"SESSION-01","harness":"codex-cli","disposition":"supported","rationale":"Wrong evidence order."},{"path":"domains/session/SPEC.md","requirement_id":"SESSION-01","harness":"claude-code","disposition":"supported","rationale":"Wrong evidence order."}]`,
		`[{"path":"domains/session/SPEC.md","requirement_id":"SESSION-01","harness":"claude-code","disposition":"unknown","rationale":"Unknown is not final."},{"path":"domains/session/SPEC.md","requirement_id":"SESSION-01","harness":"codex-cli","disposition":"supported","rationale":"The shared outcome applies."}]`,
	} {
		if _, err := parseSpecContractVerdict([]byte(verdictJSON(reviews)), plan); err == nil {
			t.Fatalf("accepted incomplete or unauthenticated applicability reviews: %s", reviews)
		}
	}
}

func TestParseSpecContractVerdict_BindsCompleteContractFormAndStatus(t *testing.T) {
	content := "# Contract\n\n**SESSION-01** When a session ends, the system shall persist it.\n\n## Purpose\n\nThe service also emits a durable receipt after completion.\n"
	evidence := specContractFormEvidence{
		Path:                  "domains/session/SPEC.md",
		VisibleContractDigest: visibleContractDigest(content),
		StableRequirementIDs:  []string{"SESSION-01"},
	}
	plan := reviewPlan{
		Version:       specContractVersion,
		BaseSHA:       strings.Repeat("a", 40),
		MergeBaseSHA:  strings.Repeat("a", 40),
		HeadSHA:       strings.Repeat("b", 40),
		Changes:       []specChange{{Path: evidence.Path, Status: "modified"}},
		Contracts:     []changedSpecContract{{Path: evidence.Path, Status: "modified", Content: content}},
		ContractForms: []specContractFormEvidence{evidence},
	}
	if !validContractFormEvidence(plan) {
		t.Fatal("exact visible-contract digest and stable IDs were not authenticated")
	}
	verdict := func(status, disposition string) []byte {
		document := specContractVerdictDocument{
			Version:         plan.Version,
			BaseSHA:         plan.BaseSHA,
			MergeBaseSHA:    plan.MergeBaseSHA,
			HeadSHA:         plan.HeadSHA,
			Changes:         plan.Changes,
			Status:          status,
			Summary:         "Contract prose was certified.",
			DeletionReviews: []specDeletionReview{},
			ContractFormReviews: []specContractFormReview{{
				Path:                  evidence.Path,
				VisibleContractDigest: evidence.VisibleContractDigest,
				Disposition:           disposition,
				Rationale:             "Mixed prose needs a stable ID.",
			}},
			ApplicabilityReviews: []specApplicabilityReview{},
			Findings:             []specFinding{},
		}
		raw, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	if _, err := parseSpecContractVerdict(verdict("approved", "needs-conversion"), plan); err == nil {
		t.Fatal("mixed-format observable prose certification was allowed to approve")
	}
	if parsed, err := parseSpecContractVerdict(verdict("needs-work", "needs-conversion"), plan); err != nil || parsed.Status != NeedsWork {
		t.Fatalf("needs-conversion verdict = %#v, %v", parsed, err)
	}
	if _, err := parseSpecContractVerdict(verdict("needs-work", "uncertain"), plan); err == nil {
		t.Fatal("uncertain complete-contract certification did not require human review")
	}
	if parsed, err := parseSpecContractVerdict(verdict("needs-human-review", "uncertain"), plan); err != nil || parsed.Status != NeedsHumanReview {
		t.Fatalf("uncertain contract-form verdict = %#v, %v", parsed, err)
	}
	wrongDigest := evidence
	wrongDigest.VisibleContractDigest = strings.Repeat("0", sha256.Size*2)
	plan.ContractForms[0] = wrongDigest
	if _, err := parseSpecContractVerdict(verdict("needs-work", "needs-conversion"), plan); err == nil {
		t.Fatal("contract-form verdict with a stale visible-content digest was accepted")
	}
}

func TestSemanticOwnerHumanVerdictFailsClosedForUnauthenticatedOrdinal(t *testing.T) {
	plan := reviewableSemanticPlan(t, 1)
	verdict := semanticOwnerHumanVerdict(plan, semanticOwnerClassification{
		Ordinal:   len(plan.OwnerIndex),
		Relation:  "uncertain",
		Rationale: "The candidate was not authenticated.",
	})
	if verdict.Status != NeedsHumanReview || !strings.Contains(verdict.Summary, "unauthenticated candidate ordinal") || len(verdict.Findings) != 0 {
		t.Fatalf("unauthenticated ordinal did not fail closed: %#v", verdict)
	}
}

func TestBuildReviewPlan_RequiresStructuredReviewForDeletedRequirements(t *testing.T) {
	repo := newReviewRepo(t)
	baseSpec := "# Contract\n\n**MOD-01** When checked, the system shall report it.\n\n**MOD-02** When removed, the system shall preserve an audit record.\n\n## BDD Traceability\n\n- Feature: `features/module.feature`\n"
	writeReviewFile(t, repo, "module/SPEC.md", baseSpec)
	writeReviewFile(t, repo, "features/module.feature", featureDocument("# SPEC: module/SPEC.md\n", "contract"))
	gittest.Run(t, repo, "add", "-A")
	gittest.Run(t, repo, "commit", "-m", "add two requirements")
	base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	writeReviewFile(t, repo, "module/SPEC.md", specDocument("MOD-01", "When checked, the system shall report it.", "features/module.feature"))
	gittest.Run(t, repo, "add", "module/SPEC.md")
	gittest.Run(t, repo, "commit", "-m", "remove requirement")
	head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	chdir(t, repo)

	plan, err := buildReviewPlan(context.Background(), base, head)
	if err != nil {
		t.Fatal(err)
	}
	deleted := deletedRequirementEvidence(plan)
	if plan.needsHuman() || len(deleted) != 1 || deleted[0].ID != "MOD-02" || deleted[0].Before == "" || deleted[0].After != "" {
		t.Fatalf("deleted requirement evidence = %#v, reasons=%v", deleted, plan.HumanReasons)
	}
	applicability := applicabilityReviewsJSON(t, plan, "supported")
	contractForms := contractFormReviewsJSON(t, plan, "complete")
	missing := fmt.Sprintf(`{"version":%q,"base_sha":%q,"merge_base_sha":%q,"head_sha":%q,"changes":[{"path":"module/SPEC.md","status":"modified"}],"status":"approved","summary":"Deletion is justified by the canonical owner.","deletion_reviews":[],"contract_form_reviews":%s,"applicability_reviews":%s,"findings":[]}`, specContractVersion, plan.BaseSHA, plan.MergeBaseSHA, plan.HeadSHA, contractForms, applicability)
	if _, err := parseSpecContractVerdict([]byte(missing), plan); err == nil {
		t.Fatal("accepted verdict without authenticated requirement deletion review")
	}
	justified := fmt.Sprintf(`{"version":%q,"base_sha":%q,"merge_base_sha":%q,"head_sha":%q,"changes":[{"path":"module/SPEC.md","status":"modified"}],"status":"approved","summary":"Deletion is justified by the canonical owner.","deletion_reviews":[{"path":"module/SPEC.md","requirement_id":"MOD-02","disposition":"justified","rationale":"Obsolete promise can be removed."}],"contract_form_reviews":%s,"applicability_reviews":%s,"findings":[]}`, specContractVersion, plan.BaseSHA, plan.MergeBaseSHA, plan.HeadSHA, contractForms, applicability)
	if verdict, err := parseSpecContractVerdict([]byte(justified), plan); err != nil || len(verdict.DeletionReviews) != 1 {
		t.Fatalf("justified deletion verdict = %#v, %v", verdict, err)
	}
	needsWork := fmt.Sprintf(`{"version":%q,"base_sha":%q,"merge_base_sha":%q,"head_sha":%q,"changes":[{"path":"module/SPEC.md","status":"modified"}],"status":"needs-work","summary":"The requirement deletion needs work.","deletion_reviews":[{"path":"module/SPEC.md","requirement_id":"MOD-02","disposition":"needs-work","rationale":"Removed promise still belongs."}],"contract_form_reviews":%s,"applicability_reviews":%s,"findings":[]}`, specContractVersion, plan.BaseSHA, plan.MergeBaseSHA, plan.HeadSHA, contractForms, applicability)
	if verdict, err := parseSpecContractVerdict([]byte(needsWork), plan); err != nil || verdict.Status != NeedsWork {
		t.Fatalf("deletion-only needs-work verdict = %#v, %v", verdict, err)
	}
}

func TestApplySpecVerdict_NeverUpgradesBlockingSpecResult(t *testing.T) {
	if got := applySpecVerdict(Approved, NeedsWork); got != NeedsWork {
		t.Fatalf("approved overall upgraded blocking SPEC verdict: %s", got)
	}
	if got := applySpecVerdict(Approved, NeedsHumanReview); got != NeedsHumanReview {
		t.Fatalf("approved overall upgraded human SPEC verdict: %s", got)
	}
	if got := applySpecVerdict(Rejected, NeedsHumanReview); got != NeedsHumanReview {
		t.Fatalf("human-review SPEC verdict was replaced by synthesis: %s", got)
	}
}
