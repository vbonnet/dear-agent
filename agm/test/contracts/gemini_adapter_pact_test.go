//go:build contract
// +build contract

package contracts

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/assert"
)

// TestGeminiAdapterPact tests the contract between AGM Gemini client and Gemini API
//
// This Pact test defines the consumer-provider contract for:
//   - Consumer: agm-gemini-client (AGM's Gemini adapter)
//   - Provider: gemini-api (Google's Gemini API)
//
// Test interactions:
//  1. Create session - POST /v1beta/models/gemini-2.0-flash-exp:generateContent
//  2. Send message - POST /v1beta/models/gemini-2.0-flash-exp:generateContent
//  3. Stream response - POST /v1beta/models/gemini-2.0-flash-exp:streamGenerateContent
func TestGeminiAdapterPact(t *testing.T) {
	// Create Pact consumer
	pact, err := consumer.NewV2Pact(consumer.MockHTTPProviderConfig{
		Consumer: "agm-gemini-client",
		Provider: "gemini-api",
		PactDir:  "./pacts",
	})
	assert.NoError(t, err)

	t.Run("create session interaction", func(t *testing.T) {
		// Define expected interaction for session creation
		err := pact.
			AddInteraction().
			Given("Gemini API is available").
			UponReceiving("a request to create a new session").
			WithRequest("POST", "/v1beta/models/gemini-2.0-flash-exp:generateContent", func(b *consumer.V2RequestBuilder) {
				b.Query("key", matchers.String("test-key"))
				b.Header("Content-Type", matchers.String("application/json"))
				b.JSONBody(map[string]interface{}{
					"contents": matchers.EachLike(map[string]interface{}{
						"role": matchers.String("user"),
						"parts": matchers.EachLike(map[string]interface{}{
							"text": matchers.String("Initialize session"),
						}, 1),
					}, 1),
					"generationConfig": map[string]interface{}{
						"maxOutputTokens": matchers.Integer(1024),
						"temperature":     matchers.Decimal(0.7),
					},
				})
			}).
			WillRespondWith(http.StatusOK, func(b *consumer.V2ResponseBuilder) {
				b.Header("Content-Type", matchers.String("application/json"))
				b.JSONBody(map[string]interface{}{
					"candidates": matchers.EachLike(map[string]interface{}{
						"content": map[string]interface{}{
							"role": matchers.String("model"),
							"parts": matchers.EachLike(map[string]interface{}{
								"text": matchers.String("Session initialized successfully"),
							}, 1),
						},
						"finishReason": matchers.String("STOP"),
					}, 1),
					"usageMetadata": map[string]interface{}{
						"promptTokenCount":     matchers.Integer(10),
						"candidatesTokenCount": matchers.Integer(5),
						"totalTokenCount":      matchers.Integer(15),
					},
				})
			}).
			ExecuteTest(t, func(config consumer.MockServerConfig) error {
				// Test client code would go here
				baseURL := fmt.Sprintf("http://%s:%d", config.Host, config.Port)
				assert.NotEmpty(t, baseURL)
				return nil
			})

		assert.NoError(t, err)
	})

	t.Run("send message interaction", func(t *testing.T) {
		// Define send message interaction
		err := pact.
			AddInteraction().
			Given("an active Gemini session exists").
			UponReceiving("a request to send a message").
			WithRequest("POST", "/v1beta/models/gemini-2.0-flash-exp:generateContent", func(b *consumer.V2RequestBuilder) {
				b.Query("key", matchers.String("test-key"))
				b.Header("Content-Type", matchers.String("application/json"))
				b.JSONBody(map[string]interface{}{
					"contents": matchers.EachLike(map[string]interface{}{
						"role": matchers.String("user"),
						"parts": matchers.EachLike(map[string]interface{}{
							"text": matchers.String("What is 2+2?"),
						}, 1),
					}, 1),
					"generationConfig": map[string]interface{}{
						"maxOutputTokens": matchers.Integer(1024),
						"temperature":     matchers.Decimal(0.7),
					},
				})
			}).
			WillRespondWith(http.StatusOK, func(b *consumer.V2ResponseBuilder) {
				b.Header("Content-Type", matchers.String("application/json"))
				b.JSONBody(map[string]interface{}{
					"candidates": matchers.EachLike(map[string]interface{}{
						"content": map[string]interface{}{
							"role": matchers.String("model"),
							"parts": matchers.EachLike(map[string]interface{}{
								"text": matchers.String("2+2 equals 4"),
							}, 1),
						},
						"finishReason": matchers.String("STOP"),
					}, 1),
					"usageMetadata": map[string]interface{}{
						"promptTokenCount":     matchers.Integer(8),
						"candidatesTokenCount": matchers.Integer(6),
						"totalTokenCount":      matchers.Integer(14),
					},
				})
			}).
			ExecuteTest(t, func(config consumer.MockServerConfig) error {
				baseURL := fmt.Sprintf("http://%s:%d", config.Host, config.Port)
				assert.NotEmpty(t, baseURL)
				return nil
			})

		assert.NoError(t, err)
	})

	t.Run("stream response interaction", func(t *testing.T) {
		// Define streaming interaction
		err := pact.
			AddInteraction().
			Given("Gemini API supports streaming").
			UponReceiving("a request to stream a response").
			WithRequest("POST", "/v1beta/models/gemini-2.0-flash-exp:streamGenerateContent", func(b *consumer.V2RequestBuilder) {
				b.Query("key", matchers.String("test-key"))
				b.Query("alt", matchers.String("sse"))
				b.Header("Content-Type", matchers.String("application/json"))
				b.JSONBody(map[string]interface{}{
					"contents": matchers.EachLike(map[string]interface{}{
						"role": matchers.String("user"),
						"parts": matchers.EachLike(map[string]interface{}{
							"text": matchers.String("Tell me a short story"),
						}, 1),
					}, 1),
					"generationConfig": map[string]interface{}{
						"maxOutputTokens": matchers.Integer(2048),
						"temperature":     matchers.Decimal(0.9),
					},
				})
			}).
			WillRespondWith(http.StatusOK, func(b *consumer.V2ResponseBuilder) {
				b.Header("Content-Type", matchers.String("text/event-stream"))
				b.Header("Cache-Control", matchers.String("no-cache"))
				b.Body("text/event-stream", []byte("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Once upon a time\"}]}}]}\n\n"))
			}).
			ExecuteTest(t, func(config consumer.MockServerConfig) error {
				baseURL := fmt.Sprintf("http://%s:%d", config.Host, config.Port)
				assert.NotEmpty(t, baseURL)
				return nil
			})

		assert.NoError(t, err)
	})
}
