package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

func TestReviewDeadlineBudgetsFitWorkflowTimeout(t *testing.T) {
	ownerWaves := (maxSemanticShards + maxConcurrentSemanticCalls - 1) / maxConcurrentSemanticCalls
	if reviewWorkflowTimeout != 60*time.Minute || reviewPipelineTimeout >= reviewWorkflowTimeout {
		t.Fatalf("workflow/pipeline deadlines = %s/%s, want a bounded pipeline below the 60-minute workflow", reviewWorkflowTimeout, reviewPipelineTimeout)
	}
	if reviewWorkflowTimeout-reviewPipelineTimeout < 5*time.Minute {
		t.Fatalf("workflow reserve = %s, want at least 5 minutes", reviewWorkflowTimeout-reviewPipelineTimeout)
	}
	if semanticOwnerSearchStageTimeout < time.Duration(ownerWaves)*reviewRequestTimeout || finalSpecReviewStageTimeout < reviewRequestTimeout || dimensionReviewStageTimeout < reviewRequestTimeout || synthesisStageTimeout < reviewRequestTimeout {
		t.Fatalf("stage budget cannot complete bounded request waves: owner=%s final=%s dimensions=%s synthesis=%s request=%s waves=%d", semanticOwnerSearchStageTimeout, finalSpecReviewStageTimeout, dimensionReviewStageTimeout, synthesisStageTimeout, reviewRequestTimeout, ownerWaves)
	}
	sequentialModelBudget := semanticOwnerSearchStageTimeout + finalSpecReviewStageTimeout + dimensionReviewStageTimeout + synthesisStageTimeout
	if sequentialModelBudget > reviewPipelineTimeout-time.Minute {
		t.Fatalf("sequential model budget = %s, want at least one minute inside pipeline %s", sequentialModelBudget, reviewPipelineTimeout)
	}
}

func TestCallClaudeUsesRequestedOutputBudget(t *testing.T) {
	var budgets []int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			MaxTokens int64 `json:"max_tokens"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		budgets = append(budgets, request.MaxTokens)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, modelResponse("end_turn", "ok"))
	}))
	defer server.Close()
	client := anthropic.NewClient(option.WithAPIKey("test-key"), option.WithBaseURL(server.URL))

	for _, budget := range []int64{defaultReviewMaxTokens, specReviewMaxTokens} {
		if _, err := callClaude(context.Background(), client, anthropic.ModelClaudeOpus4_8, anthropic.OutputConfigEffortHigh, budget, "system", "user"); err != nil {
			t.Fatalf("callClaude(%d): %v", budget, err)
		}
	}
	if len(budgets) != 2 || budgets[0] != defaultReviewMaxTokens || budgets[1] != specReviewMaxTokens {
		t.Fatalf("captured max_tokens = %v, want [%d %d]", budgets, defaultReviewMaxTokens, specReviewMaxTokens)
	}
}

func TestCallClaudeRejectsIncompleteStopReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, modelResponse("max_tokens", `{"status":"approved"}`))
	}))
	defer server.Close()
	client := anthropic.NewClient(option.WithAPIKey("test-key"), option.WithBaseURL(server.URL))

	_, err := callClaude(context.Background(), client, anthropic.ModelClaudeOpus4_8, anthropic.OutputConfigEffortHigh, specReviewMaxTokens, "system", "user")
	if err == nil || !strings.Contains(err.Error(), "stopped before end_turn") {
		t.Fatalf("callClaude max_tokens error = %v", err)
	}
}

func modelResponse(stopReason, text string) string {
	raw, _ := json.Marshal(map[string]any{
		"id":            "msg_test",
		"type":          "message",
		"role":          "assistant",
		"model":         "claude-opus-4-8",
		"content":       []map[string]string{{"type": "text", "text": text}},
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"usage":         map[string]int{"input_tokens": 1, "output_tokens": 1},
	})
	return string(raw)
}
