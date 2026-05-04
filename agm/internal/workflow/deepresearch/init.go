package deepresearch

import (
	"github.com/vbonnet/dear-agent/agm/internal/workflow"
)

func init() {
	// Register Gemini deep-research workflow
	workflow.Register(NewGeminiDeepResearch())
}
