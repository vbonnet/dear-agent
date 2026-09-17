package main

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/vbonnet/dear-agent/internal/gittest"
)

const (
	// The current authority branch contains a 177,631-byte SPEC whose candidate
	// is 177,539 bytes at its full-corpus ordinal. The synthetic candidate uses
	// the larger source byte count so the admission fixture is conservative.
	authorityBranchRawSPECBytes            = 177631
	authorityBranchMeasuredCandidateBytes  = 177539
	authorityBranchRequirementBodyBytes    = 29660
	authorityBranchRequirementsJSONBytes   = 35471
	authorityBranchAddedDeltasJSONBytes    = 47541
	authorityBranchSyntheticCandidateBytes = authorityBranchRawSPECBytes
	semanticCapacityTestOwnerPath          = "internal/buildauthority/SPEC.md"
)

func TestSemanticOwnerCandidateByteBoundary(t *testing.T) {
	contracts, changes := semanticCapacityTestChange()
	content := sizedSemanticOwnerContentForCorpus(
		t,
		contracts,
		changes,
		map[string][]byte{},
		maxSemanticCandidateBytes,
	)
	corpus := map[string][]byte{semanticCapacityTestOwnerPath: []byte(content)}
	index, reasons, err := semanticOwnerIndex(contracts, changes, corpus, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if len(reasons) != 0 || len(index) != 1 {
		t.Fatalf("exact candidate bound index=%d reasons=%v", len(index), reasons)
	}
	if size := semanticOwnerCandidateSize(t, index, semanticCapacityTestOwnerPath); size != maxSemanticCandidateBytes {
		t.Fatalf("exact candidate size = %d, want %d", size, maxSemanticCandidateBytes)
	}

	corpus[semanticCapacityTestOwnerPath] = []byte(content + "x")
	index, reasons, err = semanticOwnerIndex(contracts, changes, corpus, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf(
		"semantic owner projection is %d bytes and exceeds the %d-byte candidate limit (%s)",
		maxSemanticCandidateBytes+1,
		maxSemanticCandidateBytes,
		semanticCapacityTestOwnerPath,
	)
	if len(index) != 0 || !slices.Equal(reasons, []string{want}) {
		t.Fatalf("over-limit candidate index=%d reasons=%v, want %q", len(index), reasons, want)
	}
}

func TestSemanticOwnerShardByteBoundary(t *testing.T) {
	plan := reviewPlan{
		Version:          specContractVersion,
		BaseSHA:          strings.Repeat("a", 40),
		MergeBaseSHA:     strings.Repeat("a", 40),
		HeadSHA:          strings.Repeat("b", 40),
		OwnerIndexDigest: strings.Repeat("c", 64),
		OwnerIndex: []semanticOwnerCandidate{
			{Ordinal: 0, Path: "capacity/first/SPEC.md", RequirementIDs: []string{}, VisibleContract: "x", FeaturePaths: []string{}, Signals: []string{}},
			{Ordinal: 1, Path: "capacity/second/SPEC.md", RequirementIDs: []string{}, VisibleContract: "y", FeaturePaths: []string{}, Signals: []string{}},
		},
	}
	_, baseRaw, err := jsonDigest(semanticOwnerShardDocumentFor(plan, 0, plan.OwnerIndex))
	if err != nil {
		t.Fatal(err)
	}
	padding := maxSemanticShardBytes - len(baseRaw)
	if padding < 2 {
		t.Fatalf("semantic shard envelope leaves %d bytes for boundary padding", padding)
	}
	plan.OwnerIndex[0].VisibleContract += strings.Repeat("x", padding/2)
	plan.OwnerIndex[1].VisibleContract += strings.Repeat("y", padding-padding/2)
	for _, candidate := range plan.OwnerIndex {
		raw, err := json.Marshal(candidate)
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) > maxSemanticCandidateBytes {
			t.Fatalf("boundary fixture candidate %q is %d bytes, exceeds %d-byte candidate bound", candidate.Path, len(raw), maxSemanticCandidateBytes)
		}
	}
	_, exactRaw, err := jsonDigest(semanticOwnerShardDocumentFor(plan, 0, plan.OwnerIndex))
	if err != nil {
		t.Fatal(err)
	}
	if len(exactRaw) != maxSemanticShardBytes {
		t.Fatalf("exact semantic shard size = %d, want %d", len(exactRaw), maxSemanticShardBytes)
	}
	digest, shards, reasons, err := buildSemanticOwnerShards(plan)
	if err != nil {
		t.Fatal(err)
	}
	if digest == "" || len(reasons) != 0 || len(shards) != 1 {
		t.Fatalf("exact-bound semantic shard digest=%q shards=%d reasons=%v, want one admitted shard", digest, len(shards), reasons)
	}

	plan.OwnerIndex[1].VisibleContract += "z"
	digest, shards, reasons, err = buildSemanticOwnerShards(plan)
	if err != nil {
		t.Fatal(err)
	}
	if digest == "" || len(reasons) != 0 || len(shards) != 2 {
		t.Fatalf("one-byte-over semantic shard digest=%q shards=%d reasons=%v, want a bounded split into two shards", digest, len(shards), reasons)
	}
}

func TestSemanticOwnerIndexByteBoundary(t *testing.T) {
	plan := reviewPlan{
		Version:      specContractVersion,
		BaseSHA:      strings.Repeat("a", 40),
		MergeBaseSHA: strings.Repeat("a", 40),
		HeadSHA:      strings.Repeat("b", 40),
		OwnerIndex:   make([]semanticOwnerCandidate, 13),
	}
	for ordinal := range plan.OwnerIndex {
		plan.OwnerIndex[ordinal] = semanticOwnerCandidate{
			Ordinal:         ordinal,
			Path:            fmt.Sprintf("capacity/index-%02d/SPEC.md", ordinal),
			RequirementIDs:  []string{},
			VisibleContract: "x",
			FeaturePaths:    []string{},
			Signals:         []string{},
		}
	}
	indexDocument := func() semanticOwnerIndexDocument {
		return semanticOwnerIndexDocument{
			Version:      plan.Version,
			BaseSHA:      plan.BaseSHA,
			MergeBaseSHA: plan.MergeBaseSHA,
			HeadSHA:      plan.HeadSHA,
			Candidates:   plan.OwnerIndex,
		}
	}
	_, baseRaw, err := jsonDigest(indexDocument())
	if err != nil {
		t.Fatal(err)
	}
	remaining := maxSemanticIndexBytes - len(baseRaw)
	for index := range plan.OwnerIndex {
		candidateRaw, err := json.Marshal(plan.OwnerIndex[index])
		if err != nil {
			t.Fatal(err)
		}
		padding := min(remaining, maxSemanticCandidateBytes-len(candidateRaw))
		plan.OwnerIndex[index].VisibleContract += strings.Repeat("x", padding)
		remaining -= padding
	}
	if remaining != 0 {
		t.Fatalf("could not construct exact semantic index boundary; %d bytes remain", remaining)
	}
	_, exactRaw, err := jsonDigest(indexDocument())
	if err != nil {
		t.Fatal(err)
	}
	if len(exactRaw) != maxSemanticIndexBytes {
		t.Fatalf("exact semantic index size = %d, want %d", len(exactRaw), maxSemanticIndexBytes)
	}
	_, shards, reasons, err := buildSemanticOwnerShards(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(shards) != 0 || len(reasons) != 1 || strings.Contains(reasons[0], "semantic owner index is") {
		t.Fatalf("exact-bound semantic index shards=%d reasons=%v, want admission through the index bound followed by a shard-count refusal", len(shards), reasons)
	}

	plan.OwnerIndex[len(plan.OwnerIndex)-1].VisibleContract += "x"
	_, shards, reasons, err = buildSemanticOwnerShards(plan)
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf(
		"semantic owner index is %d bytes and exceeds the %d-byte review limit",
		maxSemanticIndexBytes+1,
		maxSemanticIndexBytes,
	)
	if len(shards) != 0 || !slices.Equal(reasons, []string{want}) {
		t.Fatalf("over-limit semantic index shards=%d reasons=%v, want %q", len(shards), reasons, want)
	}
}

func TestCurrentSPECCorpusRetainsLargeOwnerCapacityReserve(t *testing.T) {
	if authorityBranchSyntheticCandidateBytes <= authorityBranchMeasuredCandidateBytes {
		t.Fatalf(
			"synthetic authority candidate = %d, want above measured %d",
			authorityBranchSyntheticCandidateBytes,
			authorityBranchMeasuredCandidateBytes,
		)
	}
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	head := strings.TrimSpace(gittest.Run(t, repositoryRoot, "rev-parse", "--verify", "HEAD^{commit}"))
	chdir(t, repositoryRoot)
	corpus, err := loadHeadSpecCorpus(context.Background(), head)
	if err != nil {
		t.Fatal(err)
	}
	contracts, changes := semanticCapacityTestChange()
	corpus[semanticCapacityTestOwnerPath] = []byte(sizedSemanticOwnerContentForCorpus(
		t,
		contracts,
		changes,
		corpus,
		authorityBranchSyntheticCandidateBytes,
	))
	index, reasons, err := semanticOwnerIndex(contracts, changes, corpus, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if len(reasons) != 0 || len(index) != len(corpus) {
		t.Fatalf("branch-shaped owner index=%d corpus=%d reasons=%v", len(index), len(corpus), reasons)
	}
	plan := reviewPlan{
		Version:      specContractVersion,
		BaseSHA:      strings.Repeat("a", 40),
		MergeBaseSHA: strings.Repeat("a", 40),
		HeadSHA:      strings.Repeat("b", 40),
		Policy: specPolicyEvidence{
			Path:     specAuthoringPolicyPath,
			Revision: strings.Repeat("a", 40),
			Content:  testSpecAuthoringPolicy,
		},
		Changes:    changes,
		Contracts:  contracts,
		OwnerIndex: index,
	}
	digest, shards, reasons, err := buildSemanticOwnerShards(plan)
	if err != nil {
		t.Fatal(err)
	}
	if digest == "" || len(reasons) != 0 || len(shards) == 0 || len(shards) >= maxSemanticShards {
		t.Fatalf(
			"branch-shaped %d-SPEC corpus requires %d shards: digest=%q reasons=%v; want one-shard reserve below %d",
			len(corpus),
			len(shards),
			digest,
			reasons,
			maxSemanticShards,
		)
	}
	plan.OwnerIndexDigest = digest
	plan.OwnerShards = shards
	plan.OwnerIndexComplete = true
	if !validSemanticOwnerIndex(plan) {
		t.Fatal("branch-shaped semantic owner index failed independent authentication")
	}
	for _, shard := range shards {
		system, prompt, err := semanticOwnerShardPrompts(plan, shard)
		if err != nil {
			t.Fatalf("build authenticated prompt for shard %d: %v", shard.Ordinal, err)
		}
		if len(system)+len(prompt) > maxSpecPromptBytes {
			t.Fatalf("authenticated prompt for shard %d is %d bytes, exceeds %d-byte bound", shard.Ordinal, len(system)+len(prompt), maxSpecPromptBytes)
		}
	}

	var calls atomic.Int32
	var active atomic.Int32
	var peakActive atomic.Int32
	expectedPeak := int32(min(len(shards), maxConcurrentSemanticCalls))
	firstWaveReady := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		inflight := active.Add(1)
		defer active.Add(-1)
		for observed := peakActive.Load(); inflight > observed; observed = peakActive.Load() {
			if peakActive.CompareAndSwap(observed, inflight) {
				break
			}
		}
		if call == expectedPeak {
			close(firstWaveReady)
		}
		if call <= expectedPeak {
			select {
			case <-firstWaveReady:
			case <-r.Context().Done():
				return
			}
		}

		var request struct {
			Messages []struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) != 1 || len(request.Messages[0].Content) != 1 {
			t.Errorf("decode capacity model request: messages=%#v err=%v", request.Messages, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		const marker = "Authenticated bounded ownership evidence:\n"
		_, encoded, found := strings.Cut(request.Messages[0].Content[0].Text, marker)
		if !found {
			t.Error("capacity model request omitted authenticated ownership evidence")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var evidence semanticOwnerShardPromptEvidence
		if err := json.Unmarshal([]byte(encoded), &evidence); err != nil {
			t.Errorf("decode capacity owner prompt: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		results := make([]semanticOwnerClassification, 0, len(evidence.Shard.Candidates))
		for _, candidate := range evidence.Shard.Candidates {
			results = append(results, semanticOwnerClassification{
				Ordinal:   candidate.Ordinal,
				Relation:  "distinct",
				Rationale: "The bounded projection is distinct.",
			})
		}
		verdict := semanticOwnerShardVerdict{
			Version:      plan.Version,
			BaseSHA:      plan.BaseSHA,
			MergeBaseSHA: plan.MergeBaseSHA,
			HeadSHA:      plan.HeadSHA,
			IndexDigest:  plan.OwnerIndexDigest,
			ShardOrdinal: evidence.Shard.Ordinal,
			ShardDigest:  evidence.Shard.Digest,
			Results:      results,
		}
		raw, err := json.Marshal(verdict)
		if err != nil {
			t.Errorf("marshal capacity owner verdict: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, modelResponse("end_turn", string(raw)))
	}))
	defer server.Close()
	client := anthropic.NewClient(option.WithAPIKey("test-key"), option.WithBaseURL(server.URL))
	searchContext, cancelSearch := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancelSearch()
	search, concern, err := runSemanticOwnerSearch(searchContext, client, anthropic.ModelClaudeOpus4_8, anthropic.OutputConfigEffortHigh, plan)
	if err != nil {
		t.Fatal(err)
	}
	plan.OwnerSearch = &search
	if concern != nil || !validSemanticOwnerSearch(plan) {
		t.Fatalf("branch-shaped owner search concern=%#v evidence=%#v", concern, search)
	}
	if calls.Load() != int32(len(shards)) || peakActive.Load() != expectedPeak {
		t.Fatalf("branch-shaped owner search calls=%d peak=%d, want %d calls with peak %d", calls.Load(), peakActive.Load(), len(shards), expectedPeak)
	}
	t.Logf(
		"branch-shaped corpus projects %d SPECs with a %d-byte owner into %d authenticated shards at peak concurrency %d",
		len(index),
		authorityBranchSyntheticCandidateBytes,
		len(shards),
		peakActive.Load(),
	)
}

func TestAuthoritySizedChangedSPECPromptsFitBound(t *testing.T) {
	repositoryRoot := filepath.Clean(filepath.Join("..", ".."))
	head := strings.TrimSpace(gittest.Run(t, repositoryRoot, "rev-parse", "--verify", "HEAD^{commit}"))
	chdir(t, repositoryRoot)
	corpus, err := loadHeadSpecCorpus(context.Background(), head)
	if err != nil {
		t.Fatal(err)
	}
	content := authoritySizedChangedSPEC(t)
	corpus[semanticCapacityTestOwnerPath] = []byte(content)
	requirements, err := parseRequirements(content)
	if err != nil {
		t.Fatal(err)
	}
	deltas := make([]specRequirementDelta, 0, len(requirements))
	semanticRequirements := make([]semanticOwnerRequirement, 0, len(requirements))
	bodyBytes := 0
	for _, requirement := range requirements {
		bodyBytes += len(requirement.Body)
		deltas = append(deltas, specRequirementDelta{
			Path:   semanticCapacityTestOwnerPath,
			ID:     requirement.ID,
			Status: "added",
			After:  requirement.Body,
		})
		semanticRequirements = append(semanticRequirements, semanticOwnerRequirement(requirement))
	}
	requirementsRaw, err := json.Marshal(semanticRequirements)
	if err != nil {
		t.Fatal(err)
	}
	deltasRaw, err := json.Marshal(deltas)
	if err != nil {
		t.Fatal(err)
	}
	if len(deltas) != 170 || bodyBytes < authorityBranchRequirementBodyBytes || len(requirementsRaw) < authorityBranchRequirementsJSONBytes || len(deltasRaw) < authorityBranchAddedDeltasJSONBytes {
		t.Fatalf(
			"large changed-SPEC shape deltas=%d bodies=%d requirements_json=%d deltas_json=%d",
			len(deltas),
			bodyBytes,
			len(requirementsRaw),
			len(deltasRaw),
		)
	}
	contracts := []changedSpecContract{{
		Path:    semanticCapacityTestOwnerPath,
		Status:  "added",
		Content: content,
		FeaturePaths: []string{
			"agm/test/bdd/features/build_authority_guardrails.feature",
			"agm/test/bdd/features/internal_foundation_guardrails.feature",
		},
		Features:           []bddFeatureEvidence{},
		RequirementChanges: deltas,
	}}
	changes := []specChange{{Path: semanticCapacityTestOwnerPath, Status: "added"}}
	index, reasons, err := semanticOwnerIndex(contracts, changes, corpus, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if len(reasons) != 0 || len(index) != len(corpus)-1 {
		t.Fatalf("large changed-SPEC owner index=%d corpus=%d reasons=%v", len(index), len(corpus), reasons)
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
	digest, shards, reasons, err := buildSemanticOwnerShards(plan)
	if err != nil {
		t.Fatal(err)
	}
	if digest == "" || len(reasons) != 0 || len(shards) == 0 || len(shards) >= maxSemanticShards {
		t.Fatalf("large changed-SPEC shards=%d digest=%q reasons=%v", len(shards), digest, reasons)
	}
	plan.OwnerIndexDigest = digest
	plan.OwnerShards = shards
	plan.OwnerIndexComplete = true
	if !validSemanticOwnerIndex(plan) {
		t.Fatal("large changed-SPEC owner index failed independent authentication")
	}
	maximumPromptBytes := 0
	for _, shard := range shards {
		system, prompt, err := semanticOwnerShardPrompts(plan, shard)
		if err != nil {
			t.Fatalf("large changed-SPEC prompt for shard %d: %v", shard.Ordinal, err)
		}
		promptBytes := len(system) + len(prompt)
		maximumPromptBytes = max(maximumPromptBytes, promptBytes)
		if promptBytes > maxSpecPromptBytes {
			t.Fatalf("large changed-SPEC prompt for shard %d is %d bytes, exceeds %d-byte bound", shard.Ordinal, promptBytes, maxSpecPromptBytes)
		}
	}
	if maximumPromptBytes < 500*1024 {
		t.Fatalf("large changed-SPEC fixture exercises only %d prompt bytes, want at least 500 KiB", maximumPromptBytes)
	}
	t.Logf("authority-sized changed SPEC produces %d shards and a maximum %d-byte authenticated prompt", len(shards), maximumPromptBytes)
}

func TestLargeSemanticOwnerPromptAuthenticatesWithinBound(t *testing.T) {
	plan := reviewableSemanticPlan(t, 0)
	content := sizedSemanticOwnerContentForCorpus(
		t,
		plan.Contracts,
		plan.Changes,
		map[string][]byte{},
		authorityBranchSyntheticCandidateBytes,
	)
	index, reasons, err := semanticOwnerIndex(
		plan.Contracts,
		plan.Changes,
		map[string][]byte{semanticCapacityTestOwnerPath: []byte(content)},
		map[string]bool{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(reasons) != 0 || len(index) != 1 {
		t.Fatalf("large prompt owner index=%d reasons=%v", len(index), reasons)
	}
	plan.OwnerIndex = index
	digest, shards, reasons, err := buildSemanticOwnerShards(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(reasons) != 0 || len(shards) != 1 {
		t.Fatalf("large prompt shards=%d reasons=%v", len(shards), reasons)
	}
	plan.OwnerIndexDigest = digest
	plan.OwnerShards = shards
	plan.OwnerIndexComplete = true
	if !validSemanticOwnerIndex(plan) {
		t.Fatal("large semantic owner index failed independent authentication")
	}
	system, prompt, err := semanticOwnerShardPrompts(plan, shards[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(system)+len(prompt) > maxSpecPromptBytes {
		t.Fatalf(
			"large semantic owner prompt = %d bytes, exceeds %d-byte bound",
			len(system)+len(prompt),
			maxSpecPromptBytes,
		)
	}

	tampered := plan
	tampered.OwnerIndex = append([]semanticOwnerCandidate(nil), plan.OwnerIndex...)
	tampered.OwnerIndex[0].VisibleContract += "x"
	if validSemanticOwnerIndex(tampered) {
		t.Fatal("large semantic owner index authenticated after visible-contract tampering")
	}
}

func authoritySizedChangedSPEC(t *testing.T) string {
	t.Helper()
	var content strings.Builder
	content.WriteString("# Build authority capacity contract\n\n")
	for ordinal := range 170 {
		fmt.Fprintf(
			&content,
			"**BUILD-AUTH-%03d** When authenticated source evidence is reviewed, the system shall retain %sfor boundary %03d.\n\n",
			ordinal,
			strings.Repeat("complete bounded evidence ", 5),
			ordinal,
		)
	}
	content.WriteString("## Bounded non-requirement evidence\n\n")
	if content.Len() > authorityBranchRawSPECBytes {
		t.Fatalf("authority-sized fixture base is %d bytes, exceeds %d", content.Len(), authorityBranchRawSPECBytes)
	}
	content.WriteString(strings.Repeat("x", authorityBranchRawSPECBytes-content.Len()))
	result := content.String()
	if len(result) != authorityBranchRawSPECBytes {
		t.Fatalf("authority-sized changed SPEC = %d bytes, want %d", len(result), authorityBranchRawSPECBytes)
	}
	requirements, err := parseRequirements(result)
	if err != nil {
		t.Fatal(err)
	}
	if len(requirements) != 170 {
		t.Fatalf("authority-sized changed SPEC has %d requirements, want 170", len(requirements))
	}
	return result
}

func semanticCapacityTestChange() ([]changedSpecContract, []specChange) {
	const changedPath = "synthetic/changed/SPEC.md"
	changed := specWithoutTrace(
		"CHANGED-01",
		"When a session is archived, the system shall retain its terminal result.",
	)
	return []changedSpecContract{{Path: changedPath, Status: "modified", Content: changed}},
		[]specChange{{Path: changedPath, Status: "modified"}}
}

func sizedSemanticOwnerContentForCorpus(
	t *testing.T,
	contracts []changedSpecContract,
	changes []specChange,
	corpus map[string][]byte,
	targetBytes int,
) string {
	t.Helper()
	base := specWithoutTrace(
		"CAPACITY-01",
		"When semantic capacity is reviewed, the system shall retain complete ownership evidence.",
	)
	project := func(content string) []semanticOwnerCandidate {
		t.Helper()
		candidateCorpus := make(map[string][]byte, len(corpus)+1)
		maps.Copy(candidateCorpus, corpus)
		candidateCorpus[semanticCapacityTestOwnerPath] = []byte(content)
		index, reasons, err := semanticOwnerIndex(contracts, changes, candidateCorpus, map[string]bool{})
		if err != nil {
			t.Fatal(err)
		}
		if len(reasons) != 0 {
			t.Fatalf("size capacity candidate: %v", reasons)
		}
		return index
	}
	baseIndex := project(base)
	baseBytes := semanticOwnerCandidateSize(t, baseIndex, semanticCapacityTestOwnerPath)
	padding := targetBytes - baseBytes
	if padding < 1 {
		t.Fatalf("candidate target %d cannot exceed base projection %d", targetBytes, baseBytes)
	}
	content := base + "\n" + strings.Repeat("x", padding-1)
	index := project(content)
	if size := semanticOwnerCandidateSize(t, index, semanticCapacityTestOwnerPath); size != targetBytes {
		t.Fatalf("sized candidate projection = %d, want %d", size, targetBytes)
	}
	return content
}

func semanticOwnerCandidateSize(t *testing.T, index []semanticOwnerCandidate, candidatePath string) int {
	t.Helper()
	for _, candidate := range index {
		if candidate.Path != candidatePath {
			continue
		}
		raw, err := json.Marshal(candidate)
		if err != nil {
			t.Fatal(err)
		}
		return len(raw)
	}
	t.Fatalf("semantic owner index omitted %q", candidatePath)
	return 0
}
