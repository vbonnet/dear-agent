package deep_research

import (
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/workflow"
)

func init() {
	// Register Gemini deep-research workflow
	workflow.Register(NewGeminiDeepResearch())
}
