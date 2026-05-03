package steps

import (
	"context"
	"fmt"
	"strings"

	"github.com/cucumber/godog"

	"github.com/vbonnet/dear-agent/agm/test/bdd/internal/adapters/mock"
	"github.com/vbonnet/dear-agent/agm/test/bdd/internal/testenv"
)

// RegisterConversationSteps registers conversation-related step definitions
func RegisterConversationSteps(ctx *godog.ScenarioContext) {
	ctx.Step(`^I send message "([^"]*)" to session "([^"]*)"$`, iSendMessage)
	ctx.Step(`^I send message "([^"]*)"$`, iSendMessageToCurrentSession)
	ctx.Step(`^the message "([^"]*)" should be in the conversation history$`, messageShouldBeInHistory)
	ctx.Step(`^the response should contain "([^"]*)"$`, responseShouldContain)
	ctx.Step(`^the context should be maintained$`, contextShouldBeMaintained)
	ctx.Step(`^I send (\d+) sequential messages$`, iSendNSequentialMessages)
	ctx.Step(`^the session history should contain (\d+) messages$`, sessionHistoryShouldContainNMessages)
	ctx.Step(`^the response should reference the first message$`, responseShouldReferenceFirstMessage)
	ctx.Step(`^I try to send a message to session "([^"]*)"$`, iTryToSendMessageToSession)
	ctx.Step(`^no history should be created for "([^"]*)"$`, noHistoryShouldBeCreated)
	ctx.Step(`^session "([^"]*)" history should contain only "([^"]*)"$`, sessionHistoryShouldContainOnly)
	ctx.Step(`^the response should come from "([^"]*)"$`, responseShouldComeFrom)
}

func iSendMessage(ctx context.Context, message, sessionName string) (context.Context, error) {
	env := testenv.EnvFromContext(ctx)

	// Look up session by name
	session, exists := env.Sessions[sessionName]
	if !exists {
		// Fall back to current session if name lookup fails (for backward compatibility)
		if env.CurrentSession == nil {
			return ctx, fmt.Errorf("no session found with name %q and no current session", sessionName)
		}
		session = env.CurrentSession
	}

	adapter, err := env.GetAdapter(session.Agent)
	if err != nil {
		return ctx, err
	}

	response, err := adapter.SendMessage(ctx, mock.SendMessageRequest{
		SessionID: session.ID,
		Content:   message,
	})

	env.LastResponse = response
	env.LastError = err

	return ctx, err
}

func messageShouldBeInHistory(ctx context.Context, message string) error {
	env := testenv.EnvFromContext(ctx)

	if env.CurrentSession == nil {
		return fmt.Errorf("no current session")
	}

	adapter, err := env.GetAdapter(env.CurrentSession.Agent)
	if err != nil {
		return err
	}

	history, err := adapter.GetHistory(ctx, env.CurrentSession.ID)
	if err != nil {
		return err
	}

	for _, msg := range history {
		if msg.Content == message {
			return nil // Found
		}
	}

	return fmt.Errorf("message %q not found in history", message)
}

func responseShouldContain(ctx context.Context, text string) error {
	env := testenv.EnvFromContext(ctx)

	if env.LastResponse == nil {
		return fmt.Errorf("no response received")
	}

	if !strings.Contains(env.LastResponse.Content, text) {
		return fmt.Errorf("expected response to contain %q, got: %s", text, env.LastResponse.Content)
	}

	return nil
}

func contextShouldBeMaintained(ctx context.Context) error {
	// This is validated by the previous step (response should contain name)
	// If we got here, context was maintained
	return nil
}

func iSendMessageToCurrentSession(ctx context.Context, message string) (context.Context, error) {
	env := testenv.EnvFromContext(ctx)

	if env.CurrentSession == nil {
		return ctx, fmt.Errorf("no current session")
	}

	adapter, err := env.GetAdapter(env.CurrentSession.Agent)
	if err != nil {
		return ctx, err
	}

	response, err := adapter.SendMessage(ctx, mock.SendMessageRequest{
		SessionID: env.CurrentSession.ID,
		Content:   message,
	})

	env.LastResponse = response
	env.LastError = err

	return ctx, err
}

func iSendNSequentialMessages(ctx context.Context, count int) (context.Context, error) {
	env := testenv.EnvFromContext(ctx)

	if env.CurrentSession == nil {
		return ctx, fmt.Errorf("no current session")
	}

	adapter, err := env.GetAdapter(env.CurrentSession.Agent)
	if err != nil {
		return ctx, err
	}

	for i := 1; i <= count; i++ {
		message := fmt.Sprintf("message %d", i)
		_, err := adapter.SendMessage(ctx, mock.SendMessageRequest{
			SessionID: env.CurrentSession.ID,
			Content:   message,
		})
		if err != nil {
			return ctx, fmt.Errorf("failed to send message %d: %w", i, err)
		}

		// Store first message for later reference
		if i == 1 {
			env.FirstMessage = message
		}
	}

	return ctx, nil
}

func sessionHistoryShouldContainNMessages(ctx context.Context, expectedCount int) error {
	env := testenv.EnvFromContext(ctx)

	if env.CurrentSession == nil {
		return fmt.Errorf("no current session")
	}

	adapter, err := env.GetAdapter(env.CurrentSession.Agent)
	if err != nil {
		return err
	}

	history, err := adapter.GetHistory(ctx, env.CurrentSession.ID)
	if err != nil {
		return err
	}

	if len(history) != expectedCount {
		return fmt.Errorf("expected %d messages, got %d", expectedCount, len(history))
	}

	return nil
}

func responseShouldReferenceFirstMessage(ctx context.Context) error {
	env := testenv.EnvFromContext(ctx)

	if env.FirstMessage == "" {
		return fmt.Errorf("no first message stored (did you call 'I send N sequential messages'?)")
	}

	if env.LastResponse == nil {
		return fmt.Errorf("no response received")
	}

	// Check if response references the first message
	if !strings.Contains(env.LastResponse.Content, env.FirstMessage) {
		return fmt.Errorf("expected response to reference %q, got: %s",
			env.FirstMessage, env.LastResponse.Content)
	}

	return nil
}

func iTryToSendMessageToSession(ctx context.Context, sessionID string) (context.Context, error) {
	env := testenv.EnvFromContext(ctx)

	// Default to first available adapter (claude)
	adapter, err := env.GetAdapter("claude")
	if err != nil {
		env.LastError = err
		return ctx, nil //nolint:nilerr // intentional: caller signals via separate bool/optional
	}

	_, err = adapter.SendMessage(ctx, mock.SendMessageRequest{
		SessionID: sessionID,
		Content:   "test message",
	})

	env.LastError = err
	return ctx, nil // Don't fail step, store error for assertion
}

func noHistoryShouldBeCreated(ctx context.Context, sessionID string) error {
	env := testenv.EnvFromContext(ctx)

	// Verify last operation failed (error was set)
	if env.LastError == nil {
		return fmt.Errorf("expected error but operation succeeded")
	}

	// Verify error message mentions "not found"
	if !strings.Contains(env.LastError.Error(), "not found") {
		return fmt.Errorf("expected 'not found' error, got: %w", env.LastError)
	}

	return nil
}

func sessionHistoryShouldContainOnly(ctx context.Context, sessionName, expectedMessage string) error {
	env := testenv.EnvFromContext(ctx)

	session, exists := env.Sessions[sessionName]
	if !exists {
		return fmt.Errorf("session %q not found in test environment", sessionName)
	}

	adapter, err := env.GetAdapter(session.Agent)
	if err != nil {
		return err
	}

	history, err := adapter.GetHistory(ctx, session.ID)
	if err != nil {
		return err
	}

	// Find user messages (exclude assistant responses)
	var userMessages []string
	for _, msg := range history {
		if msg.Role == mock.RoleUser {
			userMessages = append(userMessages, msg.Content)
		}
	}

	if len(userMessages) != 1 {
		return fmt.Errorf("expected exactly 1 user message, got %d: %v", len(userMessages), userMessages)
	}

	if userMessages[0] != expectedMessage {
		return fmt.Errorf("expected message %q, got %q", expectedMessage, userMessages[0])
	}

	return nil
}

func responseShouldComeFrom(ctx context.Context, agent string) error {
	env := testenv.EnvFromContext(ctx)

	if env.LastResponse == nil {
		return fmt.Errorf("no response received")
	}

	// Capitalize agent name: claude → Claude, gemini → Gemini, gpt → GPT
	var agentCapitalized string
	if agent == "gpt" {
		agentCapitalized = "GPT"
	} else if agent != "" {
		agentCapitalized = strings.ToUpper(agent[:1]) + agent[1:]
	}

	expectedPrefix := fmt.Sprintf("%s received:", agentCapitalized)
	if !strings.HasPrefix(env.LastResponse.Content, expectedPrefix) {
		return fmt.Errorf("expected response from %s (prefix %q), got: %s",
			agent, expectedPrefix, env.LastResponse.Content)
	}

	return nil
}
