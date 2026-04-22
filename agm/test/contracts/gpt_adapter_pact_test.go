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

// TestGPTAdapterPact tests the contract between AGM GPT client and OpenAI API
//
// This Pact test defines the consumer-provider contract for:
//   - Consumer: agm-gpt-client (AGM's GPT adapter)
//   - Provider: openai-api (OpenAI's GPT API)
//
// Test interactions:
//  1. Create session - POST /v1/chat/completions for initial message
//  2. Send message - POST /v1/chat/completions with conversation history
//  3. Stream response - POST /v1/chat/completions with stream=true
func TestGPTAdapterPact(t *testing.T) {
	// Create Pact consumer
	pact, err := consumer.NewV2Pact(consumer.MockHTTPProviderConfig{
		Consumer: "agm-gpt-client",
		Provider: "openai-api",
		PactDir:  "./pacts",
	})
	assert.NoError(t, err)

	t.Run("create session interaction", func(t *testing.T) {
		// Define expected interaction for session creation
		err := pact.
			AddInteraction().
			Given("OpenAI API is available").
			UponReceiving("a request to create a new session").
			WithRequest("POST", "/v1/chat/completions", func(b *consumer.V2RequestBuilder) {
				b.Header("Content-Type", matchers.String("application/json"))
				b.Header("Authorization", matchers.Regex("Bearer sk-.*", "Bearer sk-test-key"))
				b.JSONBody(map[string]interface{}{
					"model": matchers.String("gpt-4-turbo"),
					"messages": matchers.EachLike(map[string]interface{}{
						"role":    matchers.String("user"),
						"content": matchers.String("Initialize session"),
					}, 1),
					"max_tokens":  matchers.Integer(1024),
					"temperature": matchers.Decimal(0.7),
				})
			}).
			WillRespondWith(http.StatusOK, func(b *consumer.V2ResponseBuilder) {
				b.Header("Content-Type", matchers.String("application/json"))
				b.JSONBody(map[string]interface{}{
					"id":      matchers.Regex("chatcmpl-[a-zA-Z0-9]+", "chatcmpl-123456789"),
					"object":  matchers.String("chat.completion"),
					"created": matchers.Integer(1677652288),
					"model":   matchers.String("gpt-4-turbo"),
					"choices": matchers.EachLike(map[string]interface{}{
						"index": matchers.Integer(0),
						"message": map[string]interface{}{
							"role":    matchers.String("assistant"),
							"content": matchers.String("Session initialized successfully"),
						},
						"finish_reason": matchers.String("stop"),
					}, 1),
					"usage": map[string]interface{}{
						"prompt_tokens":     matchers.Integer(10),
						"completion_tokens": matchers.Integer(5),
						"total_tokens":      matchers.Integer(15),
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
			Given("an active GPT session exists").
			UponReceiving("a request to send a message").
			WithRequest("POST", "/v1/chat/completions", func(b *consumer.V2RequestBuilder) {
				b.Header("Content-Type", matchers.String("application/json"))
				b.Header("Authorization", matchers.Regex("Bearer sk-.*", "Bearer sk-test-key"))
				b.JSONBody(map[string]interface{}{
					"model": matchers.String("gpt-4-turbo"),
					"messages": matchers.EachLike(map[string]interface{}{
						"role":    matchers.String("user"),
						"content": matchers.String("What is 2+2?"),
					}, 1),
					"max_tokens":  matchers.Integer(1024),
					"temperature": matchers.Decimal(0.7),
				})
			}).
			WillRespondWith(http.StatusOK, func(b *consumer.V2ResponseBuilder) {
				b.Header("Content-Type", matchers.String("application/json"))
				b.JSONBody(map[string]interface{}{
					"id":      matchers.Regex("chatcmpl-[a-zA-Z0-9]+", "chatcmpl-987654321"),
					"object":  matchers.String("chat.completion"),
					"created": matchers.Integer(1677652290),
					"model":   matchers.String("gpt-4-turbo"),
					"choices": matchers.EachLike(map[string]interface{}{
						"index": matchers.Integer(0),
						"message": map[string]interface{}{
							"role":    matchers.String("assistant"),
							"content": matchers.String("2+2 equals 4"),
						},
						"finish_reason": matchers.String("stop"),
					}, 1),
					"usage": map[string]interface{}{
						"prompt_tokens":     matchers.Integer(15),
						"completion_tokens": matchers.Integer(8),
						"total_tokens":      matchers.Integer(23),
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
			Given("OpenAI API supports streaming").
			UponReceiving("a request to stream a response").
			WithRequest("POST", "/v1/chat/completions", func(b *consumer.V2RequestBuilder) {
				b.Header("Content-Type", matchers.String("application/json"))
				b.Header("Authorization", matchers.Regex("Bearer sk-.*", "Bearer sk-test-key"))
				b.JSONBody(map[string]interface{}{
					"model": matchers.String("gpt-4-turbo"),
					"messages": matchers.EachLike(map[string]interface{}{
						"role":    matchers.String("user"),
						"content": matchers.String("Tell me a short story"),
					}, 1),
					"max_tokens":  matchers.Integer(2048),
					"temperature": matchers.Decimal(0.9),
					"stream":      matchers.Like(true),
				})
			}).
			WillRespondWith(http.StatusOK, func(b *consumer.V2ResponseBuilder) {
				b.Header("Content-Type", matchers.String("text/event-stream"))
				b.Header("Cache-Control", matchers.String("no-cache"))
				b.Header("Connection", matchers.String("keep-alive"))
				b.Body("text/event-stream", []byte("data: {\"id\":\"chatcmpl-stream123\",\"object\":\"chat.completion.chunk\",\"created\":1677652292,\"model\":\"gpt-4-turbo\",\"choices\":[{\"delta\":{\"content\":\"Once upon a time\"},\"index\":0}]}\n\n"))
			}).
			ExecuteTest(t, func(config consumer.MockServerConfig) error {
				baseURL := fmt.Sprintf("http://%s:%d", config.Host, config.Port)
				assert.NotEmpty(t, baseURL)
				return nil
			})

		assert.NoError(t, err)
	})
}
