package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

func TestSynthesizeTreatsSpecContractReportAsAuthoritative(t *testing.T) {
	var requestBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		requestBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, modelResponse("end_turn", "needs-work\nThe authoritative contract report is blocking."))
	}))
	defer server.Close()

	client := anthropic.NewClient(option.WithAPIKey("test-key"), option.WithBaseURL(server.URL))
	outcome, _, err := synthesize(
		context.Background(),
		client,
		anthropic.ModelClaudeOpus4_8,
		anthropic.OutputConfigEffortHigh,
		[]dimensionReport{{key: "spec-contract", text: "Authoritative outcome: needs-work\nThe shared owner is ambiguous."}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if outcome != NeedsWork {
		t.Fatalf("synthesize() = %s, want needs-work", outcome)
	}
	for _, expected := range []string{
		"may include an authoritative SPEC-CONTRACT report",
		"its declared authoritative outcome is binding",
		"Authoritative outcome: needs-work",
	} {
		if !strings.Contains(requestBody, expected) {
			t.Fatalf("synthesis request lacks %q: %s", expected, requestBody)
		}
	}
	if strings.Contains(requestBody, "synthesizing five independent code review dimensions") {
		t.Fatalf("synthesis request still claims only five dimensions: %s", requestBody)
	}
}
