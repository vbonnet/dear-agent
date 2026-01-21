// Package gemini provides Vertex AI Gemini adapter implementation for the agent.Agent interface.
//
// This adapter integrates with Google Cloud Vertex AI Gemini API using the official SDK.
// It implements full session management with in-memory storage and JSONL export/import.
//
// Usage:
//
//	// Set environment variables:
//	// - GCP_PROJECT_ID or GOOGLE_CLOUD_PROJECT (required)
//	// - GCP_LOCATION or VERTEX_AI_LOCATION (optional, default: "us-central1")
//	// - GEMINI_MODEL (optional, default: "gemini-2.0-flash-exp")
//
//	adapter, err := gemini.NewAdapter()
//	if err != nil {
//	    log.Fatal(err) // GCP_PROJECT_ID not set
//	}
//
//	ctx := agent.SessionContext{
//	    Name:             "my-session",
//	    WorkingDirectory: "/home/user/project",
//	}
//	sessionID, err := adapter.CreateSession(ctx)
//	// ... use session
package gemini

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	aiplatform "cloud.google.com/go/aiplatform/apiv1"
	aiplatformpb "cloud.google.com/go/aiplatform/apiv1/aiplatformpb"
	"github.com/google/uuid"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/agent"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/types/known/structpb"
)

// Compile-time interface compliance check
var _ agent.Agent = (*Adapter)(nil)

// Adapter implements the agent.Agent interface for Google Gemini via Vertex AI.
type Adapter struct {
	client    *aiplatform.PredictionClient
	sessions  map[agent.SessionID]*Session
	mu        sync.RWMutex
	projectID string
	location  string
	model     string
}

// NewAdapter creates a new Gemini adapter instance.
//
// Returns error if GCP_PROJECT_ID environment variable is not set.
// Client is lazy-initialized on first API call.
func NewAdapter() (agent.Agent, error) {
	cfg := loadConfig()
	if cfg.ProjectID == "" {
		return nil, ErrProjectIDMissing
	}

	return &Adapter{
		sessions:  make(map[agent.SessionID]*Session),
		projectID: cfg.ProjectID,
		location:  cfg.Location,
		model:     cfg.Model,
	}, nil
}

func init() {
	// Register Gemini adapter (registration errors ignored, will fail at Get time)
	adapter, _ := NewAdapter()
	if adapter != nil {
		agent.Register("gemini", adapter)
	}
}

// Name returns the agent identifier.
func (a *Adapter) Name() string {
	return "gemini"
}

// Version returns the model name.
func (a *Adapter) Version() string {
	return a.model
}

// CreateSession creates a new agent session
func (a *Adapter) CreateSession(ctx agent.SessionContext) (agent.SessionID, error) {
	if ctx.Name == "" {
		return "", fmt.Errorf("session name required")
	}
	if ctx.WorkingDirectory == "" {
		return "", fmt.Errorf("working directory required")
	}

	id := agent.SessionID(uuid.New().String())
	session := &Session{
		ID:        id,
		Context:   ctx,
		Messages:  []agent.Message{},
		Status:    agent.StatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	a.mu.Lock()
	a.sessions[id] = session
	a.mu.Unlock()

	return id, nil
}

// ResumeSession resumes an existing agent session
func (a *Adapter) ResumeSession(sessionID agent.SessionID) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if _, exists := a.sessions[sessionID]; !exists {
		return ErrSessionNotFound
	}

	return nil
}

// TerminateSession terminates an agent session
func (a *Adapter) TerminateSession(sessionID agent.SessionID) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, exists := a.sessions[sessionID]; !exists {
		return ErrSessionNotFound
	}

	delete(a.sessions, sessionID)
	return nil
}

// GetSessionStatus returns the current status of a session
func (a *Adapter) GetSessionStatus(sessionID agent.SessionID) (agent.Status, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if _, exists := a.sessions[sessionID]; exists {
		return agent.StatusActive, nil
	}
	return agent.StatusTerminated, nil
}

// SendMessage sends a message to the agent
func (a *Adapter) SendMessage(sessionID agent.SessionID, message agent.Message) error {
	// 1. Get session
	a.mu.RLock()
	session, exists := a.sessions[sessionID]
	a.mu.RUnlock()

	if !exists {
		return ErrSessionNotFound
	}

	// 2. Validate message
	if message.Content == "" {
		return ErrInvalidMessage
	}

	// 3. Lazy init client
	if a.client == nil {
		if err := a.initClient(); err != nil {
			return err
		}
	}

	// 4. Add user message
	message.ID = uuid.New().String()
	message.Timestamp = time.Now()
	a.mu.Lock()
	session.Messages = append(session.Messages, message)
	a.mu.Unlock()

	// 5. Call Vertex AI API
	resp, err := a.sendWithRetry(session.Messages)
	if err != nil {
		return err
	}

	// 6. Parse and store response
	assistantMsg, err := fromVertexAIResponse(resp)
	if err != nil {
		return err
	}

	a.mu.Lock()
	session.Messages = append(session.Messages, assistantMsg)
	session.UpdatedAt = time.Now()
	a.mu.Unlock()

	return nil
}

// GetHistory retrieves conversation history for a session
func (a *Adapter) GetHistory(sessionID agent.SessionID) ([]agent.Message, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	session, exists := a.sessions[sessionID]
	if !exists {
		return nil, ErrSessionNotFound
	}

	// Return copy to prevent external modification
	history := make([]agent.Message, len(session.Messages))
	copy(history, session.Messages)

	return history, nil
}

// ExportConversation exports conversation in the specified format
func (a *Adapter) ExportConversation(sessionID agent.SessionID, format agent.ConversationFormat) ([]byte, error) {
	history, err := a.GetHistory(sessionID)
	if err != nil {
		return nil, err
	}

	if format != agent.FormatJSONL {
		return nil, ErrFormatNotSupported
	}

	// Export JSONL
	var buf bytes.Buffer
	for _, msg := range history {
		line, err := json.Marshal(msg)
		if err != nil {
			return nil, err
		}
		buf.Write(line)
		buf.WriteByte('\n')
	}
	return buf.Bytes(), nil
}

// ImportConversation imports conversation from serialized data
func (a *Adapter) ImportConversation(data []byte, format agent.ConversationFormat) (agent.SessionID, error) {
	if format != agent.FormatJSONL {
		return "", fmt.Errorf("only JSONL import supported")
	}

	// Parse JSONL
	var messages []agent.Message
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		var msg agent.Message
		if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
			return "", err
		}
		messages = append(messages, msg)
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	// Create new session
	ctx := agent.SessionContext{
		Name:             "imported-session",
		WorkingDirectory: "/tmp",
	}
	sessionID, err := a.CreateSession(ctx)
	if err != nil {
		return "", err
	}

	// Set messages
	a.mu.Lock()
	a.sessions[sessionID].Messages = messages
	a.mu.Unlock()

	return sessionID, nil
}

// Capabilities returns Gemini's feature set.
//
// Even in V1 scaffold, this returns accurate metadata for Gemini 2.0.
func (a *Adapter) Capabilities() agent.Capabilities {
	return agent.Capabilities{
		SupportsSlashCommands: false,   // API adapter, not CLI
		SupportsHooks:         false,   // V2 feature
		SupportsTools:         true,    // Gemini supports function calling
		SupportsVision:        true,    // Gemini 2.0 has vision
		SupportsMultimodal:    false,   // V2 feature (audio/video)
		MaxContextWindow:      1000000, // Gemini 2.0: 1M+ tokens
		ModelName:             a.model,
	}
}

// ExecuteCommand executes a generic command with agent-specific translation
func (a *Adapter) ExecuteCommand(cmd agent.Command) error {
	switch cmd.Type {
	case agent.CommandRename:
		return a.executeRename(cmd)
	case agent.CommandSetDir:
		return a.executeSetDir(cmd)
	case agent.CommandAuthorize:
		// No-op for API agent (no directory restrictions)
		return nil
	case agent.CommandRunHook:
		return fmt.Errorf("hooks not supported for API agents")
	default:
		return fmt.Errorf("unsupported command: %s", cmd.Type)
	}
}

// executeRename handles the rename session command
func (a *Adapter) executeRename(cmd agent.Command) error {
	name, ok := cmd.Params["name"].(string)
	if !ok || name == "" {
		return fmt.Errorf("parameter 'name' must be non-empty string")
	}

	sessionID, ok := cmd.Params["session_id"].(agent.SessionID)
	if !ok {
		return fmt.Errorf("parameter 'session_id' required")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	session, exists := a.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}

	session.Context.Name = name
	return nil
}

// executeSetDir handles the set directory command
func (a *Adapter) executeSetDir(cmd agent.Command) error {
	path, ok := cmd.Params["path"].(string)
	if !ok || path == "" {
		return fmt.Errorf("parameter 'path' must be non-empty string")
	}

	sessionID, ok := cmd.Params["session_id"].(agent.SessionID)
	if !ok {
		return fmt.Errorf("parameter 'session_id' required")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	session, exists := a.sessions[sessionID]
	if !exists {
		return ErrSessionNotFound
	}

	session.Context.WorkingDirectory = path
	return nil
}

// initClient initializes Vertex AI Prediction client (lazy initialization)
func (a *Adapter) initClient() error {
	endpoint := fmt.Sprintf("%s-aiplatform.googleapis.com:443", a.location)
	ctx := context.Background()

	client, err := aiplatform.NewPredictionClient(ctx, option.WithEndpoint(endpoint))
	if err != nil {
		return wrapAuthError(err)
	}

	a.client = client
	return nil
}

// sendWithRetry calls Vertex AI API with exponential backoff retry logic
func (a *Adapter) sendWithRetry(messages []agent.Message) (*aiplatformpb.PredictResponse, error) {
	endpoint := fmt.Sprintf("projects/%s/locations/%s/publishers/google/models/%s",
		a.projectID, a.location, a.model)

	// Build messages value
	messagesValue, err := toVertexAIMessages(messages)
	if err != nil {
		return nil, wrapAPIError("build messages", err)
	}

	// Build instances
	instances := []*structpb.Value{
		structpb.NewStructValue(&structpb.Struct{
			Fields: map[string]*structpb.Value{
				"messages": messagesValue,
			},
		}),
	}

	// Build parameters
	parameters, err := defaultParameters()
	if err != nil {
		return nil, wrapAPIError("build parameters", err)
	}

	// Create request
	req := &aiplatformpb.PredictRequest{
		Endpoint:   endpoint,
		Instances:  instances,
		Parameters: structpb.NewStructValue(parameters),
	}

	// Retry logic
	maxRetries := 5
	baseDelay := 1 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		resp, err := a.client.Predict(ctx, req)
		cancel()

		if err == nil {
			return resp, nil
		}

		// Exponential backoff for transient errors
		if attempt < maxRetries-1 {
			delay := baseDelay * time.Duration(1<<attempt)
			time.Sleep(delay)
			continue
		}

		return nil, wrapAPIError("predict", err)
	}

	return nil, ErrMaxRetriesExceeded
}

// Helper functions for message translation
func toVertexAIMessages(msgs []agent.Message) (*structpb.Value, error) {
	var result []interface{}
	for _, msg := range msgs {
		result = append(result, map[string]interface{}{
			"role":    string(msg.Role),
			"content": msg.Content,
		})
	}
	return structpb.NewValue(result)
}

func fromVertexAIResponse(resp *aiplatformpb.PredictResponse) (agent.Message, error) {
	if len(resp.Predictions) == 0 {
		return agent.Message{}, wrapAPIError("parse response", fmt.Errorf("no predictions"))
	}

	prediction := resp.Predictions[0]
	predMap := prediction.GetStructValue().AsMap()

	content, ok := predMap["content"].([]interface{})
	if !ok || len(content) == 0 {
		return agent.Message{}, wrapAPIError("parse content", fmt.Errorf("no content"))
	}

	contentBlock, ok := content[0].(map[string]interface{})
	if !ok {
		return agent.Message{}, wrapAPIError("parse content block", fmt.Errorf("invalid block"))
	}

	text, ok := contentBlock["text"].(string)
	if !ok {
		return agent.Message{}, wrapAPIError("parse text", fmt.Errorf("no text"))
	}

	return agent.Message{
		ID:        uuid.New().String(),
		Role:      agent.RoleAssistant,
		Content:   text,
		Timestamp: time.Now(),
	}, nil
}

func defaultParameters() (*structpb.Struct, error) {
	return structpb.NewStruct(map[string]interface{}{
		"temperature": 0.7,
		"max_tokens":  2048,
	})
}
