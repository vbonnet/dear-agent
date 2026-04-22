package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- Constructor tests ---

func TestNewOllamaProvider(t *testing.T) {
	t.Run("success with defaults", func(t *testing.T) {
		p, err := NewOllamaProvider(OllamaConfig{})
		if err != nil {
			t.Fatalf("NewOllamaProvider() error = %v", err)
		}
		if p == nil {
			t.Fatal("expected non-nil provider")
		}
		if p.Name() != "ollama" {
			t.Errorf("Name() = %q, want %q", p.Name(), "ollama")
		}
	})

	t.Run("uses custom endpoint from config", func(t *testing.T) {
		p, err := NewOllamaProvider(OllamaConfig{Endpoint: "http://10.0.0.1:11434"})
		if err != nil {
			t.Fatalf("NewOllamaProvider() error = %v", err)
		}
		if p.endpoint != "http://10.0.0.1:11434" {
			t.Errorf("endpoint = %q, want %q", p.endpoint, "http://10.0.0.1:11434")
		}
	})

	t.Run("uses OLLAMA_HOST env var when config endpoint is empty", func(t *testing.T) {
		t.Setenv(ollamaEnvEndpoint, "http://remote-host:11434")

		p, err := NewOllamaProvider(OllamaConfig{})
		if err != nil {
			t.Fatalf("NewOllamaProvider() error = %v", err)
		}
		if p.endpoint != "http://remote-host:11434" {
			t.Errorf("endpoint = %q, want %q", p.endpoint, "http://remote-host:11434")
		}
	})

	t.Run("config endpoint takes precedence over OLLAMA_HOST", func(t *testing.T) {
		t.Setenv(ollamaEnvEndpoint, "http://env-host:11434")

		p, err := NewOllamaProvider(OllamaConfig{Endpoint: "http://config-host:11434"})
		if err != nil {
			t.Fatalf("NewOllamaProvider() error = %v", err)
		}
		if p.endpoint != "http://config-host:11434" {
			t.Errorf("endpoint = %q, want %q", p.endpoint, "http://config-host:11434")
		}
	})

	t.Run("falls back to default endpoint", func(t *testing.T) {
		t.Setenv(ollamaEnvEndpoint, "")

		p, err := NewOllamaProvider(OllamaConfig{})
		if err != nil {
			t.Fatalf("NewOllamaProvider() error = %v", err)
		}
		if p.endpoint != defaultOllamaEndpoint {
			t.Errorf("endpoint = %q, want %q", p.endpoint, defaultOllamaEndpoint)
		}
	})

	t.Run("uses custom model from config", func(t *testing.T) {
		p, err := NewOllamaProvider(OllamaConfig{Model: "mistral"})
		if err != nil {
			t.Fatalf("NewOllamaProvider() error = %v", err)
		}
		if p.model != "mistral" {
			t.Errorf("model = %q, want %q", p.model, "mistral")
		}
	})

	t.Run("uses default model when not specified", func(t *testing.T) {
		p, err := NewOllamaProvider(OllamaConfig{})
		if err != nil {
			t.Fatalf("NewOllamaProvider() error = %v", err)
		}
		if p.model != defaultOllamaModel {
			t.Errorf("model = %q, want %q", p.model, defaultOllamaModel)
		}
	})
}

// --- Name ---

func TestOllamaProvider_Name(t *testing.T) {
	p, err := NewOllamaProvider(OllamaConfig{})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if p.Name() != "ollama" {
		t.Errorf("Name() = %q, want %q", p.Name(), "ollama")
	}
}

// --- Capabilities ---

func TestOllamaProvider_Capabilities(t *testing.T) {
	p, err := NewOllamaProvider(OllamaConfig{})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	caps := p.Capabilities()

	t.Run("no prompt caching", func(t *testing.T) {
		if caps.SupportsCaching {
			t.Error("Ollama should not support prompt caching")
		}
	})

	t.Run("supports streaming", func(t *testing.T) {
		if !caps.SupportsStreaming {
			t.Error("Ollama should support streaming")
		}
	})

	t.Run("context window set", func(t *testing.T) {
		if caps.MaxTokensPerRequest <= 0 {
			t.Errorf("MaxTokensPerRequest = %d, want > 0", caps.MaxTokensPerRequest)
		}
	})

	t.Run("single concurrent request", func(t *testing.T) {
		if caps.MaxConcurrentRequests != 1 {
			t.Errorf("MaxConcurrentRequests = %d, want 1", caps.MaxConcurrentRequests)
		}
	})

	t.Run("supported models not empty", func(t *testing.T) {
		if len(caps.SupportedModels) == 0 {
			t.Error("SupportedModels should not be empty")
		}
	})
}

// --- Generate ---

func TestOllamaProvider_Generate(t *testing.T) {
	t.Run("error on empty prompt", func(t *testing.T) {
		p, err := NewOllamaProvider(OllamaConfig{})
		if err != nil {
			t.Fatalf("setup: %v", err)
		}

		_, err = p.Generate(context.Background(), &GenerateRequest{Prompt: ""})
		if err == nil {
			t.Fatal("expected error for empty prompt")
		}

		var provErr *ProviderError
		if !errors.As(err, &provErr) {
			t.Errorf("expected ProviderError, got %T", err)
		}
		if provErr.Provider != "ollama" {
			t.Errorf("ProviderError.Provider = %q, want %q", provErr.Provider, "ollama")
		}
	})

	t.Run("successful generate with system prompt", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/chat" {
				t.Errorf("unexpected path: %s", r.URL.Path)
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			if r.Method != http.MethodPost {
				t.Errorf("unexpected method: %s", r.Method)
			}

			// Decode request and verify fields
			var req ollamaChatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("failed to decode request: %v", err)
			}
			if len(req.Messages) != 2 {
				t.Errorf("expected 2 messages (system+user), got %d", len(req.Messages))
			}
			if req.Messages[0].Role != "system" {
				t.Errorf("first message role = %q, want %q", req.Messages[0].Role, "system")
			}
			if req.Messages[1].Role != "user" {
				t.Errorf("second message role = %q, want %q", req.Messages[1].Role, "user")
			}
			if req.Stream {
				t.Error("Stream should be false for non-streaming generate")
			}

			resp := ollamaChatResponse{
				Model:           "llama3.2",
				Message:         ollamaMessage{Role: "assistant", Content: "Hello, world!"},
				Done:            true,
				DoneReason:      "stop",
				PromptEvalCount: 10,
				EvalCount:       5,
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer srv.Close()

		p, err := NewOllamaProvider(OllamaConfig{Endpoint: srv.URL})
		if err != nil {
			t.Fatalf("setup: %v", err)
		}

		resp, err := p.Generate(context.Background(), &GenerateRequest{
			Prompt:       "Say hello",
			SystemPrompt: "You are a helpful assistant.",
			MaxTokens:    100,
		})
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}

		if resp.Text != "Hello, world!" {
			t.Errorf("Text = %q, want %q", resp.Text, "Hello, world!")
		}
		if resp.Model != "llama3.2" {
			t.Errorf("Model = %q, want %q", resp.Model, "llama3.2")
		}
		if resp.Usage.InputTokens != 10 {
			t.Errorf("Usage.InputTokens = %d, want 10", resp.Usage.InputTokens)
		}
		if resp.Usage.OutputTokens != 5 {
			t.Errorf("Usage.OutputTokens = %d, want 5", resp.Usage.OutputTokens)
		}
		if resp.Usage.TotalTokens != 15 {
			t.Errorf("Usage.TotalTokens = %d, want 15", resp.Usage.TotalTokens)
		}
		if resp.Usage.CostUSD != 0 {
			t.Errorf("Usage.CostUSD = %f, want 0 (local inference is free)", resp.Usage.CostUSD)
		}
	})

	t.Run("successful generate without system prompt", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req ollamaChatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("failed to decode request: %v", err)
			}
			if len(req.Messages) != 1 {
				t.Errorf("expected 1 message (user only), got %d", len(req.Messages))
			}
			if req.Messages[0].Role != "user" {
				t.Errorf("message role = %q, want %q", req.Messages[0].Role, "user")
			}

			resp := ollamaChatResponse{
				Model:           "llama3.2",
				Message:         ollamaMessage{Role: "assistant", Content: "response text"},
				Done:            true,
				DoneReason:      "stop",
				PromptEvalCount: 5,
				EvalCount:       3,
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer srv.Close()

		p, err := NewOllamaProvider(OllamaConfig{Endpoint: srv.URL})
		if err != nil {
			t.Fatalf("setup: %v", err)
		}

		resp, err := p.Generate(context.Background(), &GenerateRequest{
			Prompt: "Hello",
		})
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}
		if resp.Text != "response text" {
			t.Errorf("Text = %q, want %q", resp.Text, "response text")
		}
	})

	t.Run("uses model from request over provider default", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req ollamaChatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("failed to decode request: %v", err)
			}
			if req.Model != "codellama" {
				t.Errorf("request model = %q, want %q", req.Model, "codellama")
			}

			resp := ollamaChatResponse{
				Model:      "codellama",
				Message:    ollamaMessage{Role: "assistant", Content: "code response"},
				Done:       true,
				DoneReason: "stop",
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer srv.Close()

		p, err := NewOllamaProvider(OllamaConfig{Endpoint: srv.URL, Model: "llama3.2"})
		if err != nil {
			t.Fatalf("setup: %v", err)
		}

		resp, err := p.Generate(context.Background(), &GenerateRequest{
			Prompt: "write code",
			Model:  "codellama", // override default
		})
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}
		if resp.Model != "codellama" {
			t.Errorf("Model = %q, want %q", resp.Model, "codellama")
		}
	})

	t.Run("sets generation options when specified", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var req ollamaChatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("failed to decode request: %v", err)
			}
			if req.Options == nil {
				t.Error("expected non-nil options")
			} else {
				if req.Options.NumPredict != 50 {
					t.Errorf("NumPredict = %d, want 50", req.Options.NumPredict)
				}
				if req.Options.Temperature != 0.8 {
					t.Errorf("Temperature = %f, want 0.8", req.Options.Temperature)
				}
			}

			resp := ollamaChatResponse{
				Model:      "llama3.2",
				Message:    ollamaMessage{Role: "assistant", Content: "ok"},
				Done:       true,
				DoneReason: "stop",
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Errorf("failed to encode response: %v", err)
			}
		}))
		defer srv.Close()

		p, err := NewOllamaProvider(OllamaConfig{Endpoint: srv.URL})
		if err != nil {
			t.Fatalf("setup: %v", err)
		}

		_, err = p.Generate(context.Background(), &GenerateRequest{
			Prompt:      "hello",
			MaxTokens:   50,
			Temperature: 0.8,
		})
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}
	})

	t.Run("error on HTTP non-200", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, `{"error":"model not found"}`, http.StatusNotFound)
		}))
		defer srv.Close()

		p, err := NewOllamaProvider(OllamaConfig{Endpoint: srv.URL})
		if err != nil {
			t.Fatalf("setup: %v", err)
		}

		_, err = p.Generate(context.Background(), &GenerateRequest{Prompt: "hello"})
		if err == nil {
			t.Fatal("expected error for HTTP 404")
		}

		var provErr *ProviderError
		if !errors.As(err, &provErr) {
			t.Errorf("expected ProviderError, got %T", err)
		}
	})

	t.Run("error on empty response text", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			resp := ollamaChatResponse{
				Model:      "llama3.2",
				Message:    ollamaMessage{Role: "assistant", Content: ""},
				Done:       true,
				DoneReason: "stop",
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				// can't use t.Errorf here but this is test infrastructure
				http.Error(w, "encode error", http.StatusInternalServerError)
			}
		}))
		defer srv.Close()

		p, err := NewOllamaProvider(OllamaConfig{Endpoint: srv.URL})
		if err != nil {
			t.Fatalf("setup: %v", err)
		}

		_, err = p.Generate(context.Background(), &GenerateRequest{Prompt: "hello"})
		if err == nil {
			t.Fatal("expected error for empty response text")
		}

		var provErr *ProviderError
		if !errors.As(err, &provErr) {
			t.Errorf("expected ProviderError, got %T", err)
		}
	})

	t.Run("error on invalid JSON response", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("not valid json"))
		}))
		defer srv.Close()

		p, err := NewOllamaProvider(OllamaConfig{Endpoint: srv.URL})
		if err != nil {
			t.Fatalf("setup: %v", err)
		}

		_, err = p.Generate(context.Background(), &GenerateRequest{Prompt: "hello"})
		if err == nil {
			t.Fatal("expected error for invalid JSON")
		}

		var provErr *ProviderError
		if !errors.As(err, &provErr) {
			t.Errorf("expected ProviderError, got %T", err)
		}
	})

	t.Run("error when server unreachable", func(t *testing.T) {
		p, err := NewOllamaProvider(OllamaConfig{Endpoint: "http://127.0.0.1:19999"})
		if err != nil {
			t.Fatalf("setup: %v", err)
		}

		_, err = p.Generate(context.Background(), &GenerateRequest{Prompt: "hello"})
		if err == nil {
			t.Fatal("expected error for unreachable server")
		}

		var provErr *ProviderError
		if !errors.As(err, &provErr) {
			t.Errorf("expected ProviderError, got %T", err)
		}
	})

	t.Run("metadata contains done and token counts", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			resp := ollamaChatResponse{
				Model:           "llama3.2",
				Message:         ollamaMessage{Role: "assistant", Content: "answer"},
				Done:            true,
				DoneReason:      "stop",
				PromptEvalCount: 20,
				EvalCount:       8,
			}
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				http.Error(w, "encode error", http.StatusInternalServerError)
			}
		}))
		defer srv.Close()

		p, err := NewOllamaProvider(OllamaConfig{Endpoint: srv.URL})
		if err != nil {
			t.Fatalf("setup: %v", err)
		}

		resp, err := p.Generate(context.Background(), &GenerateRequest{Prompt: "question"})
		if err != nil {
			t.Fatalf("Generate() error = %v", err)
		}

		if resp.Metadata["done"] != true {
			t.Errorf("Metadata[done] = %v, want true", resp.Metadata["done"])
		}
		if resp.Metadata["done_reason"] != "stop" {
			t.Errorf("Metadata[done_reason] = %v, want %q", resp.Metadata["done_reason"], "stop")
		}
		if resp.Metadata["input_tokens"] != 20 {
			t.Errorf("Metadata[input_tokens] = %v, want 20", resp.Metadata["input_tokens"])
		}
		if resp.Metadata["output_tokens"] != 8 {
			t.Errorf("Metadata[output_tokens] = %v, want 8", resp.Metadata["output_tokens"])
		}
	})
}

// --- Factory integration ---

func TestFactory_NewProvider_Ollama(t *testing.T) {
	t.Run("ollama family creates OllamaProvider", func(t *testing.T) {
		f := NewFactory()

		p, err := f.NewProvider("ollama", "")
		if err != nil {
			t.Fatalf("NewProvider(ollama) error = %v", err)
		}
		if p.Name() != "ollama" {
			t.Errorf("Name() = %q, want %q", p.Name(), "ollama")
		}
	})

	t.Run("local family creates OllamaProvider", func(t *testing.T) {
		f := NewFactory()

		p, err := f.NewProvider("local", "")
		if err != nil {
			t.Fatalf("NewProvider(local) error = %v", err)
		}
		if p.Name() != "ollama" {
			t.Errorf("Name() = %q, want %q", p.Name(), "ollama")
		}
	})

	t.Run("ollama with custom model", func(t *testing.T) {
		f := NewFactory()

		p, err := f.NewProvider("ollama", "mistral")
		if err != nil {
			t.Fatalf("NewProvider(ollama, mistral) error = %v", err)
		}

		op, ok := p.(*OllamaProvider)
		if !ok {
			t.Fatalf("expected *OllamaProvider, got %T", p)
		}
		if op.model != "mistral" {
			t.Errorf("model = %q, want %q", op.model, "mistral")
		}
	})
}

// --- calculateUsage ---

func TestOllamaProvider_calculateUsage(t *testing.T) {
	p, err := NewOllamaProvider(OllamaConfig{})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	resp := ollamaChatResponse{
		PromptEvalCount: 30,
		EvalCount:       12,
	}

	usage := p.calculateUsage(resp)

	if usage.InputTokens != 30 {
		t.Errorf("InputTokens = %d, want 30", usage.InputTokens)
	}
	if usage.OutputTokens != 12 {
		t.Errorf("OutputTokens = %d, want 12", usage.OutputTokens)
	}
	if usage.TotalTokens != 42 {
		t.Errorf("TotalTokens = %d, want 42", usage.TotalTokens)
	}
	if usage.CostUSD != 0 {
		t.Errorf("CostUSD = %f, want 0", usage.CostUSD)
	}
}
