package profile

import (
	"errors"
	"testing"
	"time"
)

func TestStaticProfileValidate(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"ok simple", "ce-ywii", false},
		{"ok dotted", "agent.worker_1", false},
		{"empty", "", true},
		{"slash traversal", "../escape", true},
		{"dotdot", "..", true},
		{"dot", ".", true},
		{"with slash", "a/b", true},
		{"with space", "a b", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := StaticProfile{AgentID: tt.id}.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate(%q) err=%v, wantErr=%v", tt.id, err, tt.wantErr)
			}
			if tt.wantErr && err != nil && !errors.Is(err, ErrInvalidID) {
				t.Fatalf("expected ErrInvalidID, got %v", err)
			}
		})
	}
}

func TestDynamicProfileValidate(t *testing.T) {
	if err := (DynamicProfile{AgentID: "a", SessionID: "s"}).Validate(); err != nil {
		t.Fatalf("valid profile rejected: %v", err)
	}
	if err := (DynamicProfile{AgentID: "a", SessionID: ""}).Validate(); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("expected ErrInvalidID for empty session, got %v", err)
	}
	if err := (DynamicProfile{AgentID: "../x", SessionID: "s"}).Validate(); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("expected ErrInvalidID for bad agent, got %v", err)
	}
}

func TestStaticProfileRoundTripFields(t *testing.T) {
	// Sanity that the struct carries the identity dimensions the bead requires.
	p := StaticProfile{
		AgentID:      "ce-ywii",
		Name:         "ce-ywii-worker",
		Role:         "go-implementer",
		Capabilities: []string{"edit-go", "run-tests"},
		Constraints:  []string{"no-force-push", "no-src-writes"},
		Model:        "claude-opus-4-8",
		SpawnedAt:    time.Unix(1000, 0),
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("valid profile rejected: %v", err)
	}
	if len(p.Capabilities) != 2 || len(p.Constraints) != 2 {
		t.Fatalf("capability/constraint slices not preserved")
	}
}
