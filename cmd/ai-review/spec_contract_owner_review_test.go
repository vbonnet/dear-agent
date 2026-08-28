package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

func TestSemanticOwnerIndexFailsClosedAtCandidateAndShardBounds(t *testing.T) {
	changed := specWithoutTrace("OWN-01", "When a session ends, the system shall keep its final result.")
	contracts := []changedSpecContract{{Path: "domains/session/SPEC.md", Content: changed}}
	changes := []specChange{{Path: "domains/session/SPEC.md", Status: "modified"}}
	corpus := map[string][]byte{"domains/session/SPEC.md": []byte(changed)}
	for index := range maxSemanticCandidates + 1 {
		path := fmt.Sprintf("domains/candidate-%04d/SPEC.md", index)
		corpus[path] = []byte(specWithoutTrace(fmt.Sprintf("CAND-%04d", index), "When storage fills, the system shall reject a new upload."))
	}
	index, reasons, err := semanticOwnerIndex(contracts, changes, corpus, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if len(index) != 0 || len(reasons) != 1 || !strings.Contains(reasons[0], fmt.Sprintf("%d-candidate", maxSemanticCandidates)) {
		t.Fatalf("candidate overflow index=%d reasons=%v", len(index), reasons)
	}

	largeIndex := make([]semanticOwnerCandidate, 0, 65)
	for ordinal := range 65 {
		largeIndex = append(largeIndex, semanticOwnerCandidate{
			Ordinal:         ordinal,
			Path:            fmt.Sprintf("domains/large-%02d/SPEC.md", ordinal),
			RequirementIDs:  []string{fmt.Sprintf("LARGE-%02d", ordinal)},
			VisibleContract: strings.Repeat("x", 30*1024),
			FeaturePaths:    []string{},
			Signals:         []string{},
		})
	}
	plan := reviewPlan{Version: specContractVersion, BaseSHA: strings.Repeat("a", 40), MergeBaseSHA: strings.Repeat("a", 40), HeadSHA: strings.Repeat("b", 40), OwnerIndex: largeIndex}
	_, shards, reasons, err := buildSemanticOwnerShards(plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(shards) != 0 || len(reasons) != 1 || !strings.Contains(reasons[0], fmt.Sprintf("%d-shard", maxSemanticShards)) {
		t.Fatalf("shard overflow shards=%d reasons=%v", len(shards), reasons)
	}
}

func TestParseSemanticOwnerShardVerdictRejectsIncompleteOrUnauthenticatedResults(t *testing.T) {
	plan := reviewableSemanticPlan(t, 3)
	shard := plan.OwnerShards[0]
	worstOrdinalPlan := reviewableSemanticPlan(t, maxSemanticCandidates)
	worstOrdinalShard := worstOrdinalPlan.OwnerShards[len(worstOrdinalPlan.OwnerShards)-1]
	maximumBytes, err := maximumSemanticOwnerVerdictSize(worstOrdinalPlan, worstOrdinalShard)
	if err != nil {
		t.Fatal(err)
	}
	if maximumBytes > maxSemanticVerdictBytes {
		t.Fatalf("maximum-value canonical semantic owner verdict is %d bytes, limit %d", maximumBytes, maxSemanticVerdictBytes)
	}
	valid := semanticOwnerShardVerdict{
		Version:      plan.Version,
		BaseSHA:      plan.BaseSHA,
		MergeBaseSHA: plan.MergeBaseSHA,
		HeadSHA:      plan.HeadSHA,
		IndexDigest:  plan.OwnerIndexDigest,
		ShardOrdinal: shard.Ordinal,
		ShardDigest:  shard.Digest,
		Results: []semanticOwnerClassification{
			{Ordinal: 0, Relation: "distinct", Rationale: "The observable promise is different."},
			{Ordinal: 1, Relation: "possible-owner", Rationale: "The observable promise may overlap."},
			{Ordinal: 2, Relation: "uncertain", Rationale: "The bounded normative evidence is insufficient."},
		},
	}
	validRaw, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := parseSemanticOwnerShardVerdict(validRaw, plan, shard); err != nil || len(got.Results) != 3 {
		t.Fatalf("valid semantic owner verdict = %#v, %v", got, err)
	}

	mutations := []func(*semanticOwnerShardVerdict){
		func(verdict *semanticOwnerShardVerdict) { verdict.Results = verdict.Results[:2] },
		func(verdict *semanticOwnerShardVerdict) {
			verdict.Results[0], verdict.Results[1] = verdict.Results[1], verdict.Results[0]
		},
		func(verdict *semanticOwnerShardVerdict) { verdict.Results[1].Ordinal = verdict.Results[0].Ordinal },
		func(verdict *semanticOwnerShardVerdict) { verdict.Results[1].Relation = "same" },
		func(verdict *semanticOwnerShardVerdict) {
			verdict.Results[1].Rationale = strings.Repeat("x", maxSemanticRationaleBytes+1)
		},
		func(verdict *semanticOwnerShardVerdict) { verdict.ShardDigest = strings.Repeat("0", sha256.Size*2) },
		func(verdict *semanticOwnerShardVerdict) { verdict.IndexDigest = strings.Repeat("0", sha256.Size*2) },
	}
	for index, mutate := range mutations {
		copy := valid
		copy.Results = append([]semanticOwnerClassification(nil), valid.Results...)
		mutate(&copy)
		raw, err := json.Marshal(copy)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parseSemanticOwnerShardVerdict(raw, plan, shard); err == nil {
			t.Fatalf("mutation %d was accepted: %s", index, raw)
		}
	}
	unknown := append(append([]byte(nil), validRaw[:len(validRaw)-1]...), []byte(`,"override":true}`)...)
	if _, err := parseSemanticOwnerShardVerdict(unknown, plan, shard); err == nil {
		t.Fatalf("unknown field was accepted: %s", unknown)
	}
	withNullResults := valid
	withNullResults.Results = nil
	nullRaw, err := json.Marshal(withNullResults)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseSemanticOwnerShardVerdict(nullRaw, plan, shard); err == nil {
		t.Fatalf("null results were accepted: %s", nullRaw)
	}

	emptyVisiblePlan := reviewableSemanticPlan(t, 1)
	emptyVisiblePlan.OwnerIndex[0].VisibleContract = ""
	digest, shards, reasons, err := buildSemanticOwnerShards(emptyVisiblePlan)
	if err != nil || len(reasons) != 0 || len(shards) != 1 {
		t.Fatalf("empty visible test shard=%#v reasons=%v err=%v", shards, reasons, err)
	}
	emptyVisiblePlan.OwnerIndexDigest = digest
	emptyVisiblePlan.OwnerShards = shards
	emptyVisibleVerdict := semanticOwnerShardVerdict{
		Version:      emptyVisiblePlan.Version,
		BaseSHA:      emptyVisiblePlan.BaseSHA,
		MergeBaseSHA: emptyVisiblePlan.MergeBaseSHA,
		HeadSHA:      emptyVisiblePlan.HeadSHA,
		IndexDigest:  digest,
		ShardOrdinal: 0,
		ShardDigest:  shards[0].Digest,
		Results:      []semanticOwnerClassification{{Ordinal: 0, Relation: "distinct", Rationale: "The contract is different."}},
	}
	emptyVisibleRaw, err := json.Marshal(emptyVisibleVerdict)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseSemanticOwnerShardVerdict(emptyVisibleRaw, emptyVisiblePlan, shards[0]); err == nil || !strings.Contains(err.Error(), "empty visible") {
		t.Fatalf("empty visible projection proved a distinct contract: %v", err)
	}
}

func TestReviewSpecContractRunsEveryOwnerShardBeforeFinalReview(t *testing.T) {
	for _, test := range []struct {
		name             string
		uncertainOrdinal int
		wantStatus       Outcome
		wantCalls        int32
	}{
		{name: "all distinct reaches final review", uncertainOrdinal: -1, wantStatus: Approved, wantCalls: 3},
		{name: "uncertain candidate stops before final review", uncertainOrdinal: 128, wantStatus: NeedsHumanReview, wantCalls: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			plan := reviewableSemanticPlan(t, maxSemanticShardCandidates+1)
			if len(plan.OwnerShards) != 2 {
				t.Fatalf("test plan shards = %d, want 2", len(plan.OwnerShards))
			}
			finalApplicability := applicabilityReviewsJSON(t, plan, "supported")
			finalContractForms := contractFormReviewsJSON(t, plan, "complete")
			finalVerdict := fmt.Sprintf(`{"version":%q,"base_sha":%q,"merge_base_sha":%q,"head_sha":%q,"changes":[{"path":"domains/session/SPEC.md","status":"modified"}],"status":"approved","summary":"The shared contract has one owner and complete test evidence.","deletion_reviews":[],"contract_form_reviews":%s,"applicability_reviews":%s,"findings":[]}`, plan.Version, plan.BaseSHA, plan.MergeBaseSHA, plan.HeadSHA, finalContractForms, finalApplicability)
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				var request struct {
					Messages []struct {
						Content []struct {
							Text string `json:"text"`
						} `json:"content"`
					} `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) != 1 || len(request.Messages[0].Content) != 1 {
					t.Errorf("decode model request: messages=%#v err=%v", request.Messages, err)
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				user := request.Messages[0].Content[0].Text
				const marker = "Authenticated bounded ownership evidence:\n"
				if _, encoded, found := strings.Cut(user, marker); found {
					var evidence semanticOwnerShardPromptEvidence
					if err := json.Unmarshal([]byte(encoded), &evidence); err != nil {
						t.Errorf("decode semantic owner prompt: %v", err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					results := make([]semanticOwnerClassification, 0, len(evidence.Shard.Candidates))
					for _, candidate := range evidence.Shard.Candidates {
						relation := "distinct"
						if candidate.Ordinal == test.uncertainOrdinal {
							relation = "uncertain"
						}
						results = append(results, semanticOwnerClassification{Ordinal: candidate.Ordinal, Relation: relation, Rationale: "The bounded normative projection supports this classification."})
					}
					verdict := semanticOwnerShardVerdict{Version: plan.Version, BaseSHA: plan.BaseSHA, MergeBaseSHA: plan.MergeBaseSHA, HeadSHA: plan.HeadSHA, IndexDigest: plan.OwnerIndexDigest, ShardOrdinal: evidence.Shard.Ordinal, ShardDigest: evidence.Shard.Digest, Results: results}
					raw, err := json.Marshal(verdict)
					if err != nil {
						t.Errorf("marshal semantic verdict: %v", err)
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, modelResponse("end_turn", string(raw)))
					return
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, modelResponse("end_turn", finalVerdict))
			}))
			defer server.Close()
			client := anthropic.NewClient(option.WithAPIKey("test-key"), option.WithBaseURL(server.URL))

			verdict, err := reviewSpecContract(context.Background(), client, anthropic.ModelClaudeOpus4_8, anthropic.OutputConfigEffortHigh, plan)
			if err != nil {
				t.Fatal(err)
			}
			if verdict.Status != test.wantStatus || calls.Load() != test.wantCalls {
				t.Fatalf("review verdict=%s calls=%d, want verdict=%s calls=%d", verdict.Status, calls.Load(), test.wantStatus, test.wantCalls)
			}
			if test.uncertainOrdinal >= 0 {
				path := plan.OwnerIndex[test.uncertainOrdinal].Path
				if !strings.Contains(verdict.Summary, path) || len(verdict.Findings) != 1 || verdict.Findings[0].Path != path {
					t.Fatalf("human owner verdict does not identify %q: summary=%q findings=%#v", path, verdict.Summary, verdict.Findings)
				}
			}
		})
	}
}
