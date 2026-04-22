package monitoring

import (
	"context"
	"regexp"
	"sync"
	"testing"

	"github.com/vbonnet/dear-agent/pkg/eventbus"
)

// collectingBus captures published events for test assertions.
type collectingBus struct {
	mu     sync.Mutex
	events []*eventbus.Event
}

func newCollectingBus() (*eventbus.LocalBus, *collectingBus) {
	bus := eventbus.NewBus(nil)
	col := &collectingBus{}
	bus.Subscribe("*", "test-collector", func(_ context.Context, e *eventbus.Event) (*eventbus.Response, error) {
		col.mu.Lock()
		col.events = append(col.events, e)
		col.mu.Unlock()
		return nil, nil
	})
	return bus, col
}

func (c *collectingBus) drain() []*eventbus.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.events
	c.events = nil
	return out
}

func TestDefaultCommandPatterns_Coverage(t *testing.T) {
	patterns := DefaultCommandPatterns()
	if len(patterns) == 0 {
		t.Fatal("DefaultCommandPatterns returned empty slice")
	}
	names := map[string]bool{}
	for _, p := range patterns {
		if p.Name == "" {
			t.Error("pattern has empty Name")
		}
		if p.Regex == nil {
			t.Errorf("pattern %q has nil Regex", p.Name)
		}
		if p.EventType == "" {
			t.Errorf("pattern %q has empty EventType", p.Name)
		}
		if p.ExtractData == nil {
			t.Errorf("pattern %q has nil ExtractData", p.Name)
		}
		names[p.Name] = true
	}

	expected := []string{
		"go_test_start", "go_test_result",
		"npm_test_start", "npm_test_summary",
		"pytest_start", "pytest_summary",
		"git_commit_cli", "git_push", "make_build",
	}
	for _, n := range expected {
		if !names[n] {
			t.Errorf("expected pattern %q missing", n)
		}
	}
}

func TestParseLine_GoTestResult(t *testing.T) {
	bus, col := newCollectingBus()
	defer bus.Close()
	op := NewOutputParser("agent-1", bus)

	op.ParseLine("ok  \tgithub.com/foo/bar\t1.234s")

	events := col.drain()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Type != EventTestPassed {
		t.Errorf("expected event type %q, got %q", EventTestPassed, e.Type)
	}
	if e.Data["status"] != "ok" {
		t.Errorf("expected status ok, got %v", e.Data["status"])
	}
	if e.Data["package"] != "github.com/foo/bar" {
		t.Errorf("expected package github.com/foo/bar, got %v", e.Data["package"])
	}
	if e.Data["agent_id"] != "agent-1" {
		t.Errorf("expected agent_id agent-1, got %v", e.Data["agent_id"])
	}
}

func TestParseLine_GoTestFailed(t *testing.T) {
	bus, col := newCollectingBus()
	defer bus.Close()
	op := NewOutputParser("agent-2", bus)

	op.ParseLine("FAIL\tgithub.com/foo/baz\t0.500s")

	events := col.drain()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Data["status"] != "FAIL" {
		t.Errorf("expected status FAIL, got %v", events[0].Data["status"])
	}
}

func TestParseLine_NpmTestSummary(t *testing.T) {
	bus, col := newCollectingBus()
	defer bus.Close()
	op := NewOutputParser("agent-3", bus)

	op.ParseLine("Tests: 10 passed, 2 failed, 12 total")

	events := col.drain()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Data["passed"] != "10" {
		t.Errorf("expected passed=10, got %v", e.Data["passed"])
	}
	if e.Data["failed"] != "2" {
		t.Errorf("expected failed=2, got %v", e.Data["failed"])
	}
	if e.Data["total"] != "12" {
		t.Errorf("expected total=12, got %v", e.Data["total"])
	}
}

func TestParseLine_PytestSummary(t *testing.T) {
	bus, col := newCollectingBus()
	defer bus.Close()
	op := NewOutputParser("agent-4", bus)

	op.ParseLine("====== 42 passed in 3.21s ======")

	events := col.drain()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	e := events[0]
	if e.Data["passed"] != "42" {
		t.Errorf("expected passed=42, got %v", e.Data["passed"])
	}
	if e.Data["duration"] != "3.21s" {
		t.Errorf("expected duration=3.21s, got %v", e.Data["duration"])
	}
}

func TestParseLine_GitCommit(t *testing.T) {
	bus, col := newCollectingBus()
	defer bus.Close()
	op := NewOutputParser("agent-5", bus)

	op.ParseLine("git commit -m 'fix: something'")

	events := col.drain()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "sub_agent.command.git_commit" {
		t.Errorf("unexpected event type: %s", events[0].Type)
	}
}

func TestParseLine_GitPush(t *testing.T) {
	bus, col := newCollectingBus()
	defer bus.Close()
	op := NewOutputParser("agent-6", bus)

	op.ParseLine("git push origin main")

	events := col.drain()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "sub_agent.command.git_push" {
		t.Errorf("unexpected event type: %s", events[0].Type)
	}
}

func TestParseLine_MakeBuild(t *testing.T) {
	bus, col := newCollectingBus()
	defer bus.Close()
	op := NewOutputParser("agent-7", bus)

	op.ParseLine("make build")

	events := col.drain()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "sub_agent.command.build" {
		t.Errorf("unexpected event type: %s", events[0].Type)
	}
}

func TestParseLine_NoMatch(t *testing.T) {
	bus, col := newCollectingBus()
	defer bus.Close()
	op := NewOutputParser("agent-8", bus)

	op.ParseLine("hello world, just some random output")

	events := col.drain()
	if len(events) != 0 {
		t.Errorf("expected no events for unmatched line, got %d", len(events))
	}
}

func TestParseLine_FirstPatternWins(t *testing.T) {
	bus, col := newCollectingBus()
	defer bus.Close()
	op := NewOutputParser("agent-9", bus)

	// "go test" matches go_test_start; should not also match other patterns
	op.ParseLine("go test ./...")

	events := col.drain()
	if len(events) != 1 {
		t.Fatalf("expected exactly 1 event (first match only), got %d", len(events))
	}
	if events[0].Type != EventTestStarted {
		t.Errorf("expected %s, got %s", EventTestStarted, events[0].Type)
	}
}

func TestParseLine_RawOutputPresent(t *testing.T) {
	bus, col := newCollectingBus()
	defer bus.Close()
	op := NewOutputParser("agent-10", bus)

	line := "ok  \tgithub.com/x/y\t0.001s"
	op.ParseLine(line)

	events := col.drain()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Data["raw_output"] != line {
		t.Errorf("raw_output mismatch: got %v", events[0].Data["raw_output"])
	}
}

func TestAddPattern(t *testing.T) {
	bus, col := newCollectingBus()
	defer bus.Close()
	op := NewOutputParser("agent-11", bus)

	op.AddPattern(CommandPattern{
		Name:      "custom",
		Regex:     regexp.MustCompile(`^CUSTOM:\s+(.+)`),
		EventType: "custom.event",
		ExtractData: func(matches []string) map[string]interface{} {
			return map[string]interface{}{"payload": matches[1]}
		},
	})

	op.ParseLine("CUSTOM: hello")
	events := col.drain()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "custom.event" {
		t.Errorf("expected custom.event, got %s", events[0].Type)
	}
	if events[0].Data["payload"] != "hello" {
		t.Errorf("expected payload=hello, got %v", events[0].Data["payload"])
	}
}

func TestParseLine_TimestampPresent(t *testing.T) {
	bus, col := newCollectingBus()
	defer bus.Close()
	op := NewOutputParser("agent-12", bus)

	op.ParseLine("go test ./...")

	events := col.drain()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if _, ok := events[0].Data["timestamp"]; !ok {
		t.Error("expected timestamp in event data")
	}
}

