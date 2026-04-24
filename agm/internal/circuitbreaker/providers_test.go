package circuitbreaker

import (
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

func TestAGMSessionCounter_CountWorkers(t *testing.T) {
	tests := []struct {
		name     string
		sessions []ops.SessionSummary
		want     int
	}{
		{
			name:     "nil sessions",
			sessions: nil,
			want:     0,
		},
		{
			name:     "empty sessions",
			sessions: []ops.SessionSummary{},
			want:     0,
		},
		{
			name: "only role:worker sessions counted",
			sessions: []ops.SessionSummary{
				{Name: "w1", Status: "active", Tags: []string{"role:worker"}},
				{Name: "w2", Status: "active", Tags: []string{"role:worker", "cap:web-search"}},
			},
			want: 2,
		},
		{
			name: "human sessions excluded (no role tag)",
			sessions: []ops.SessionSummary{
				{Name: "my-session", Status: "active"},
				{Name: "debug-thing", Status: "active", Tags: []string{"cap:claude-code"}},
			},
			want: 0,
		},
		{
			name: "supervisor sessions excluded",
			sessions: []ops.SessionSummary{
				{Name: "orchestrator", Status: "active", Tags: []string{"role:orchestrator"}},
				{Name: "overseer", Status: "active", Tags: []string{"role:overseer"}},
			},
			want: 0,
		},
		{
			name: "archived workers excluded",
			sessions: []ops.SessionSummary{
				{Name: "w1", Status: "archived", Tags: []string{"role:worker"}},
				{Name: "w2", Status: "active", Tags: []string{"role:worker"}},
			},
			want: 1,
		},
		{
			name: "mixed: only active role:worker counted",
			sessions: []ops.SessionSummary{
				{Name: "w1", Status: "active", Tags: []string{"role:worker"}},
				{Name: "human", Status: "active"},
				{Name: "orch", Status: "active", Tags: []string{"role:orchestrator"}},
				{Name: "w2", Status: "stopped", Tags: []string{"role:worker"}},
				{Name: "w3", Status: "archived", Tags: []string{"role:worker"}},
			},
			want: 2,
		},
		{
			name: "stopped worker with role:worker still counted",
			sessions: []ops.SessionSummary{
				{Name: "w1", Status: "stopped", Tags: []string{"role:worker"}},
			},
			want: 1,
		},
	}

	counter := &AGMSessionCounter{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := counter.CountWorkers(tt.sessions)
			if got != tt.want {
				t.Errorf("CountWorkers() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestHasTag(t *testing.T) {
	tests := []struct {
		name   string
		tags   []string
		target string
		want   bool
	}{
		{"nil tags", nil, "role:worker", false},
		{"empty tags", []string{}, "role:worker", false},
		{"has tag", []string{"role:worker", "cap:web-search"}, "role:worker", true},
		{"does not have tag", []string{"role:orchestrator"}, "role:worker", false},
		{"partial match not accepted", []string{"role:worker-v2"}, "role:worker", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasTag(tt.tags, tt.target)
			if got != tt.want {
				t.Errorf("hasTag(%v, %q) = %v, want %v", tt.tags, tt.target, got, tt.want)
			}
		})
	}
}
