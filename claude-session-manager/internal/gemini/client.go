package gemini

import (
	"context"

	"github.com/google/generative-ai-go/genai"
	"github.com/google/uuid"
	"google.golang.org/api/option"
)

// GeminiClient provides interface to Gemini API
type GeminiClient interface {
	// CreateSession creates a new chat session with specified model
	CreateSession(ctx context.Context, model string) (Session, error)

	// SendMessage sends user message to session, returns assistant response
	SendMessage(ctx context.Context, session Session, message string) (string, error)

	// Close releases client resources
	Close() error
}

// Session represents a Gemini chat session
type Session struct {
	ID      string
	Model   string
	History []*Message
}

// Message represents a chat message
type Message struct {
	Role    string // "user" or "model"
	Content string
}

// NewClient creates a new Gemini client with API key
func NewClient(apiKey string) (GeminiClient, error) {
	if apiKey == "" {
		return nil, &UserError{
			Message: "GOOGLE_API_KEY environment variable not set",
			Usage:   "Get API key at: https://makersuite.google.com/app/apikey",
		}
	}

	return &RealClient{apiKey: apiKey}, nil
}

// RealClient implements GeminiClient using Google Generative AI SDK
type RealClient struct {
	apiKey string
	client *genai.Client // Lazy initialized
}

func (c *RealClient) CreateSession(ctx context.Context, model string) (Session, error) {
	// Lazy initialize SDK client
	if c.client == nil {
		client, err := genai.NewClient(ctx, option.WithAPIKey(c.apiKey))
		if err != nil {
			return Session{}, &APIError{
				Message: "Failed to connect to Gemini API. Check your network connection.",
				Cause:   err,
			}
		}
		c.client = client
	}

	// Create session
	session := Session{
		ID:      uuid.New().String(),
		Model:   model,
		History: []*Message{},
	}

	return session, nil
}

func (c *RealClient) SendMessage(ctx context.Context, session Session, message string) (string, error) {
	if c.client == nil {
		return "", &APIError{Message: "Client not initialized"}
	}

	// Get model
	model := c.client.GenerativeModel(session.Model)

	// Start chat (V1: no history, fresh each time)
	chat := model.StartChat()

	// Send message
	resp, err := chat.SendMessage(ctx, genai.Text(message))
	if err != nil {
		return "", &APIError{
			Message: "Failed to send message to Gemini API",
			Cause:   err,
		}
	}

	// Extract response text
	if len(resp.Candidates) == 0 {
		return "", &APIError{Message: "No response from Gemini API"}
	}

	content := resp.Candidates[0].Content
	if len(content.Parts) == 0 {
		return "", &APIError{Message: "Empty response from Gemini API"}
	}

	// Type assertion to extract text
	textPart, ok := content.Parts[0].(genai.Text)
	if !ok {
		return "", &APIError{Message: "Unexpected response format from Gemini API"}
	}

	return string(textPart), nil
}

func (c *RealClient) Close() error {
	if c.client != nil {
		return c.client.Close()
	}
	return nil
}
