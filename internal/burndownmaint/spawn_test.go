package burndownmaint

import (
	"reflect"
	"testing"
)

func TestBuildSessionArgs(t *testing.T) {
	want := []string{"session", "new", "burndown-1", "--detached", "--harness", "opencode-cli", "--model", "qwen", "--workspace", "oss"}
	got := BuildSessionArgs("burndown-1", Route{Harness: "opencode-cli", Model: "qwen", Workspace: "oss"})
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildSessionArgs() = %v, want %v", got, want)
	}
}

func TestBuildSessionArgsOmitsEmptyRouteValues(t *testing.T) {
	got := BuildSessionArgs("burndown-1", Route{})
	want := []string{"session", "new", "burndown-1", "--detached"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("BuildSessionArgs() = %v, want %v", got, want)
	}
}
