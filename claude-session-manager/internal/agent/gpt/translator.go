package gpt

import (
	"time"

	"github.com/google/uuid"
	"github.com/sashabaranov/go-openai"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/agent"
)

// toOpenAIMessage converts an agent.Message to OpenAI ChatCompletionMessage.
func toOpenAIMessage(msg agent.Message) openai.ChatCompletionMessage {
	role := openai.ChatMessageRoleUser
	if msg.Role == agent.RoleAssistant {
		role = openai.ChatMessageRoleAssistant
	}
	return openai.ChatCompletionMessage{
		Role:    role,
		Content: msg.Content,
	}
}

// fromOpenAIMessage converts an OpenAI ChatCompletionMessage to agent.Message.
func fromOpenAIMessage(msg openai.ChatCompletionMessage, model string) agent.Message {
	role := agent.RoleAssistant
	if msg.Role == openai.ChatMessageRoleUser {
		role = agent.RoleUser
	}
	return agent.Message{
		ID:        uuid.New().String(),
		Role:      role,
		Content:   msg.Content,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"model": model,
		},
	}
}

// toOpenAIMessages converts a slice of agent.Message to OpenAI format.
func toOpenAIMessages(messages []agent.Message) []openai.ChatCompletionMessage {
	result := make([]openai.ChatCompletionMessage, len(messages))
	for i, msg := range messages {
		result[i] = toOpenAIMessage(msg)
	}
	return result
}
