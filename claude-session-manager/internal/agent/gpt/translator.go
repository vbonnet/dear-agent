package gpt

import (
	"time"

	"github.com/google/uuid"
	"github.com/openai/openai-go"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/agent"
)

// toOpenAIMessage converts an agent.Message to OpenAI ChatCompletionMessageParamUnion.
func toOpenAIMessage(msg agent.Message) openai.ChatCompletionMessageParamUnion {
	if msg.Role == agent.RoleUser {
		return openai.UserMessage(msg.Content)
	}
	return openai.AssistantMessage(msg.Content)
}

// fromOpenAIMessage converts an OpenAI ChatCompletionMessage to agent.Message.
func fromOpenAIMessage(msg openai.ChatCompletionMessage, model string) agent.Message {
	return agent.Message{
		ID:        uuid.New().String(),
		Role:      agent.Role(msg.Role),
		Content:   msg.Content,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"model": model,
		},
	}
}

// toOpenAIMessages converts a slice of agent.Message to OpenAI format.
func toOpenAIMessages(messages []agent.Message) []openai.ChatCompletionMessageParamUnion {
	result := make([]openai.ChatCompletionMessageParamUnion, len(messages))
	for i, msg := range messages {
		result[i] = toOpenAIMessage(msg)
	}
	return result
}
