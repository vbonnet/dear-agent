package vroomprompt

import (
	"strings"
	"testing"
)

func TestWorkerRulePreservesRoute(t *testing.T) {
	got := WorkerRule(Route{Harness: "opencode-cli", Model: "deepseek-v4", Mode: "auto", Workspace: "oss"})
	for _, want := range []string{"harness=opencode-cli", "model=deepseek-v4", "--mode=auto", "--workspace=oss"} {
		if !strings.Contains(got, want) {
			t.Fatalf("WorkerRule() = %q, missing %q", got, want)
		}
	}
}
