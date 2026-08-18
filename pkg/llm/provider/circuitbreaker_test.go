package provider

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCircuitBreakerTreatsNilResponseAsFailure(t *testing.T) {
	breaker := NewCircuitBreaker(&stubProvider{name: "nil-provider"}, CircuitBreakerConfig{FailureThreshold: 2})

	for attempt := 1; attempt <= 2; attempt++ {
		response, err := breaker.Generate(context.Background(), &GenerateRequest{Prompt: "test"})
		if response != nil || err == nil || !strings.Contains(err.Error(), "nil response") {
			t.Fatalf("attempt %d: response=%v error=%v, want nil-response failure", attempt, response, err)
		}
	}
	if breaker.State() != CBOpen {
		t.Fatalf("state = %s, want open after nil-response threshold", breaker.State())
	}
	if _, err := breaker.Generate(context.Background(), &GenerateRequest{Prompt: "test"}); err == nil || !strings.Contains(err.Error(), "circuit breaker open") {
		t.Fatalf("post-threshold error = %v, want open-circuit failure", err)
	}
}

func TestCircuitBreakerRejectsNilFallbackResponses(t *testing.T) {
	breaker := NewCircuitBreaker(
		&stubProvider{name: "primary", err: errors.New("primary failed")},
		CircuitBreakerConfig{
			FailureThreshold: 1,
			FallbackProvider: &stubProvider{name: "nil-fallback"},
		},
	)

	for attempt := 1; attempt <= 2; attempt++ {
		response, err := breaker.Generate(context.Background(), &GenerateRequest{Prompt: "test"})
		if response != nil || err == nil || !strings.Contains(err.Error(), `provider "nil-fallback" returned a nil response`) {
			t.Fatalf("attempt %d: response=%v error=%v, want nil-fallback failure", attempt, response, err)
		}
	}
}

func TestCircuitBreakerReportsFallbackSource(t *testing.T) {
	breaker := NewCircuitBreaker(
		&stubProvider{name: "primary", err: errors.New("primary failed")},
		CircuitBreakerConfig{
			FailureThreshold: 1,
			FallbackProvider: &stubProvider{name: "fallback", resp: &GenerateResponse{
				Text:  "ok",
				Model: "fallback-model",
			}},
			FallbackModel: "fallback-model",
		},
	)

	response, source, err := breaker.GenerateWithSource(context.Background(), &GenerateRequest{
		Prompt: "test",
		Model:  "primary-model",
	})
	if err != nil || response == nil {
		t.Fatalf("GenerateWithSource() = (%v, %v), want fallback response", response, err)
	}
	if source.Provider != "fallback" || source.Model != "fallback-model" || !source.Fallback {
		t.Fatalf("source = %#v, want fallback/fallback-model", source)
	}
}
