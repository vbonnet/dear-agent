//go:build integration

package tests

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/gemini"
)

func TestGeminiAPIIntegration(t *testing.T) {
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		t.Skip("GOOGLE_API_KEY not set, skipping integration test")
	}

	ctx := context.Background()

	// Create client
	client, err := gemini.NewClient(apiKey)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create session
	session, err := client.CreateSession(ctx, "gemini-pro")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	if session.Model != "gemini-pro" {
		t.Errorf("Expected model gemini-pro, got %s", session.Model)
	}

	// Send message
	response, err := client.SendMessage(ctx, session, "Say hello in one word")
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	if response == "" {
		t.Error("Expected non-empty response from Gemini API")
	}

	t.Logf("Gemini response: %s", response)
}

func TestGeminiAPIWithMultipleMessages(t *testing.T) {
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		t.Skip("GOOGLE_API_KEY not set, skipping integration test")
	}

	ctx := context.Background()

	// Create client
	client, err := gemini.NewClient(apiKey)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Create session
	session, err := client.CreateSession(ctx, "gemini-pro")
	if err != nil {
		t.Fatalf("Failed to create session: %v", err)
	}

	// Send multiple messages (V1: no history context between calls)
	messages := []string{
		"What is 2 + 2?",
		"What is the capital of France?",
		"Name a primary color",
	}

	for _, msg := range messages {
		response, err := client.SendMessage(ctx, session, msg)
		if err != nil {
			t.Fatalf("Failed to send message %q: %v", msg, err)
		}

		if response == "" {
			t.Errorf("Empty response for message: %q", msg)
		}

		t.Logf("Q: %s | A: %s", msg, response)
	}
}

func TestGeminiAPIErrorHandling(t *testing.T) {
	t.Run("invalid API key", func(t *testing.T) {
		client, err := gemini.NewClient("invalid-key-12345")
		if err != nil {
			t.Fatalf("NewClient should not fail with invalid key: %v", err)
		}
		defer client.Close()

		ctx := context.Background()

		// This should fail when trying to connect
		_, err = client.CreateSession(ctx, "gemini-pro")
		if err == nil {
			t.Error("Expected error with invalid API key, got nil")
		}

		var apiErr *gemini.APIError
		if !errors.As(err, &apiErr) {
			t.Errorf("Expected APIError, got %T", err)
		}
	})

	t.Run("empty message", func(t *testing.T) {
		apiKey := os.Getenv("GOOGLE_API_KEY")
		if apiKey == "" {
			t.Skip("GOOGLE_API_KEY not set")
		}

		client, err := gemini.NewClient(apiKey)
		if err != nil {
			t.Fatalf("Failed to create client: %v", err)
		}
		defer client.Close()

		ctx := context.Background()
		session, err := client.CreateSession(ctx, "gemini-pro")
		if err != nil {
			t.Fatalf("Failed to create session: %v", err)
		}

		// Send empty message
		response, err := client.SendMessage(ctx, session, "")
		if err == nil && response == "" {
			// SDK might handle empty message gracefully or return error
			// Either is acceptable for this test
			t.Log("Empty message handled gracefully")
		}
	})
}
