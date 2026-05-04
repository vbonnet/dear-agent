package provider

import (
	"context"
	"errors"
	"testing"

	openai "github.com/sashabaranov/go-openai"
)

// fakeOpenAIClient is the test seam — it implements openAIChatClient and
// records the request it sees so assertions can inspect message
// construction without making a real network call.
type fakeOpenAIClient struct {
	resp openai.ChatCompletionResponse
	err  error
	last openai.ChatCompletionRequest
}

func (f *fakeOpenAIClient) CreateChatCompletion(_ context.Context, req openai.ChatCompletionRequest) (openai.ChatCompletionResponse, error) {
	f.last = req
	if f.err != nil {
		return openai.ChatCompletionResponse{}, f.err
	}
	return f.resp, nil
}

func newTestOpenAIProvider(t *testing.T, fake *fakeOpenAIClient) *OpenAIProvider {
	t.Helper()
	p, err := NewOpenAIProvider(OpenAIConfig{
		Model:  "gpt-4o-mini",
		client: fake,
	})
	if err != nil {
		t.Fatalf("NewOpenAIProvider: %v", err)
	}
	return p
}

func TestNewOpenAIProvider_AuthFailure(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	if _, err := NewOpenAIProvider(OpenAIConfig{}); err == nil {
		t.Fatal("expected error when OPENAI_API_KEY is unset")
	}
}

func TestNewOpenAIProvider_BadKeyFormat(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "not-a-real-key")
	if _, err := NewOpenAIProvider(OpenAIConfig{}); err == nil {
		t.Fatal("expected error when key does not start with sk-")
	}
}

func TestNewOpenAIProvider_DefaultsModel(t *testing.T) {
	p, err := NewOpenAIProvider(OpenAIConfig{client: &fakeOpenAIClient{}})
	if err != nil {
		t.Fatalf("NewOpenAIProvider: %v", err)
	}
	if p.model != "gpt-4o-mini" {
		t.Errorf("default model = %q, want gpt-4o-mini", p.model)
	}
}

func TestOpenAIProvider_Name(t *testing.T) {
	p := newTestOpenAIProvider(t, &fakeOpenAIClient{})
	if got := p.Name(); got != "openai" {
		t.Errorf("Name() = %q, want openai", got)
	}
}

func TestOpenAIProvider_Capabilities(t *testing.T) {
	p := newTestOpenAIProvider(t, &fakeOpenAIClient{})
	caps := p.Capabilities()
	if !caps.SupportsStreaming {
		t.Error("expected streaming support advertised")
	}
	if caps.MaxTokensPerRequest != 128000 {
		t.Errorf("MaxTokensPerRequest = %d, want 128000", caps.MaxTokensPerRequest)
	}
	if len(caps.SupportedModels) == 0 {
		t.Error("expected at least one supported model")
	}
}

func TestOpenAIProvider_Generate_EmptyPrompt(t *testing.T) {
	p := newTestOpenAIProvider(t, &fakeOpenAIClient{})
	if _, err := p.Generate(context.Background(), &GenerateRequest{Prompt: ""}); err == nil {
		t.Fatal("expected error on empty prompt")
	}
}

func TestOpenAIProvider_Generate_NilRequest(t *testing.T) {
	p := newTestOpenAIProvider(t, &fakeOpenAIClient{})
	if _, err := p.Generate(context.Background(), nil); err == nil {
		t.Fatal("expected error on nil request")
	}
}

func TestOpenAIProvider_Generate_HappyPath(t *testing.T) {
	fake := &fakeOpenAIClient{
		resp: openai.ChatCompletionResponse{
			ID: "chatcmpl-abc",
			Choices: []openai.ChatCompletionChoice{
				{
					Message:      openai.ChatCompletionMessage{Role: "assistant", Content: "hello world"},
					FinishReason: "stop",
				},
			},
			Usage: openai.Usage{PromptTokens: 12, CompletionTokens: 7, TotalTokens: 19},
		},
	}
	p := newTestOpenAIProvider(t, fake)

	resp, err := p.Generate(context.Background(), &GenerateRequest{
		Prompt:       "say hello",
		SystemPrompt: "you are terse",
		MaxTokens:    100,
		Temperature:  0.5,
		Model:        "gpt-4o",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Text != "hello world" {
		t.Errorf("Text = %q, want %q", resp.Text, "hello world")
	}
	if resp.Model != "gpt-4o" {
		t.Errorf("Model = %q, want gpt-4o (request override)", resp.Model)
	}
	if resp.Usage.InputTokens != 12 || resp.Usage.OutputTokens != 7 || resp.Usage.TotalTokens != 19 {
		t.Errorf("Usage tokens = %+v", resp.Usage)
	}
	if fake.last.Model != "gpt-4o" {
		t.Errorf("request model = %q, want gpt-4o", fake.last.Model)
	}
	if len(fake.last.Messages) != 2 {
		t.Fatalf("messages = %d, want 2 (system+user)", len(fake.last.Messages))
	}
	if fake.last.Messages[0].Role != openai.ChatMessageRoleSystem {
		t.Errorf("first message role = %q, want system", fake.last.Messages[0].Role)
	}
	if fake.last.Messages[1].Role != openai.ChatMessageRoleUser {
		t.Errorf("second message role = %q, want user", fake.last.Messages[1].Role)
	}
	if fake.last.MaxCompletionTokens != 100 {
		t.Errorf("MaxCompletionTokens = %d, want 100", fake.last.MaxCompletionTokens)
	}
}

func TestOpenAIProvider_Generate_NoSystemPromptOmitsSystemMessage(t *testing.T) {
	fake := &fakeOpenAIClient{
		resp: openai.ChatCompletionResponse{
			Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{Content: "ok"}}},
			Usage:   openai.Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
		},
	}
	p := newTestOpenAIProvider(t, fake)
	if _, err := p.Generate(context.Background(), &GenerateRequest{Prompt: "hi"}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(fake.last.Messages) != 1 {
		t.Errorf("messages = %d, want 1", len(fake.last.Messages))
	}
	if fake.last.Messages[0].Role != openai.ChatMessageRoleUser {
		t.Errorf("only message role = %q, want user", fake.last.Messages[0].Role)
	}
}

func TestOpenAIProvider_Generate_APIError(t *testing.T) {
	want := errors.New("rate limited")
	p := newTestOpenAIProvider(t, &fakeOpenAIClient{err: want})
	_, err := p.Generate(context.Background(), &GenerateRequest{Prompt: "hi"})
	if err == nil {
		t.Fatal("expected error from API failure")
	}
	var perr *ProviderError
	if !errors.As(err, &perr) {
		t.Fatalf("expected ProviderError, got %T (%v)", err, err)
	}
	if !errors.Is(err, want) {
		t.Errorf("expected wrapped %q, got %v", want, err)
	}
}

func TestOpenAIProvider_Generate_EmptyChoicesIsError(t *testing.T) {
	p := newTestOpenAIProvider(t, &fakeOpenAIClient{
		resp: openai.ChatCompletionResponse{Choices: nil},
	})
	if _, err := p.Generate(context.Background(), &GenerateRequest{Prompt: "hi"}); err == nil {
		t.Fatal("expected error on empty choices")
	}
}

func TestLooksLikeOpenAIModel(t *testing.T) {
	cases := map[string]bool{
		"gpt-4o":             true,
		"gpt-4-turbo":        true,
		"gpt-5-pro":          true,
		"o1":                 true,
		"o1-mini":            true,
		"o3-mini":            true,
		"chatgpt-4o-latest":  true,
		"claude-opus-4-7":    false,
		"gemini-1.5-pro":     false,
		"llama3.2":           false,
		"":                   false,
	}
	for in, want := range cases {
		if got := looksLikeOpenAIModel(in); got != want {
			t.Errorf("looksLikeOpenAIModel(%q) = %v, want %v", in, got, want)
		}
	}
}
