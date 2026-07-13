package mergeloop

import (
	"reflect"
	"testing"
)

func TestBuildSessionNewArgs(t *testing.T) {
	req := AgentRequest{SessionName: "mergeloop/pr-42", Prompt: "fix CI"}
	want := []string{"session", "new", "mergeloop/pr-42", "--workspace", "oss", "--detached", "--prompt", "fix CI",
		"--harness", "opencode-cli", "--model", "glm-5.2"}
	got := BuildSessionNewArgs(req, "oss", SpawnRoute{Harness: "opencode-cli", Model: "glm-5.2"})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildSessionNewArgs() = %v, want %v", got, want)
	}
}

func TestBuildSessionNewArgsOmitsEmptyRouteValues(t *testing.T) {
	req := AgentRequest{SessionName: "mergeloop/pr-42", Prompt: "fix CI"}
	got := BuildSessionNewArgs(req, "oss", SpawnRoute{})
	for _, arg := range got {
		if arg == "--harness" || arg == "--model" {
			t.Fatalf("empty route emitted %q in %v", arg, got)
		}
	}
}
