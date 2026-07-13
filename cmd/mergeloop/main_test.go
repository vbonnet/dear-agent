package main

import "testing"

func TestMergeLoopAgentRouteFlags(t *testing.T) {
	opts := options{mode: "tick"}
	fs := newMergeLoopFlagSet("tick", &opts)
	if err := fs.Parse([]string{"--enable-agents", "--agent-harness", "opencode-cli", "--agent-model", "deepseek-v4"}); err != nil {
		t.Fatal(err)
	}
	if !opts.enableAgents || opts.agentHarness != "opencode-cli" || opts.agentModel != "deepseek-v4" {
		t.Fatalf("parsed options = %#v", opts)
	}
}

func TestMergeLoopAgentRouteDefaults(t *testing.T) {
	opts := options{mode: "tick"}
	fs := newMergeLoopFlagSet("tick", &opts)
	if err := fs.Parse(nil); err != nil {
		t.Fatal(err)
	}
	if opts.agentHarness != "claude-code" || opts.agentModel != "" {
		t.Fatalf("default route = harness %q model %q", opts.agentHarness, opts.agentModel)
	}
}
