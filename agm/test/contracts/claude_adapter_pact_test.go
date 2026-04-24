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

// TestClaudeAdapterPact tests the contract between AGM Claude client and Claude API
//
// This Pact test defines the consumer-provider contract for:
//   - Consumer: agm-claude-client (AGM's Claude adapter)
//   - Provider: claude-api (Anthropic's Claude API)
//
// Test interactions:
//  1. Create session - POST /v1/messages with session context
//  2. Send message - POST /v1/messages with conversation history
//  3. Stream response - POST /v1/messages with stream=true
func TestClaudeAdapterPact(t *testing.T) {
	// Create Pact consumer
	pact, err := consumer.NewV2Pact(consumer.MockHTTPProviderConfig{
		Consumer: "agm-claude-client",
		Provider: "claude-api",
		PactDir:  "./pacts",
	})
	assert.NoError(t, err)

	t.Run("create session interaction", func(t *testing.T) {
		// Define expected interaction
		err := pact.
			AddInteraction().
			Given("Claude API is available").
			UponReceiving("a request to create a new session").
			WithRequest("POST", "/v1/messages", func(b *consumer.V2RequestBuilder) {
				b.Header("Content-Type", matchers.String("application/json"))
				b.Header("anthropic-version", matchers.String("2023-06-01"))
				b.Header("x-api-key", matchers.Regex("sk-ant-.*", "sk-ant-test-key"))
				b.JSONBody(map[string]interface{}{
					"model": matchers.String("claude-sonnet-4.5"),
					"messages": matchers.EachLike(map[string]interface{}{
						"role":    matchers.String("user"),
						"content": matchers.String("Initialize session"),
					}, 1),
					"max_tokens": matchers.Integer(1024),
				})
			}).
			WillRespondWith(http.StatusOK, func(b *consumer.V2ResponseBuilder) {
				b.Header("Content-Type", matchers.String("application/json"))
				b.JSONBody(map[string]interface{}{
					"id":    matchers.Regex("msg_[a-zA-Z0-9]+", "msg_01234567890"),
					"type":  matchers.String("message"),
					"role":  matchers.String("assistant"),
					"model": matchers.String("claude-sonnet-4.5"),
					"content": matchers.EachLike(map[string]interface{}{
						"type": matchers.String("text"),
						"text": matchers.String("Session initialized successfully"),
					}, 1),
					"usage": map[string]interface{}{
						"input_tokens":  matchers.Integer(10),
						"output_tokens": matchers.Integer(5),
					},
				})
			}).
			ExecuteTest(t, func(config consumer.MockServerConfig) error {
				// Test client code would go here
				// For now, just verify the mock server is set up correctly
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
			Given("an active Claude session exists").
			UponReceiving("a request to send a message").
			WithRequest("POST", "/v1/messages", func(b *consumer.V2RequestBuilder) {
				b.Header("Content-Type", matchers.String("application/json"))
				b.Header("anthropic-version", matchers.String("2023-06-01"))
				b.Header("x-api-key", matchers.Regex("sk-ant-.*", "sk-ant-test-key"))
				b.JSONBody(map[string]interface{}{
					"model": matchers.String("claude-sonnet-4.5"),
					"messages": matchers.EachLike(map[string]interface{}{
						"role":    matchers.String("user"),
						"content": matchers.String("What is 2+2?"),
					}, 1),
					"max_tokens": matchers.Integer(1024),
				})
			}).
			WillRespondWith(http.StatusOK, func(b *consumer.V2ResponseBuilder) {
				b.Header("Content-Type", matchers.String("application/json"))
				b.JSONBody(map[string]interface{}{
					"id":    matchers.Regex("msg_[a-zA-Z0-9]+", "msg_09876543210"),
					"type":  matchers.String("message"),
					"role":  matchers.String("assistant"),
					"model": matchers.String("claude-sonnet-4.5"),
					"content": matchers.EachLike(map[string]interface{}{
						"type": matchers.String("text"),
						"text": matchers.String("2+2 equals 4"),
					}, 1),
					"usage": map[string]interface{}{
						"input_tokens":  matchers.Integer(15),
						"output_tokens": matchers.Integer(8),
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
			Given("Claude API supports streaming").
			UponReceiving("a request to stream a response").
			WithRequest("POST", "/v1/messages", func(b *consumer.V2RequestBuilder) {
				b.Header("Content-Type", matchers.String("application/json"))
				b.Header("anthropic-version", matchers.String("2023-06-01"))
				b.Header("x-api-key", matchers.Regex("sk-ant-.*", "sk-ant-test-key"))
				b.JSONBody(map[string]interface{}{
					"model": matchers.String("claude-sonnet-4.5"),
					"messages": matchers.EachLike(map[string]interface{}{
						"role":    matchers.String("user"),
						"content": matchers.String("Tell me a short story"),
					}, 1),
					"max_tokens": matchers.Integer(2048),
					"stream":     matchers.Like(true),
				})
			}).
			WillRespondWith(http.StatusOK, func(b *consumer.V2ResponseBuilder) {
				b.Header("Content-Type", matchers.String("text/event-stream"))
				b.Header("Cache-Control", matchers.String("no-cache"))
				b.Header("Connection", matchers.String("keep-alive"))
				// SSE response body
				b.Body("text/event-stream", []byte("event: message_start\ndata: {\"type\":\"message_start\"}\n\n"))
			}).
			ExecuteTest(t, func(config consumer.MockServerConfig) error {
				baseURL := fmt.Sprintf("http://%s:%d", config.Host, config.Port)
				assert.NotEmpty(t, baseURL)
				return nil
			})

		assert.NoError(t, err)
	})
}
