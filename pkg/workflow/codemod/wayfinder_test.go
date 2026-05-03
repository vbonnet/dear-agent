package codemod

import (
	"strings"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/workflow"
)

func TestFromWayfinder_RoadmapPhasesBecomeNodes(t *testing.T) {
	in := []byte(`schema_version: "2.0"
project_name: "Auth Service"
description: "OAuth2 work"
roadmap:
  phases:
    - id: SETUP
      name: "Planning"
      status: completed
      tasks:
        - id: task-1.1
          title: "Break it down"
          status: completed
    - id: BUILD
      name: "BUILD Loop"
      status: in-progress
`)
	r, err := FromWayfinder(in, "session.yaml")
	if err != nil {
		t.Fatalf("FromWayfinder: %v", err)
	}
	if !r.Changed() {
		t.Fatalf("expected changes, got none")
	}
	w, err := workflow.LoadBytes(r.Output)
	if err != nil {
		t.Fatalf("synthesised workflow doesn't load: %v\noutput:\n%s", err, string(r.Output))
	}
	if w.Name != "auth-service" {
		t.Errorf("Name = %q, want auth-service", w.Name)
	}
	if len(w.Nodes) != 2 {
		t.Fatalf("Nodes = %d, want 2", len(w.Nodes))
	}
	if w.Nodes[0].ID != "setup" {
		t.Errorf("Nodes[0].ID = %q, want setup", w.Nodes[0].ID)
	}
	if w.Nodes[1].ID != "build" {
		t.Errorf("Nodes[1].ID = %q, want build", w.Nodes[1].ID)
	}
	// Linear phase chain → node[1] depends on node[0].
	if len(w.Nodes[1].Depends) != 1 || w.Nodes[1].Depends[0] != "setup" {
		t.Errorf("Nodes[1].Depends = %v, want [setup]", w.Nodes[1].Depends)
	}
	// Task summary as comment on the SETUP node.
	if !strings.Contains(string(r.Output), "tasks: task-1.1 \"Break it down\"") {
		t.Errorf("expected task summary comment in output:\n%s", string(r.Output))
	}
}

func TestFromWayfinder_FallsBackToWaypointHistory(t *testing.T) {
	in := []byte(`schema_version: "2.0"
project_name: "FallbackProj"
waypoint_history:
  - name: CHARTER
    status: completed
  - name: PROBLEM
    status: completed
`)
	r, err := FromWayfinder(in, "fallback.yaml")
	if err != nil {
		t.Fatalf("FromWayfinder: %v", err)
	}
	w, err := workflow.LoadBytes(r.Output)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(w.Nodes) != 2 {
		t.Fatalf("Nodes = %d, want 2", len(w.Nodes))
	}
	if w.Nodes[0].ID != "charter" {
		t.Errorf("Nodes[0].ID = %q", w.Nodes[0].ID)
	}
}

func TestFromWayfinder_RejectsEmptySession(t *testing.T) {
	in := []byte(`project_name: ""
`)
	if _, err := FromWayfinder(in, "x.yaml"); err == nil {
		t.Fatal("expected error for nameless session")
	}
}

func TestFromWayfinder_RejectsNoNodesOrWaypoints(t *testing.T) {
	in := []byte(`project_name: "Empty"
`)
	if _, err := FromWayfinder(in, "x.yaml"); err == nil {
		t.Fatal("expected error for session with no nodes or waypoints")
	}
}

func TestFromWayfinder_RejectsPhaseWithoutID(t *testing.T) {
	in := []byte(`project_name: "BadPhase"
roadmap:
  phases:
    - name: anonymous
`)
	if _, err := FromWayfinder(in, "x.yaml"); err == nil {
		t.Fatal("expected error for phase without id")
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Auth Service":     "auth-service",
		"task-8.1":         "task-8-1",
		"BUILD":            "build",
		"  Trim Me  ":      "trim-me",
		"!@#$":             "node",
		"Multi   Spaces":   "multi-spaces",
		"snake_case_thing": "snake-case-thing",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}
