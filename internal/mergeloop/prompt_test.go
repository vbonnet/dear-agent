package mergeloop

import (
	"strings"
	"testing"
)

func TestBuildPromptUsesSafePush(t *testing.T) {
	prompt := BuildPrompt("owner/repo", PR{Number: 42}, AgentFixCI)
	if !strings.Contains(prompt, "`safe-push` for every push") {
		t.Fatalf("prompt does not require safe-push: %q", prompt)
	}
	if strings.Contains(prompt, "git push") {
		t.Fatalf("prompt contains raw git-push guidance: %q", prompt)
	}
}
