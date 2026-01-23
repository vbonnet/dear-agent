package launcher

import (
	"strings"
	"testing"

	"github.com/[REDACTED_EMPLOYER]-src/ai-tools/autonomous-swarm/pkg/taskqueue"
)

func TestBuildGraph(t *testing.T) {
	tests := []struct {
		name  string
		beads []taskqueue.Bead
	}{
		{
			name:  "empty graph",
			beads: []taskqueue.Bead{},
		},
		{
			name: "single bead",
			beads: []taskqueue.Bead{
				{ID: "A", DependsOn: []string{}},
			},
		},
		{
			name: "linear chain",
			beads: []taskqueue.Bead{
				{ID: "A", DependsOn: []string{}},
				{ID: "B", DependsOn: []string{"A"}},
				{ID: "C", DependsOn: []string{"B"}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph, err := BuildGraph(tt.beads)
			if err != nil {
				t.Fatalf("BuildGraph() error = %v", err)
			}

			if len(graph.beads) != len(tt.beads) {
				t.Errorf("graph.beads length = %d, want %d", len(graph.beads), len(tt.beads))
			}
		})
	}
}

func TestTopologicalSort(t *testing.T) {
	tests := []struct {
		name      string
		beads     []taskqueue.Bead
		wantOrder []string // Expected order (or one valid order for parallel beads)
		wantError bool
	}{
		{
			name:      "empty graph",
			beads:     []taskqueue.Bead{},
			wantOrder: []string{},
			wantError: false,
		},
		{
			name: "single bead",
			beads: []taskqueue.Bead{
				{ID: "A", DependsOn: []string{}},
			},
			wantOrder: []string{"A"},
			wantError: false,
		},
		{
			name: "linear chain A→B→C",
			beads: []taskqueue.Bead{
				{ID: "A", DependsOn: []string{}},
				{ID: "B", DependsOn: []string{"A"}},
				{ID: "C", DependsOn: []string{"B"}},
			},
			wantOrder: []string{"A", "B", "C"},
			wantError: false,
		},
		{
			name: "parallel beads (all independent)",
			beads: []taskqueue.Bead{
				{ID: "A", DependsOn: []string{}},
				{ID: "B", DependsOn: []string{}},
				{ID: "C", DependsOn: []string{}},
			},
			// Order doesn't matter for parallel beads - just verify all are present
			wantOrder: []string{"A", "B", "C"},
			wantError: false,
		},
		{
			name: "diamond graph A→B,C→D",
			beads: []taskqueue.Bead{
				{ID: "A", DependsOn: []string{}},
				{ID: "B", DependsOn: []string{"A"}},
				{ID: "C", DependsOn: []string{"A"}},
				{ID: "D", DependsOn: []string{"B", "C"}},
			},
			// Valid orders: A, B, C, D or A, C, B, D (B and C are interchangeable)
			// We just verify A comes first and D comes last
			wantOrder: []string{}, // Special case: verify A first, D last
			wantError: false,
		},
		{
			name: "circular dependency A→B→A",
			beads: []taskqueue.Bead{
				{ID: "A", DependsOn: []string{"B"}},
				{ID: "B", DependsOn: []string{"A"}},
			},
			wantOrder: nil,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph, err := BuildGraph(tt.beads)
			if err != nil {
				t.Fatalf("BuildGraph() error = %v", err)
			}

			sorted, err := graph.TopologicalSort()

			if tt.wantError {
				if err == nil {
					t.Errorf("TopologicalSort() expected error, got nil")
				}
				if !strings.Contains(err.Error(), "circular dependency") {
					t.Errorf("TopologicalSort() error = %v, want circular dependency error", err)
				}
				return
			}

			if err != nil {
				t.Errorf("TopologicalSort() unexpected error = %v", err)
				return
			}

			// Special case: diamond graph - just verify A first, D last
			if tt.name == "diamond graph A→B,C→D" {
				if len(sorted) != 4 {
					t.Errorf("TopologicalSort() length = %d, want 4", len(sorted))
				}
				if sorted[0] != "A" {
					t.Errorf("TopologicalSort() first element = %s, want A", sorted[0])
				}
				if sorted[3] != "D" {
					t.Errorf("TopologicalSort() last element = %s, want D", sorted[3])
				}
				return
			}

			// For parallel beads, just verify all beads are present
			if tt.name == "parallel beads (all independent)" {
				if len(sorted) != len(tt.wantOrder) {
					t.Errorf("TopologicalSort() length = %d, want %d", len(sorted), len(tt.wantOrder))
				}
				// Create set of expected beads
				expected := make(map[string]bool)
				for _, id := range tt.wantOrder {
					expected[id] = true
				}
				// Verify all sorted beads are in expected set
				for _, id := range sorted {
					if !expected[id] {
						t.Errorf("TopologicalSort() unexpected bead %s", id)
					}
				}
				return
			}

			// For other cases, verify exact order
			if len(sorted) != len(tt.wantOrder) {
				t.Errorf("TopologicalSort() length = %d, want %d", len(sorted), len(tt.wantOrder))
			}

			for i, id := range sorted {
				if i < len(tt.wantOrder) && id != tt.wantOrder[i] {
					t.Errorf("TopologicalSort() position %d = %s, want %s", i, id, tt.wantOrder[i])
				}
			}
		})
	}
}

func TestTopologicalSort_ComplexCycle(t *testing.T) {
	// Test cycle: A→B→C→A
	beads := []taskqueue.Bead{
		{ID: "A", DependsOn: []string{"C"}},
		{ID: "B", DependsOn: []string{"A"}},
		{ID: "C", DependsOn: []string{"B"}},
	}

	graph, err := BuildGraph(beads)
	if err != nil {
		t.Fatalf("BuildGraph() error = %v", err)
	}

	_, err = graph.TopologicalSort()
	if err == nil {
		t.Errorf("TopologicalSort() expected error for cycle A→B→C→A, got nil")
	}

	if !strings.Contains(err.Error(), "circular dependency") {
		t.Errorf("TopologicalSort() error = %v, want circular dependency error", err)
	}

	// Verify error message contains cycle beads
	errMsg := err.Error()
	if !strings.Contains(errMsg, "A") || !strings.Contains(errMsg, "B") || !strings.Contains(errMsg, "C") {
		t.Errorf("TopologicalSort() error message should contain all cycle beads (A, B, C), got: %v", err)
	}
}
