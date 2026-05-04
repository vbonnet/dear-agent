package router

import (
	"context"

	"github.com/vbonnet/dear-agent/pkg/llm/provider"
	"github.com/vbonnet/dear-agent/pkg/workflow"
)

// AIExecutor adapts a *Router to the workflow.AIExecutor interface so
// the workflow runner can use role-based routing without knowing about
// it. Construct one per process and pass it to workflow.NewRunner.
//
// Resolution rules at call time:
//
//	1. If node.Model is set, route that literal model id (the workflow
//	   author asked for a specific model — honour it).
//	2. Else if node.Role is set, route via the role chain.
//	3. Else use the router's configured default role. If neither is set,
//	   return an error.
type AIExecutor struct {
	router *Router
}

// NewAIExecutor wraps a Router as a workflow.AIExecutor.
func NewAIExecutor(r *Router) *AIExecutor {
	return &AIExecutor{router: r}
}

// Generate implements workflow.AIExecutor. The runner has already
// rendered any templates in node.Prompt and node.System before calling.
func (e *AIExecutor) Generate(
	ctx context.Context,
	node *workflow.AINode,
	_ map[string]string,
	_ map[string]string,
) (string, error) {
	req := &provider.GenerateRequest{
		Prompt:       node.Prompt,
		SystemPrompt: node.System,
		MaxTokens:    node.MaxTokens,
		Metadata: map[string]any{
			"workflow_effort": node.Effort,
		},
	}

	if node.Model != "" {
		resp, err := e.router.GenerateForModel(ctx, node.Model, req)
		if err != nil {
			return "", err
		}
		return resp.Text, nil
	}

	resp, err := e.router.Generate(ctx, node.Role, req)
	if err != nil {
		return "", err
	}
	return resp.Text, nil
}
