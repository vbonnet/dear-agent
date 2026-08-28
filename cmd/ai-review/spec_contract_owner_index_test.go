package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/vbonnet/dear-agent/internal/gittest"
)

func TestBuildReviewPlan_UsesStrictEARSLintAndIgnoresFencedExamples(t *testing.T) {
	for _, tc := range []struct {
		name      string
		extra     string
		wantHuman bool
	}{
		{name: "invalid prose outside fence", extra: "A malformed sentence shall not match strict EARS.\n", wantHuman: true},
		{name: "stable requirement without EARS keyword", extra: "**MOD-02** When checked, the system must report it.\n", wantHuman: true},
		{name: "invalid prose inside fence", extra: "```text\nA malformed sentence shall not match strict EARS.\n```\n"},
		{name: "invalid prose inside HTML comment", extra: "<!--\nA malformed sentence shall not match strict EARS.\n-->\n"},
		{name: "invalid prose inside indented code", extra: "    A malformed sentence shall not match strict EARS.\n"},
		{name: "invalid prose inside container nested fence", extra: "> ```text\n> A malformed sentence shall not match strict EARS.\n> ```\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newReviewRepo(t)
			base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			spec := "# Contract\n\n**MOD-01** When checked, the system shall report it.\n\n" + tc.extra + "\n## BDD Traceability\n\n- Feature: `features/module.feature`\n"
			writeReviewFile(t, repo, "module/SPEC.md", spec)
			writeReviewFile(t, repo, "features/module.feature", featureDocument("# SPEC: module/SPEC.md\n", "contract"))
			gittest.Run(t, repo, "add", "-A")
			gittest.Run(t, repo, "commit", "-m", "add contract")
			head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			chdir(t, repo)

			plan, err := buildReviewPlan(context.Background(), base, head)
			if err != nil {
				t.Fatal(err)
			}
			gotHuman := strings.Contains(strings.Join(plan.HumanReasons, "\n"), "strict EARS")
			if gotHuman != tc.wantHuman {
				t.Fatalf("HumanReasons = %v, strict EARS human=%t", plan.HumanReasons, tc.wantHuman)
			}
		})
	}
}

func TestBuildReviewPlan_ProvidesCompleteChangedContractAndSemanticCandidates(t *testing.T) {
	repo := newReviewRepo(t)
	writeReviewFile(t, repo, "shared/SPEC.md", specDocument("SHARED-01", "When a separate event occurs, the system shall emit a distinct signal.", "features/session.feature"))
	writeReviewFile(t, repo, "contracts/archive/SPEC.md", "# Archive contract\n\n**ARCH-01** When a session finalizes, the system shall retain its final outcome for observers.\n")
	writeReviewFile(t, repo, "unrelated/SPEC.md", "# Unrelated contract\n\n**OTHER-01** When storage fills, the system shall reject a new upload.\n")
	notes := strings.Repeat("Unchanged context line.\n\n", 12) + "REMOTE-CURRENT-CONTEXT\n"
	baseline := specDocument("SESSION-01", "When a session starts, the system shall record the initial state.", "features/session.feature") + "\n## Applicability\n\n" + notes
	writeReviewFile(t, repo, "domains/session/SPEC.md", baseline)
	writeReviewFile(t, repo, "features/session.feature", featureDocument("# SPEC: domains/session/SPEC.md\n# RELATED-SPEC: shared/SPEC.md\n", "session completion"))
	gittest.Run(t, repo, "add", "-A")
	gittest.Run(t, repo, "commit", "-m", "add product contracts")
	base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	changed := specDocument("SESSION-01", "When a session completes, the system shall retain the final outcome.", "features/session.feature") + "\n## Applicability\n\n" + notes
	writeReviewFile(t, repo, "domains/session/SPEC.md", changed)
	gittest.Run(t, repo, "add", "domains/session/SPEC.md")
	gittest.Run(t, repo, "commit", "-m", "change product contract")
	head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	chdir(t, repo)

	plan, err := buildReviewPlan(context.Background(), base, head)
	if err != nil {
		t.Fatal(err)
	}
	if plan.needsHuman() || len(plan.Contracts) != 1 || plan.Contracts[0].Content != changed {
		t.Fatalf("changed contract context = %#v, reasons=%v", plan.Contracts, plan.HumanReasons)
	}
	candidates := make(map[string]string, len(plan.OwnerIndex))
	for _, candidate := range plan.OwnerIndex {
		candidates[candidate.Path] = strings.Join(candidate.Signals, "\n")
	}
	_, unrelatedIncluded := candidates["unrelated/SPEC.md"]
	if len(candidates) != 3 || !strings.Contains(candidates["shared/SPEC.md"], "shared BDD backlink") || !strings.Contains(candidates["contracts/archive/SPEC.md"], "contract term") || !unrelatedIncluded {
		t.Fatalf("semantic candidates = %#v, want every unchanged contract with BDD and lexical ranking signals", plan.OwnerIndex)
	}
	if !plan.OwnerIndexComplete || plan.ActiveHarnessInventory.Revision != base || len(plan.ActiveHarnessInventory.Members) != 5 || len(plan.Applicability) != 5 || len(plan.ContractForms) != 1 || plan.ContractForms[0].VisibleContractDigest != visibleContractDigest(changed) || !slices.Equal(plan.ContractForms[0].StableRequirementIDs, []string{"SESSION-01"}) {
		t.Fatalf("authenticated active-member applicability evidence is incomplete: inventory=%#v applicability=%#v complete=%t", plan.ActiveHarnessInventory, plan.Applicability, plan.OwnerIndexComplete)
	}
	completeOwnerSearchForTest(t, &plan)
	incomplete := plan
	incomplete.Applicability = append([]specApplicabilityEvidence(nil), plan.Applicability[:len(plan.Applicability)-1]...)
	if _, _, err := specReviewPrompts(incomplete); err == nil || !strings.Contains(err.Error(), "incomplete active-harness applicability evidence") {
		t.Fatalf("prompt accepted a truncated active-member applicability grid: %v", err)
	}
	tamperedForm := plan
	tamperedForm.ContractForms = append([]specContractFormEvidence(nil), plan.ContractForms...)
	tamperedForm.ContractForms[0].StableRequirementIDs = []string{"SESSION-OTHER"}
	if _, _, err := specReviewPrompts(tamperedForm); err == nil || !strings.Contains(err.Error(), "incomplete changed-contract form evidence") {
		t.Fatalf("prompt accepted contract-form evidence with inexact stable IDs: %v", err)
	}
	_, prompt, err := specReviewPrompts(plan)
	if err != nil || !strings.Contains(prompt, "semantic_owner_search") || !strings.Contains(prompt, "every authenticated candidate was classified distinct") || !strings.Contains(prompt, "contract_form_reviews") || !strings.Contains(prompt, "mixed-format") {
		t.Fatalf("final prompt does not authenticate the completed sharded owner search: %v\n%s", err, prompt)
	}
	_, classifierPrompt, err := semanticOwnerShardPrompts(plan, plan.OwnerShards[0])
	if err != nil || !strings.Contains(classifierPrompt, "possible-owner") || !strings.Contains(classifierPrompt, "uncertain") || !strings.Contains(classifierPrompt, "do not omit, add, duplicate, or reorder") {
		t.Fatalf("classifier prompt does not route uncertain ownership to human review: %v\n%s", err, classifierPrompt)
	}
	if !strings.Contains(plan.Contracts[0].Content, "REMOTE-CURRENT-CONTEXT") {
		t.Fatal("complete current contract context was not retained")
	}
	if len(plan.Contracts[0].Features) != 1 || len(plan.Contracts[0].Features[0].Scenarios) != 1 || !strings.Contains(plan.Contracts[0].Features[0].Content, "Scenario:") {
		t.Fatalf("complete authenticated BDD context was not retained: %#v", plan.Contracts[0].Features)
	}
	if strings.Contains(plan.Diff, "REMOTE-CURRENT-CONTEXT") {
		t.Fatalf("test did not distinguish complete contract context from diff hunks: %q", plan.Diff)
	}
}

func TestBuildReviewPlan_RejectsHarnessLocalNormativeOwnersWithoutPeerComparison(t *testing.T) {
	paths := []string{
		".claude/hooks/SPEC.md",
		".codex/hooks/SPEC.md",
		".agents/hooks/SPEC.md",
		".opencode/hooks/SPEC.md",
		".pi/hooks/SPEC.md",
		"harness/claude/SPEC.md",
		"harnesses/codex-cli/SPEC.md",
		"agy/SPEC.md",
		"agy-cli/SPEC.md",
		"antigravity/SPEC.md",
		"research-pipeline/.claude-plugin/SPEC.md",
		".claude/cmd/SPEC.md",
		".codex/internal/.claude/SPEC.md",
		"agm/internal/.codex/SPEC.md",
		"cmd/.claude/SPEC.md",
		"harnesses/future-harness/SPEC.md",
		"nested/plugins/future-harness/SPEC.md",
		".CLAUDE/hooks/SPEC.md",
		"AGY/hooks/SPEC.md",
		"nested/Plugins/future-harness/SPEC.md",
		".agentſ/hooks/SPEC.md",
		"nested/CLAUDE-PLUGIN/SPEC.md",
		"Harneſſes/future-harness/SPEC.md",
	}
	for index, path := range paths {
		t.Run(path, func(t *testing.T) {
			repo := newReviewRepo(t)
			base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			feature := fmt.Sprintf("features/harness-%d.feature", index)
			writeReviewFile(t, repo, path, specDocument(fmt.Sprintf("LOCAL-%02d", index), "When a session completes, the system shall retain its final outcome.", feature))
			writeReviewFile(t, repo, feature, featureDocument("# SPEC: "+path+"\n", "local harness contract"))
			gittest.Run(t, repo, "add", "-A")
			gittest.Run(t, repo, "commit", "-m", "add harness-local contract")
			head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			chdir(t, repo)

			plan, err := buildReviewPlan(context.Background(), base, head)
			if err != nil {
				t.Fatal(err)
			}
			if !plan.needsHuman() || !strings.Contains(strings.Join(plan.HumanReasons, "\n"), "harness-local normative SPEC ownership is forbidden") || len(plan.OwnerIndex) != 0 || plan.OwnerIndexComplete {
				t.Fatalf("harness-local owner was not rejected before semantic peer comparison: %#v", plan)
			}
		})
	}
}

func TestBuildReviewPlan_AllowsLogicalDomainOwnersUnderInternalAndCmd(t *testing.T) {
	for index, path := range []string{"internal/session/SPEC.md", "cmd/session/SPEC.md", "agm/internal/claude/SPEC.md", "agm/cmd/codex/SPEC.md", "pkg/plugin/SPEC.md", "agm/tests/e2e-install/Dockerfiles/SPEC.md"} {
		t.Run(path, func(t *testing.T) {
			repo := newReviewRepo(t)
			base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			feature := fmt.Sprintf("features/domain-%d.feature", index)
			writeReviewFile(t, repo, path, specDocument(fmt.Sprintf("DOMAIN-%02d", index), "When a session completes, the system shall retain its final outcome.", feature))
			writeReviewFile(t, repo, feature, featureDocument("# SPEC: "+path+"\n", "logical domain contract"))
			gittest.Run(t, repo, "add", "-A")
			gittest.Run(t, repo, "commit", "-m", "add logical domain contract")
			head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
			chdir(t, repo)

			plan, err := buildReviewPlan(context.Background(), base, head)
			if err != nil {
				t.Fatal(err)
			}
			if plan.needsHuman() || !plan.OwnerIndexComplete {
				t.Fatalf("logical domain owner was rejected by path: %#v", plan)
			}
		})
	}
}

func TestSemanticOwnerIndexPartitionsEveryUnchangedSPECExactlyOnce(t *testing.T) {
	changed := specWithoutTrace("OWN-01", "When an adapter completes, the system shall preserve the terminal result.")
	contracts := []changedSpecContract{{
		Path:    "domains/session/SPEC.md",
		Content: changed,
	}}
	changes := []specChange{{Path: "domains/session/SPEC.md", Status: "modified"}}
	corpus := map[string][]byte{"domains/session/SPEC.md": []byte(changed)}
	for index := range 598 {
		prefix := []string{"pkg", "cmd", "agm", "internal"}[index%4]
		path := fmt.Sprintf("%s/component-%03d/SPEC.md", prefix, index)
		body := fmt.Sprintf("When storage bucket %03d fills, the system shall reject a new upload.", index)
		corpus[path] = []byte(specWithoutTrace(fmt.Sprintf("OTHER-%03d", index), body))
	}
	index, reasons, err := semanticOwnerIndex(contracts, changes, corpus, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if len(reasons) != 0 || len(index) != 598 {
		t.Fatalf("owner index candidates = %d, reasons=%v; want complete 598-SPEC projection", len(index), reasons)
	}
	seen := make(map[string]bool, len(index))
	for ordinal, candidate := range index {
		if candidate.Ordinal != ordinal || seen[candidate.Path] || (ordinal > 0 && candidate.Path <= index[ordinal-1].Path) {
			t.Fatalf("owner index is duplicated, reordered, or has a bad ordinal at %d: %#v", ordinal, candidate)
		}
		seen[candidate.Path] = true
	}
	plan := reviewPlan{Version: specContractVersion, BaseSHA: strings.Repeat("a", 40), MergeBaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40), OwnerIndex: index}
	digest, shards, shardReasons, err := buildSemanticOwnerShards(plan)
	if err != nil {
		t.Fatal(err)
	}
	if digest == "" || len(shardReasons) != 0 || len(shards) == 0 || len(shards) > maxSemanticShards {
		t.Fatalf("bounded shards = %d digest=%q reasons=%v", len(shards), digest, shardReasons)
	}
	nextOrdinal := 0
	for _, shard := range shards {
		if len(shard.Candidates) == 0 || len(shard.Candidates) > maxSemanticShardCandidates {
			t.Fatalf("shard %d candidate count = %d", shard.Ordinal, len(shard.Candidates))
		}
		for _, candidate := range shard.Candidates {
			if candidate.Ordinal != nextOrdinal {
				t.Fatalf("shards skipped or reordered ordinal: got %d want %d", candidate.Ordinal, nextOrdinal)
			}
			nextOrdinal++
		}
	}
	if nextOrdinal != len(index) {
		t.Fatalf("shards covered %d candidates, want %d", nextOrdinal, len(index))
	}
}

func TestCurrentSPECCorpusFitsSemanticOwnerShardBounds(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	head := strings.TrimSpace(gittest.Run(t, repositoryRoot, "rev-parse", "--verify", "HEAD^{commit}"))
	chdir(t, repositoryRoot)
	corpus, err := loadHeadSpecCorpus(context.Background(), head)
	if err != nil {
		t.Fatal(err)
	}
	changed := specWithoutTrace("CHECK-01", "When the current corpus is reviewed, the system shall classify every unchanged owner.")
	index, reasons, err := semanticOwnerIndex(
		[]changedSpecContract{{Path: "synthetic/current/SPEC.md", Content: changed}},
		[]specChange{{Path: "synthetic/current/SPEC.md", Status: "modified"}},
		corpus,
		map[string]bool{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(reasons) != 0 {
		t.Fatalf("current %d-SPEC corpus exceeds semantic owner index bounds: %v", len(corpus), reasons)
	}
	legacyTelemetryFound := false
	for _, candidate := range index {
		if candidate.Path != "pkg/telemetry/SPEC.md" {
			continue
		}
		legacyTelemetryFound = len(candidate.RequirementIDs) == 0 && strings.Contains(candidate.VisibleContract, "FR-1.1") && strings.Contains(candidate.VisibleContract, "EventListener interface")
	}
	if !legacyTelemetryFound {
		t.Fatal("current legacy telemetry SPEC promises were absent from the bounded owner projection")
	}
	plan := reviewPlan{Version: specContractVersion, BaseSHA: strings.Repeat("a", 40), MergeBaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40), OwnerIndex: index}
	digest, shards, reasons, err := buildSemanticOwnerShards(plan)
	if err != nil {
		t.Fatal(err)
	}
	if digest == "" || len(reasons) != 0 || len(shards) == 0 || len(shards) > maxSemanticShards {
		t.Fatalf("current %d-SPEC corpus requires %d shards: digest=%q reasons=%v", len(corpus), len(shards), digest, reasons)
	}
	t.Logf("current corpus projects %d SPECs, including bounded legacy telemetry promises, into %d shards", len(index), len(shards))
}

func TestSemanticOwnerIndexPreservesBoundedMixedFormatNormativePromises(t *testing.T) {
	changed := specWithoutTrace("SESSION-01", "When a session completes, the system shall preserve its terminal result.")
	mixed := "# Mixed monitoring contract\n\n## EARS requirements\n\n**MON-01** When monitoring starts, the system shall expose its current status.\n\n## Functional Requirements\n\n- **FR-1.1**: EventListener interface for implementing listeners\n- **NFR-1.1**: Listener execution does not block event recording\n"
	index, reasons, err := semanticOwnerIndex(
		[]changedSpecContract{{Path: "domains/session/SPEC.md", Content: changed}},
		[]specChange{{Path: "domains/session/SPEC.md", Status: "modified"}},
		map[string][]byte{
			"domains/session/SPEC.md": []byte(changed),
			"pkg/monitoring/SPEC.md":  []byte(mixed),
		},
		map[string]bool{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(reasons) != 0 || len(index) != 1 || !slices.Equal(index[0].RequirementIDs, []string{"MON-01"}) || !strings.Contains(index[0].VisibleContract, "MON-01") || !strings.Contains(index[0].VisibleContract, "FR-1.1") || !strings.Contains(index[0].VisibleContract, "EventListener interface") {
		t.Fatalf("mixed-format normative projection=%#v reasons=%v", index, reasons)
	}
	base := strings.Repeat("a", 40)
	plan := reviewPlan{
		Version:      specContractVersion,
		BaseSHA:      base,
		MergeBaseSHA: base,
		HeadSHA:      strings.Repeat("b", 40),
		Policy:       specPolicyEvidence{Path: specAuthoringPolicyPath, Revision: base, Content: testSpecAuthoringPolicy},
		Changes:      []specChange{{Path: "domains/session/SPEC.md", Status: "modified"}},
		Contracts:    []changedSpecContract{{Path: "domains/session/SPEC.md", Status: "modified", Content: changed, FeaturePaths: []string{}, Features: []bddFeatureEvidence{}, RequirementChanges: []specRequirementDelta{}}},
		OwnerIndex:   index,
	}
	digest, shards, shardReasons, err := buildSemanticOwnerShards(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(shardReasons) != 0 || len(shards) != 1 {
		t.Fatalf("legacy owner shard=%#v reasons=%v", shards, shardReasons)
	}
	plan.OwnerIndexDigest = digest
	plan.OwnerShards = shards
	plan.OwnerIndexComplete = true
	_, prompt, err := semanticOwnerShardPrompts(plan, shards[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"visible_contract", "MON-01", "FR-1.1", "EventListener interface", "return uncertain when it is empty or insufficient"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("mixed-format owner classifier prompt omits %q: %s", expected, prompt)
		}
	}
}

func TestSemanticOwnerIndexIncludesZeroOverlapOwnerEvidence(t *testing.T) {
	changed := specWithoutTrace("OWN-01", "When a session ends, the system shall keep its final result.")
	owner := specWithoutTrace("TERM-01", "While execution terminates, observers shall persist completion evidence.")
	contracts := []changedSpecContract{{Path: "domains/session/SPEC.md", Content: changed}}
	changes := []specChange{{Path: "domains/session/SPEC.md", Status: "modified"}}
	corpus := map[string][]byte{
		"domains/session/SPEC.md": []byte(changed),
		"domains/outcome/SPEC.md": []byte(owner),
	}

	candidates, reasons, err := semanticOwnerIndex(contracts, changes, corpus, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if len(reasons) != 0 || len(candidates) != 1 || candidates[0].Path != "domains/outcome/SPEC.md" || len(candidates[0].Signals) != 0 {
		t.Fatalf("zero-overlap owner evidence candidates=%#v reasons=%v", candidates, reasons)
	}
}

func TestSemanticOwnerIndexAuthenticatesAndClassifiesEveryChangedPeer(t *testing.T) {
	firstPath := "domains/first/SPEC.md"
	secondPath := "domains/second/SPEC.md"
	first := specWithoutTrace("FIRST-01", "When a session ends, the system shall retain the terminal result for later inspection.")
	second := specWithoutTrace("SECOND-01", "When execution finishes, the system shall preserve its final outcome for subsequent inspection.")
	unchanged := specWithoutTrace("OTHER-01", "When storage fills, the system shall reject a new upload.")
	contracts := []changedSpecContract{
		{Path: firstPath, Status: "modified", Content: first, FeaturePaths: []string{}},
		{Path: secondPath, Status: "modified", Content: second, FeaturePaths: []string{}},
	}
	changes := []specChange{{Path: firstPath, Status: "modified"}, {Path: secondPath, Status: "modified"}}
	index, reasons, err := semanticOwnerIndex(
		contracts,
		changes,
		map[string][]byte{
			firstPath:               []byte(first),
			secondPath:              []byte(second),
			"domains/other/SPEC.md": []byte(unchanged),
		},
		map[string]bool{firstPath: true, secondPath: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(reasons) != 0 || len(index) != 3 {
		t.Fatalf("changed-peer owner index=%#v reasons=%v", index, reasons)
	}
	changedPeerOrdinals := make(map[string]int)
	for ordinal, candidate := range index {
		if candidate.Ordinal != ordinal {
			t.Fatalf("candidate ordinal=%d at index %d", candidate.Ordinal, ordinal)
		}
		if candidate.Path == firstPath || candidate.Path == secondPath {
			if !candidate.ChangedPeer || candidate.ChangedBDDBacklink || len(candidate.Signals) != 0 {
				t.Fatalf("changed peer retained self-derived advisory signals: %#v", candidate)
			}
			changedPeerOrdinals[candidate.Path] = ordinal
		} else if candidate.ChangedPeer {
			t.Fatalf("unchanged candidate was marked as a changed peer: %#v", candidate)
		}
	}
	if len(changedPeerOrdinals) != 2 {
		t.Fatalf("changed peer coverage=%v, want both changed contracts", changedPeerOrdinals)
	}

	base := strings.Repeat("a", 40)
	plan := reviewPlan{
		Version:      specContractVersion,
		BaseSHA:      base,
		MergeBaseSHA: base,
		HeadSHA:      strings.Repeat("b", 40),
		Policy:       specPolicyEvidence{Path: specAuthoringPolicyPath, Revision: base, Content: testSpecAuthoringPolicy},
		Changes:      changes,
		Contracts:    contracts,
		OwnerIndex:   index,
	}
	completeOwnerSearchForTest(t, &plan)
	if !validSemanticOwnerIndex(plan) {
		t.Fatal("complete changed-peer index failed independent authentication")
	}
	_, prompt, err := semanticOwnerShardPrompts(plan, plan.OwnerShards[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"changed_peer=false", "changed_peer=true", "never against itself", "every unordered pair of changed contracts"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("changed-peer prompt omitted %q: %s", required, prompt)
		}
	}

	rebuild := func(candidates []semanticOwnerCandidate) reviewPlan {
		t.Helper()
		mutated := plan
		mutated.OwnerIndex = append([]semanticOwnerCandidate(nil), candidates...)
		for ordinal := range mutated.OwnerIndex {
			mutated.OwnerIndex[ordinal].Ordinal = ordinal
		}
		digest, shards, shardReasons, buildErr := buildSemanticOwnerShards(mutated)
		if buildErr != nil || len(shardReasons) != 0 {
			t.Fatalf("rebuild mutated owner index: shards=%#v reasons=%v err=%v", shards, shardReasons, buildErr)
		}
		mutated.OwnerIndexDigest = digest
		mutated.OwnerShards = shards
		mutated.OwnerIndexComplete = true
		return mutated
	}
	missing := append([]semanticOwnerCandidate(nil), index...)
	missing = append(missing[:changedPeerOrdinals[secondPath]], missing[changedPeerOrdinals[secondPath]+1:]...)
	if validSemanticOwnerIndex(rebuild(missing)) {
		t.Fatal("an owner index missing one changed peer authenticated")
	}
	wrongMarker := append([]semanticOwnerCandidate(nil), index...)
	wrongMarker[changedPeerOrdinals[firstPath]].ChangedPeer = false
	if validSemanticOwnerIndex(rebuild(wrongMarker)) {
		t.Fatal("a changed peer with a downgraded marker authenticated")
	}
	selfSignal := append([]semanticOwnerCandidate(nil), index...)
	selfSignal[changedPeerOrdinals[firstPath]].Signals = []string{"matches its own promise"}
	if validSemanticOwnerIndex(rebuild(selfSignal)) {
		t.Fatal("a changed peer with self-derived signals authenticated")
	}

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		results := make([]semanticOwnerClassification, 0, len(plan.OwnerShards[0].Candidates))
		for _, candidate := range plan.OwnerShards[0].Candidates {
			relation := "distinct"
			if candidate.Ordinal == changedPeerOrdinals[secondPath] {
				relation = "possible-owner"
			}
			results = append(results, semanticOwnerClassification{Ordinal: candidate.Ordinal, Relation: relation, Rationale: "This changed promise may duplicate its peer."})
		}
		verdict := semanticOwnerShardVerdict{Version: plan.Version, BaseSHA: plan.BaseSHA, MergeBaseSHA: plan.MergeBaseSHA, HeadSHA: plan.HeadSHA, IndexDigest: plan.OwnerIndexDigest, ShardOrdinal: 0, ShardDigest: plan.OwnerShards[0].Digest, Results: results}
		raw, marshalErr := json.Marshal(verdict)
		if marshalErr != nil {
			t.Errorf("marshal semantic verdict: %v", marshalErr)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, modelResponse("end_turn", string(raw)))
	}))
	defer server.Close()
	client := anthropic.NewClient(option.WithAPIKey("test-key"), option.WithBaseURL(server.URL))
	verdict, err := reviewSpecContract(context.Background(), client, anthropic.ModelClaudeOpus4_8, anthropic.OutputConfigEffortHigh, plan)
	if err != nil {
		t.Fatal(err)
	}
	if verdict.Status != NeedsHumanReview || calls.Load() != 1 || len(verdict.Findings) != 1 || verdict.Findings[0].Path != secondPath || !strings.Contains(verdict.Findings[0].Message, "candidate SPEC") {
		t.Fatalf("changed-peer concern did not stop before final review: verdict=%#v calls=%d", verdict, calls.Load())
	}
}

func TestSemanticOwnerShardPromptIncludesCompleteChangedVisibleContract(t *testing.T) {
	plan := reviewableSemanticPlan(t, 1)
	plan.Contracts[0].Content += "\n## Legacy observable contract\n\nThe service returns a durable receipt after every successful completion.\n"
	_, prompt, err := semanticOwnerShardPrompts(plan, plan.OwnerShards[0])
	if err != nil {
		t.Fatal(err)
	}
	const marker = "Authenticated bounded ownership evidence:\n"
	_, raw, found := strings.Cut(prompt, marker)
	if !found {
		t.Fatal("semantic owner prompt omitted authenticated evidence")
	}
	var evidence semanticOwnerShardPromptEvidence
	if err := json.Unmarshal([]byte(raw), &evidence); err != nil {
		t.Fatalf("decode semantic owner evidence: %v", err)
	}
	if len(evidence.ChangedContracts) != 1 || !strings.Contains(evidence.ChangedContracts[0].VisibleContract, "Legacy observable contract") || !strings.Contains(evidence.ChangedContracts[0].VisibleContract, "durable receipt") {
		t.Fatalf("changed visible contract = %#v, want complete legacy prose", evidence.ChangedContracts)
	}
	if !strings.Contains(prompt, "Changed contracts and every candidate contain complete bounded visible contract text") {
		t.Fatal("semantic owner classifier instructions do not require reading changed visible contracts")
	}
}

func TestBuildReviewPlan_ShardsOwnerCorpusBeyondFormerTwelveCandidateCap(t *testing.T) {
	repo := newReviewRepo(t)
	for index := range 20 {
		path := fmt.Sprintf("domains/candidate-%02d/SPEC.md", index)
		body := fmt.Sprintf("When storage bucket %03d fills, the system shall reject a new upload.", index)
		writeReviewFile(t, repo, path, specWithoutTrace(fmt.Sprintf("CAND-%03d", index), body))
	}
	baseline := specDocument("SESSION-01", "When session archival starts, the system shall record the initial state.", "features/session.feature")
	writeReviewFile(t, repo, "domains/session/SPEC.md", baseline)
	writeReviewFile(t, repo, "features/session.feature", featureDocument("# SPEC: domains/session/SPEC.md\n", "session archival"))
	gittest.Run(t, repo, "add", "-A")
	gittest.Run(t, repo, "commit", "-m", "add candidate corpus")
	base := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	changed := specDocument("SESSION-01", "When session archival completes, the system shall preserve the terminal outcome.", "features/session.feature")
	writeReviewFile(t, repo, "domains/session/SPEC.md", changed)
	gittest.Run(t, repo, "add", "domains/session/SPEC.md")
	gittest.Run(t, repo, "commit", "-m", "change shared contract")
	head := strings.TrimSpace(gittest.Run(t, repo, "rev-parse", "HEAD"))
	chdir(t, repo)

	plan, err := buildReviewPlan(context.Background(), base, head)
	if err != nil {
		t.Fatal(err)
	}
	if plan.needsHuman() || !plan.OwnerIndexComplete || len(plan.OwnerIndex) != 20 || len(plan.OwnerShards) == 0 {
		t.Fatalf("bounded exhaustive owner search did not admit the ordinary corpus: %#v", plan)
	}
}
