package a2a_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/a2a"
)

func TestSessionCard_HarnessDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		harness string
		name    string
	}{
		{harness: "claude-code", name: "Claude Code Session session-1"},
		{harness: "codex-cli", name: "Codex Session session-1"},
		{harness: "agy", name: "Antigravity Session session-1"},
		{harness: "opencode-cli", name: "OpenCode Session session-1"},
	}

	for _, tt := range tests {
		t.Run(tt.harness, func(t *testing.T) {
			t.Parallel()
			card := a2a.SessionCard{Harness: tt.harness, SessionID: "session-1"}.Build()
			if card.Name != tt.name {
				t.Fatalf("Name = %q, want %q", card.Name, tt.name)
			}
			if len(card.Skills) != 1 || !slices.Contains(card.Skills[0].Tags, tt.harness) {
				t.Fatalf("default skill tags = %v, want harness %q", card.Skills[0].Tags, tt.harness)
			}
			if !strings.Contains(card.Skills[0].Description, strings.TrimSuffix(tt.name, " Session session-1")) {
				t.Fatalf("default skill description = %q", card.Skills[0].Description)
			}
		})
	}
}

func TestSessionCard_EmptyHarnessIsNeutral(t *testing.T) {
	t.Parallel()

	card := a2a.SessionCard{SessionID: "session-1"}.Build()
	if card.Name != "Agent Session session-1" {
		t.Fatalf("Name = %q, want neutral name", card.Name)
	}
	if got := card.Skills[0].Tags; !slices.Equal(got, []string{"general"}) {
		t.Fatalf("default skill tags = %v, want no harness tag", got)
	}
}

func TestSessionCard_ExplicitPresentationWins(t *testing.T) {
	t.Parallel()

	customSkills := a2a.SessionCard{Harness: "claude-code"}.Build().Skills
	card := a2a.SessionCard{
		Harness: "codex-cli",
		Name:    "review worker",
		Skills:  customSkills,
	}.Build()
	if card.Name != "review worker" {
		t.Fatalf("Name = %q, want explicit name", card.Name)
	}
	if !reflect.DeepEqual(card.Skills, customSkills) {
		t.Fatalf("explicit skills were replaced: %#v", card.Skills)
	}
}
