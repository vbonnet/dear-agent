package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeAI is a scripted AIExecutor for tests. Returns the prompt back by
// default so templating and output plumbing are trivially testable.
type fakeAI struct {
	mu        sync.Mutex
	responses map[string]string // prompt substring → response
	calls     []string
	err       error
}

func (f *fakeAI) Generate(_ context.Context, node *AINode, _ map[string]string, _ map[string]string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, node.Prompt)
	if f.err != nil {
		return "", f.err
	}
	for sub, resp := range f.responses {
		if strings.Contains(node.Prompt, sub) {
			return resp, nil
		}
	}
	// Default: echo the prompt.
	return node.Prompt, nil
}

func TestRunnerSimpleAINode(t *testing.T) {
	ai := &fakeAI{responses: map[string]string{"hello": "world"}}
	r := NewRunner(ai)
	w := &Workflow{
		Name:    "simple",
		Version: "1",
		Nodes: []Node{
			{ID: "n1", Kind: KindAI, AI: &AINode{Prompt: "hello"}},
		},
	}
	rep, err := r.Run(context.Background(), w, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !rep.Succeeded {
		t.Error("expected Succeeded=true")
	}
	if len(rep.Results) != 1 || rep.Results[0].Output != "world" {
		t.Errorf("unexpected results: %+v", rep.Results)
	}
}

func TestRunnerTemplateInterpolation(t *testing.T) {
	ai := &fakeAI{}
	r := NewRunner(ai)
	w := &Workflow{
		Name:    "tpl",
		Version: "1",
		Inputs:  []InputSpec{{Name: "topic", Required: true}},
		Nodes: []Node{
			{ID: "a", Kind: KindAI, AI: &AINode{Prompt: "analyze {{.Inputs.topic}}"}},
			{ID: "b", Kind: KindAI, Depends: []string{"a"},
				AI: &AINode{Prompt: "refine based on: {{.Outputs.a}}"}},
		},
	}
	rep, err := r.Run(context.Background(), w, map[string]string{"topic": "flash-attention"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if rep.Results[0].Output != "analyze flash-attention" {
		t.Errorf("node a output wrong: %q", rep.Results[0].Output)
	}
	if !strings.Contains(rep.Results[1].Output, "analyze flash-attention") {
		t.Errorf("node b did not inherit upstream output: %q", rep.Results[1].Output)
	}
}

func TestRunnerRespectsDependencies(t *testing.T) {
	ai := &fakeAI{}
	r := NewRunner(ai)
	w := &Workflow{
		Name: "dep", Version: "1",
		Nodes: []Node{
			{ID: "c", Kind: KindAI, Depends: []string{"b"}, AI: &AINode{Prompt: "c"}},
			{ID: "a", Kind: KindAI, AI: &AINode{Prompt: "a"}},
			{ID: "b", Kind: KindAI, Depends: []string{"a"}, AI: &AINode{Prompt: "b"}},
		},
	}
	rep, err := r.Run(context.Background(), w, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var order []string
	for _, r := range rep.Results {
		order = append(order, r.NodeID)
	}
	if order[0] != "a" || order[1] != "b" || order[2] != "c" {
		t.Errorf("order = %v, want [a b c]", order)
	}
}

func TestRunnerBashNode(t *testing.T) {
	r := NewRunner(&fakeAI{})
	w := &Workflow{
		Name: "sh", Version: "1",
		Nodes: []Node{
			{ID: "n", Kind: KindBash, Bash: &BashNode{Cmd: "echo hello"}},
		},
	}
	rep, err := r.Run(context.Background(), w, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := strings.TrimSpace(rep.Results[0].Output)
	if got != "hello" {
		t.Errorf("bash output = %q, want hello", got)
	}
}

func TestRunnerBashNonzeroExitFails(t *testing.T) {
	r := NewRunner(&fakeAI{})
	w := &Workflow{
		Name: "sh", Version: "1",
		Nodes: []Node{
			{ID: "n", Kind: KindBash, Bash: &BashNode{Cmd: "exit 7"}},
		},
	}
	_, err := r.Run(context.Background(), w, nil)
	if err == nil {
		t.Error("non-zero exit should fail by default")
	}
}

func TestRunnerBashAllowNonzeroExit(t *testing.T) {
	r := NewRunner(&fakeAI{})
	w := &Workflow{
		Name: "sh", Version: "1",
		Nodes: []Node{
			{ID: "n", Kind: KindBash, Bash: &BashNode{Cmd: "exit 3", AllowNonzeroExit: true}},
		},
	}
	rep, err := r.Run(context.Background(), w, nil)
	if err != nil {
		t.Fatalf("allow-nonzero should succeed: %v", err)
	}
	if code, _ := rep.Results[0].Meta["exit_code"].(int); code != 3 {
		t.Errorf("exit_code = %v, want 3", code)
	}
}

func TestRunnerGateBlocksUntilSignal(t *testing.T) {
	r := NewRunner(&fakeAI{})
	w := &Workflow{
		Name: "gate", Version: "1",
		Nodes: []Node{
			{ID: "g", Kind: KindGate, Gate: &GateNode{Name: "approve"}},
		},
	}
	done := make(chan struct{})
	var runErr error
	go func() {
		_, runErr = r.Run(context.Background(), w, nil)
		close(done)
	}()
	// Give the gate a moment to install its signal channel.
	time.Sleep(50 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("Gate did not block")
	default:
	}
	r.Signal("approve")
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Gate did not unblock on Signal")
	}
	if runErr != nil {
		t.Errorf("Run err: %v", runErr)
	}
}

func TestRunnerGateTimeoutFires(t *testing.T) {
	r := NewRunner(&fakeAI{})
	r.SignalTimeout = 100 * time.Millisecond
	w := &Workflow{
		Name: "gate-timeout", Version: "1",
		Nodes: []Node{
			{ID: "g", Kind: KindGate, Gate: &GateNode{Name: "never"}},
		},
	}
	_, err := r.Run(context.Background(), w, nil)
	if err == nil {
		t.Error("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("err = %v, want timeout", err)
	}
}

func TestRunnerLoopUntilEquality(t *testing.T) {
	// Loop increments a counter bash node until it equals 3.
	counter := int32(0)
	ai := &fakeAI{}
	r := NewRunner(ai)
	// Use a shared shell file that holds the counter. Simpler: compute
	// from atomic + echo back.
	genCmd := func() string {
		// bash node can't read the atomic directly, so let's use a temp file
		// approach in the test. But we can also do it via the AI node: each
		// call returns the next integer. Simpler: make the AI node return
		// fixed sequence.
		_ = atomic.AddInt32(&counter, 1)
		return fmt.Sprintf("echo %d", counter)
	}
	// Install a responder that returns counter each time.
	ai.responses = map[string]string{"count": "will be replaced"}
	r.AI = &fakeAI{responses: map[string]string{"count": "3"}}
	_ = genCmd // unused; keep simple via static responses
	w := &Workflow{
		Name: "loop", Version: "1",
		Nodes: []Node{
			{ID: "lp", Kind: KindLoop, Loop: &LoopNode{
				Until:    "Outputs.step == 3",
				MaxIters: 10,
				Nodes: []Node{
					{ID: "step", Kind: KindAI, AI: &AINode{Prompt: "count"}},
				},
			}},
		},
	}
	rep, err := r.Run(context.Background(), w, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	iters, _ := rep.Results[0].Meta["iterations"].(int)
	if iters != 1 {
		// AI returns "3" on first call → Until is true after iter 1.
		t.Errorf("iterations = %d, want 1", iters)
	}
}

func TestRunnerLoopMaxIters(t *testing.T) {
	// Loop that never satisfies Until — bounded by MaxIters.
	r := NewRunner(&fakeAI{responses: map[string]string{"count": "0"}})
	w := &Workflow{
		Name: "loop-max", Version: "1",
		Nodes: []Node{
			{ID: "lp", Kind: KindLoop, Loop: &LoopNode{
				Until:    "Outputs.step == 3",
				MaxIters: 4,
				Nodes: []Node{
					{ID: "step", Kind: KindAI, AI: &AINode{Prompt: "count"}},
				},
			}},
		},
	}
	_, err := r.Run(context.Background(), w, nil)
	if err == nil || !strings.Contains(err.Error(), "max_iters") {
		t.Errorf("expected max_iters error, got %v", err)
	}
}

func TestRunnerPropagatesAIError(t *testing.T) {
	ai := &fakeAI{err: errors.New("provider down")}
	r := NewRunner(ai)
	w := &Workflow{
		Name: "err", Version: "1",
		Nodes: []Node{
			{ID: "n", Kind: KindAI, AI: &AINode{Prompt: "go"}},
		},
	}
	_, err := r.Run(context.Background(), w, nil)
	if err == nil || !strings.Contains(err.Error(), "provider down") {
		t.Errorf("expected provider-down error, got %v", err)
	}
}

func TestRunnerNodeTimeout(t *testing.T) {
	r := NewRunner(&fakeAI{})
	w := &Workflow{
		Name: "timeout", Version: "1",
		Nodes: []Node{
			{ID: "n", Kind: KindBash, Timeout: 100 * time.Millisecond,
				Bash: &BashNode{Cmd: "sleep 5"}},
		},
	}
	_, err := r.Run(context.Background(), w, nil)
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestMergeInputsRejectsMissingRequired(t *testing.T) {
	_, err := mergeInputs([]InputSpec{{Name: "x", Required: true}}, nil)
	if err == nil {
		t.Error("expected missing-input error")
	}
}

func TestMergeInputsAppliesDefault(t *testing.T) {
	out, err := mergeInputs([]InputSpec{{Name: "x", Default: "fallback"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out["x"] != "fallback" {
		t.Errorf("default not applied: %v", out)
	}
}

func TestEvalConditionFormats(t *testing.T) {
	nc := &nodeContext{
		outputs: map[string]string{"a": "done", "n": "42", "flag": "true"},
		inputs:  map[string]string{},
	}
	cases := []struct {
		expr string
		want bool
	}{
		{"Outputs.a == done", true},
		{"Outputs.a == pending", false},
		{"Outputs.a != pending", true},
		{"Outputs.n > 10", true},
		{"Outputs.n > 100", false},
		{"Outputs.flag", true},
		{"Outputs.missing", false},
	}
	for _, tc := range cases {
		got, err := evalCondition(tc.expr, nc)
		if err != nil {
			t.Errorf("evalCondition(%q): %v", tc.expr, err)
			continue
		}
		if got != tc.want {
			t.Errorf("evalCondition(%q) = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

func TestRunnerWhenClauseSkipsWhenFalse(t *testing.T) {
	ai := &fakeAI{responses: map[string]string{"decide": "NO"}}
	r := NewRunner(ai)
	w := &Workflow{
		Name: "when-false", Version: "1",
		Nodes: []Node{
			{ID: "decide", Kind: KindAI, AI: &AINode{Prompt: "decide"}},
			{ID: "branch_a", Kind: KindAI, Depends: []string{"decide"},
				When: "Outputs.decide == YES",
				AI:   &AINode{Prompt: "do-a"}},
			{ID: "branch_b", Kind: KindAI, Depends: []string{"decide"},
				When: "Outputs.decide == NO",
				AI:   &AINode{Prompt: "do-b"}},
		},
	}
	rep, err := r.Run(context.Background(), w, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	byID := make(map[string]Result, len(rep.Results))
	for _, res := range rep.Results {
		byID[res.NodeID] = res
	}
	if byID["decide"].Output != "NO" {
		t.Errorf("decide output = %q, want NO", byID["decide"].Output)
	}
	if skipped, _ := byID["branch_a"].Meta["skipped"].(bool); !skipped {
		t.Error("branch_a should have been skipped (When == YES but decision was NO)")
	}
	if byID["branch_a"].Output != "" {
		t.Errorf("skipped node should have empty output, got %q", byID["branch_a"].Output)
	}
	if skipped, _ := byID["branch_b"].Meta["skipped"].(bool); skipped {
		t.Error("branch_b should NOT have been skipped (When == NO matches)")
	}
	if byID["branch_b"].Output != "do-b" {
		t.Errorf("branch_b output = %q, want do-b", byID["branch_b"].Output)
	}
}

func TestRunnerWhenClauseTruthy(t *testing.T) {
	r := NewRunner(&fakeAI{})
	w := &Workflow{
		Name: "when-truthy", Version: "1",
		Inputs: []InputSpec{{Name: "flag", Required: true}},
		Nodes: []Node{
			{ID: "check", Kind: KindBash, When: "Inputs.flag",
				Bash: &BashNode{Cmd: "echo ran"}},
		},
	}
	rep, err := r.Run(context.Background(), w, map[string]string{"flag": "true"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.TrimSpace(rep.Results[0].Output) != "ran" {
		t.Errorf("expected bash to run; output = %q", rep.Results[0].Output)
	}
	rep, err = r.Run(context.Background(), w, map[string]string{"flag": "false"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if skipped, _ := rep.Results[0].Meta["skipped"].(bool); !skipped {
		t.Error("'false' flag should skip the node")
	}
}

func TestRunnerWhenClauseMalformedFails(t *testing.T) {
	r := NewRunner(&fakeAI{})
	w := &Workflow{
		Name: "when-bad", Version: "1",
		Nodes: []Node{
			{ID: "n", Kind: KindAI, When: "garbage.scope.x",
				AI: &AINode{Prompt: "x"}},
		},
	}
	_, err := r.Run(context.Background(), w, nil)
	if err == nil {
		t.Fatal("malformed When should fail the node (and the run)")
	}
	if !strings.Contains(err.Error(), "when") {
		t.Errorf("error = %v, want mention of when", err)
	}
}

func TestRunnerWhenClauseSkippedUpstreamDoesntBlockDownstream(t *testing.T) {
	ai := &fakeAI{responses: map[string]string{"first": "FIRST"}}
	r := NewRunner(ai)
	w := &Workflow{
		Name: "skip-chain", Version: "1",
		Nodes: []Node{
			{ID: "first", Kind: KindAI, AI: &AINode{Prompt: "first"}},
			{ID: "middle", Kind: KindAI, Depends: []string{"first"},
				When: "Outputs.first == SKIP-ME",
				AI:   &AINode{Prompt: "middle"}},
			{ID: "last", Kind: KindAI, Depends: []string{"middle"},
				AI: &AINode{Prompt: "last"}},
		},
	}
	rep, err := r.Run(context.Background(), w, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	byID := make(map[string]Result, len(rep.Results))
	for _, res := range rep.Results {
		byID[res.NodeID] = res
	}
	if skipped, _ := byID["middle"].Meta["skipped"].(bool); !skipped {
		t.Error("middle should be skipped")
	}
	if byID["last"].Output != "last" {
		t.Errorf("last should still run; output = %q", byID["last"].Output)
	}
}

func TestTopoOrderCycleRejected(t *testing.T) {
	nodes := []Node{
		{ID: "a", Kind: KindAI, AI: &AINode{Prompt: "a"}, Depends: []string{"b"}},
		{ID: "b", Kind: KindAI, AI: &AINode{Prompt: "b"}, Depends: []string{"a"}},
	}
	_, err := topoOrder(nodes)
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Errorf("expected cycle error, got %v", err)
	}
}

func TestRunnerBashEnvVarExposure(t *testing.T) {
	r := NewRunner(&fakeAI{})
	w := &Workflow{
		Name:    "env-exposure",
		Version: "1",
		Inputs:  []InputSpec{{Name: "greeting", Required: true}},
		Nodes: []Node{
			{ID: "n", Kind: KindBash, Bash: &BashNode{Cmd: "echo $INPUT_greeting"}},
		},
	}
	rep, err := r.Run(context.Background(), w, map[string]string{"greeting": "hello"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := strings.TrimSpace(rep.Results[0].Output)
	if got != "hello" {
		t.Errorf("INPUT_greeting = %q, want hello", got)
	}
}

// TestRunnerBashEnvVarPreservesCase guards Bug 1 — the shipped templates
// in configs/workflows/*.yaml reference inputs in their declared casing
// (e.g. $INPUT_db, $INPUT_repos_root). The runner used to uppercase
// every key, which silently broke every template that used a lowercase
// or mixed-case input name.
func TestRunnerBashEnvVarPreservesCase(t *testing.T) {
	r := NewRunner(&fakeAI{})
	w := &Workflow{
		Name:    "case-preserve",
		Version: "1",
		Inputs: []InputSpec{
			{Name: "repos_root", Required: true},
			{Name: "MixedCase", Required: true},
		},
		Nodes: []Node{
			{ID: "n", Kind: KindBash, Bash: &BashNode{
				Cmd: `printf '%s\n%s\n' "$INPUT_repos_root" "$INPUT_MixedCase"`,
			}},
		},
	}
	rep, err := r.Run(context.Background(), w, map[string]string{
		"repos_root": "/srv/repos",
		"MixedCase":  "preserved",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := strings.TrimSpace(rep.Results[0].Output)
	want := "/srv/repos\npreserved"
	if got != want {
		t.Errorf("output = %q, want %q (case must be preserved when exposing $INPUT_*)", got, want)
	}
}

// TestRunnerBashCapturesStderr guards Bug 4 — the comment on executeBash
// promises stderr is captured into the Meta map, but cmd.Output() only
// captures stdout. A failing bash node must surface its stderr via
// Result.Meta["stderr"] for diagnostics.
func TestRunnerBashCapturesStderr(t *testing.T) {
	r := NewRunner(&fakeAI{})
	w := &Workflow{
		Name: "stderr-capture", Version: "1",
		Nodes: []Node{
			{ID: "n", Kind: KindBash, Bash: &BashNode{
				Cmd:              `echo on-stdout; echo on-stderr 1>&2; exit 7`,
				AllowNonzeroExit: true,
			}},
		},
	}
	rep, err := r.Run(context.Background(), w, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	res := rep.Results[0]
	if got := strings.TrimSpace(res.Output); got != "on-stdout" {
		t.Errorf("stdout = %q, want %q (stderr must not bleed into Output)", got, "on-stdout")
	}
	stderr, ok := res.Meta["stderr"].(string)
	if !ok {
		t.Fatalf("Meta[\"stderr\"] missing or wrong type: %#v", res.Meta["stderr"])
	}
	if !strings.Contains(stderr, "on-stderr") {
		t.Errorf("Meta[\"stderr\"] = %q, want it to contain %q", stderr, "on-stderr")
	}
	if code, _ := res.Meta["exit_code"].(int); code != 7 {
		t.Errorf("exit_code = %v, want 7", code)
	}
}

// TestRunnerBashStderrAbsentWhenSilent makes sure the runner does not
// add an empty "stderr" key when nothing was written to stderr — keeps
// Meta tidy for the common case.
func TestRunnerBashStderrAbsentWhenSilent(t *testing.T) {
	r := NewRunner(&fakeAI{})
	w := &Workflow{
		Name: "silent-stderr", Version: "1",
		Nodes: []Node{
			{ID: "n", Kind: KindBash, Bash: &BashNode{Cmd: `echo quiet`}},
		},
	}
	rep, err := r.Run(context.Background(), w, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, present := rep.Results[0].Meta["stderr"]; present {
		t.Errorf("Meta[\"stderr\"] should be absent when stderr is empty; got %#v", rep.Results[0].Meta["stderr"])
	}
}

func TestRunnerBashShellQuoteFunction(t *testing.T) {
	r := NewRunner(&fakeAI{})
	w := &Workflow{
		Name:    "shq",
		Version: "1",
		Inputs:  []InputSpec{{Name: "x", Required: true}},
		Nodes: []Node{
			{ID: "n", Kind: KindBash, Bash: &BashNode{Cmd: "printf '%s' {{shq .Inputs.x}}"}},
		},
	}
	rep, err := r.Run(context.Background(), w, map[string]string{"x": "a;b"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got := rep.Results[0].Output
	if got != "a;b" {
		t.Errorf("shq output = %q, want a;b", got)
	}
}

func TestEnvVarKey(t *testing.T) {
	// Case is preserved so shell var refs match the YAML-declared input name
	// exactly. Non-alphanumeric characters become underscores.
	cases := []struct{ in, want string }{
		{"foo", "INPUT_foo"},
		{"FOO", "INPUT_FOO"},
		{"my-key", "INPUT_my_key"},
		{"stage.output", "INPUT_stage_output"},
		{"repos_root", "INPUT_repos_root"},
		{"lookback_days", "INPUT_lookback_days"},
		{"MixedCase", "INPUT_MixedCase"},
		{"with9digit", "INPUT_with9digit"},
	}
	for _, tc := range cases {
		if got := envVarKey("INPUT_", tc.in); got != tc.want {
			t.Errorf("envVarKey(INPUT_, %q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestShellQuote(t *testing.T) {
	cases := []struct{ in, want string }{
		{"hello", "'hello'"},
		{"it's", "'it'\\''s'"},
		{"a;b", "'a;b'"},
		{"$(rm -rf /)", "'$(rm -rf /)'"},
	}
	for _, tc := range cases {
		if got := shellQuote(tc.in); got != tc.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRunnerCancelsOnContextDone(t *testing.T) {
	r := NewRunner(&fakeAI{})
	w := &Workflow{
		Name: "c", Version: "1",
		Nodes: []Node{
			{ID: "n", Kind: KindBash, Bash: &BashNode{Cmd: "sleep 5"}},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	_, err := r.Run(ctx, w, nil)
	if err == nil {
		t.Error("expected context-cancelled error")
	}
}
