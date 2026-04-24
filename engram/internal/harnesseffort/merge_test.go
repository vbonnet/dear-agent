package harnesseffort

import (
	"reflect"
	"testing"
)

// makeConfig is a helper for building test configs concisely.
func makeConfig(opts ...func(*HarnessEffortConfig)) HarnessEffortConfig {
	cfg := HarnessEffortConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

func withAlias(k, v string) func(*HarnessEffortConfig) {
	return func(cfg *HarnessEffortConfig) {
		if cfg.ModelAliases == nil {
			cfg.ModelAliases = make(map[string]string)
		}
		cfg.ModelAliases[k] = v
	}
}

func withSubagent(pref string) func(*HarnessEffortConfig) {
	return func(cfg *HarnessEffortConfig) { cfg.SubagentPreference = pref }
}

func withTaskType(name string, order ...string) func(*HarnessEffortConfig) {
	return func(cfg *HarnessEffortConfig) {
		if cfg.TaskTypes == nil {
			cfg.TaskTypes = make(map[string]TaskTypeConfig)
		}
		cfg.TaskTypes[name] = TaskTypeConfig{HarnessOrder: order}
	}
}

func withTier(tierName, desc string, providers map[string]ProviderConfig) func(*HarnessEffortConfig) {
	return func(cfg *HarnessEffortConfig) {
		if cfg.Tiers == nil {
			cfg.Tiers = make(map[string]TierConfig)
		}
		cfg.Tiers[tierName] = TierConfig{Description: desc, Providers: providers}
	}
}

func provs(pairs ...string) map[string]ProviderConfig {
	m := make(map[string]ProviderConfig, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		m[pairs[i]] = ProviderConfig{Model: pairs[i+1]}
	}
	return m
}

// TestMergeConfigs_BaseOnly: merging empty src into base returns base unchanged.
func TestMergeConfigs_BaseOnly(t *testing.T) {
	base := makeConfig(
		withAlias("latest-opus", "claude-opus-4-5"),
		withSubagent("codex"),
		withTaskType("coding", "codex", "opencode"),
		withTier("high", "High effort", provs("anthropic", "claude-opus-4-5")),
	)
	empty := HarnessEffortConfig{}

	got := MergeConfigs(base, empty)

	if !reflect.DeepEqual(got, base) {
		t.Errorf("MergeConfigs(base, empty) = %+v, want %+v", got, base)
	}
}

// TestMergeConfigs_OverrideOneProvider: override single tier's provider; others unchanged.
func TestMergeConfigs_OverrideOneProvider(t *testing.T) {
	base := makeConfig(
		withTier("high", "High effort", map[string]ProviderConfig{
			"anthropic": {Model: "claude-opus-4-5", Effort: "high"},
			"openai":    {Model: "gpt-4o", Effort: "high"},
		}),
	)
	src := makeConfig(
		withTier("high", "", map[string]ProviderConfig{
			"anthropic": {Model: "claude-opus-5"},
		}),
	)

	got := MergeConfigs(base, src)

	highTier := got.Tiers["high"]
	if highTier.Providers["anthropic"].Model != "claude-opus-5" {
		t.Errorf("anthropic model = %q, want %q", highTier.Providers["anthropic"].Model, "claude-opus-5")
	}
	if highTier.Providers["openai"].Model != "gpt-4o" {
		t.Errorf("openai model = %q, want %q (should be unchanged)", highTier.Providers["openai"].Model, "gpt-4o")
	}
	if highTier.Providers["openai"].Effort != "high" {
		t.Errorf("openai effort = %q, want %q (should be unchanged)", highTier.Providers["openai"].Effort, "high")
	}
}

// TestMergeConfigs_CustomProvider: custom provider key preserved alongside built-ins.
func TestMergeConfigs_CustomProvider(t *testing.T) {
	base := makeConfig(
		withTier("medium", "Medium", provs("anthropic", "claude-sonnet-4-5")),
	)
	src := makeConfig(
		withTier("medium", "", map[string]ProviderConfig{
			"custom-llm": {Model: "my-model-v1", Effort: "medium"},
		}),
	)

	got := MergeConfigs(base, src)

	tier := got.Tiers["medium"]
	if _, ok := tier.Providers["anthropic"]; !ok {
		t.Error("anthropic provider missing after merge")
	}
	if _, ok := tier.Providers["custom-llm"]; !ok {
		t.Error("custom-llm provider missing after merge")
	}
	if tier.Providers["custom-llm"].Model != "my-model-v1" {
		t.Errorf("custom-llm model = %q, want %q", tier.Providers["custom-llm"].Model, "my-model-v1")
	}
}

// TestMergeConfigs_EmptySrc: empty src (zero value) returns copy of dst.
func TestMergeConfigs_EmptySrc(t *testing.T) {
	dst := makeConfig(
		withAlias("a", "b"),
		withSubagent("gemini"),
		withTaskType("research", "gemini", "codex"),
		withTier("low", "Low", provs("anthropic", "claude-haiku-4-5")),
	)
	src := HarnessEffortConfig{}

	got := MergeConfigs(dst, src)

	if !reflect.DeepEqual(got, dst) {
		t.Errorf("MergeConfigs(dst, empty) mutated result:\ngot  %+v\nwant %+v", got, dst)
	}
}

// TestMergeConfigs_ModelAliasOverride: src alias table keys win over dst.
func TestMergeConfigs_ModelAliasOverride(t *testing.T) {
	dst := makeConfig(
		withAlias("latest-opus", "claude-opus-4-5"),
		withAlias("latest-sonnet", "claude-sonnet-4-5"),
	)
	src := makeConfig(
		withAlias("latest-opus", "claude-opus-5"),
		withAlias("latest-haiku", "claude-haiku-5"),
	)

	got := MergeConfigs(dst, src)

	tests := []struct{ key, want string }{
		{"latest-opus", "claude-opus-5"},       // src overrides dst
		{"latest-sonnet", "claude-sonnet-4-5"}, // dst preserved
		{"latest-haiku", "claude-haiku-5"},     // src adds new key
	}
	for _, tt := range tests {
		if got.ModelAliases[tt.key] != tt.want {
			t.Errorf("ModelAliases[%q] = %q, want %q", tt.key, got.ModelAliases[tt.key], tt.want)
		}
	}
}

// TestMergeConfigs_SubagentPreference: non-empty src preference overwrites.
func TestMergeConfigs_SubagentPreference(t *testing.T) {
	t.Run("src_overwrites", func(t *testing.T) {
		dst := makeConfig(withSubagent("codex"))
		src := makeConfig(withSubagent("opencode"))
		got := MergeConfigs(dst, src)
		if got.SubagentPreference != "opencode" {
			t.Errorf("SubagentPreference = %q, want %q", got.SubagentPreference, "opencode")
		}
	})
	t.Run("empty_src_preserves_dst", func(t *testing.T) {
		dst := makeConfig(withSubagent("codex"))
		src := HarnessEffortConfig{}
		got := MergeConfigs(dst, src)
		if got.SubagentPreference != "codex" {
			t.Errorf("SubagentPreference = %q, want %q", got.SubagentPreference, "codex")
		}
	})
}

// TestMergeConfigs_TaskTypeHarnessOrder: src harness_order fully replaces (not appends).
func TestMergeConfigs_TaskTypeHarnessOrder(t *testing.T) {
	dst := makeConfig(withTaskType("coding", "codex", "opencode", "gemini"))
	src := makeConfig(withTaskType("coding", "gemini", "opencode"))

	got := MergeConfigs(dst, src)

	want := []string{"gemini", "opencode"}
	if !reflect.DeepEqual(got.TaskTypes["coding"].HarnessOrder, want) {
		t.Errorf("HarnessOrder = %v, want %v (should be full replacement)", got.TaskTypes["coding"].HarnessOrder, want)
	}
}

// TestResolveAliases_BasicSubstitution: latest-opus → concrete model.
func TestResolveAliases_BasicSubstitution(t *testing.T) {
	cfg := makeConfig(
		withAlias("latest-opus", "claude-opus-5"),
		withTier("high", "High", map[string]ProviderConfig{
			"anthropic": {Model: "latest-opus", Effort: "high"},
		}),
	)

	got := ResolveAliases(cfg)

	model := got.Tiers["high"].Providers["anthropic"].Model
	if model != "claude-opus-5" {
		t.Errorf("resolved model = %q, want %q", model, "claude-opus-5")
	}
	// Effort should be preserved
	effort := got.Tiers["high"].Providers["anthropic"].Effort
	if effort != "high" {
		t.Errorf("effort = %q, want %q (should be preserved)", effort, "high")
	}
}

// TestResolveAliases_UnknownAlias: passes through unchanged.
func TestResolveAliases_UnknownAlias(t *testing.T) {
	cfg := makeConfig(
		withAlias("latest-opus", "claude-opus-5"),
		withTier("high", "High", map[string]ProviderConfig{
			"anthropic": {Model: "some-unknown-alias"},
			"openai":    {Model: "gpt-5"},
		}),
	)

	got := ResolveAliases(cfg)

	if got.Tiers["high"].Providers["anthropic"].Model != "some-unknown-alias" {
		t.Errorf("unknown alias should pass through unchanged, got %q",
			got.Tiers["high"].Providers["anthropic"].Model)
	}
	if got.Tiers["high"].Providers["openai"].Model != "gpt-5" {
		t.Errorf("non-alias model should pass through unchanged, got %q",
			got.Tiers["high"].Providers["openai"].Model)
	}
}

// TestResolveAliases_NoAliases: empty aliases table, models unchanged.
func TestResolveAliases_NoAliases(t *testing.T) {
	cfg := makeConfig(
		withTier("medium", "Medium", map[string]ProviderConfig{
			"anthropic": {Model: "claude-sonnet-4-5"},
			"openai":    {Model: "gpt-4o"},
		}),
	)

	got := ResolveAliases(cfg)

	if got.Tiers["medium"].Providers["anthropic"].Model != "claude-sonnet-4-5" {
		t.Errorf("model changed unexpectedly: %q", got.Tiers["medium"].Providers["anthropic"].Model)
	}
	if got.Tiers["medium"].Providers["openai"].Model != "gpt-4o" {
		t.Errorf("model changed unexpectedly: %q", got.Tiers["medium"].Providers["openai"].Model)
	}
}

// TestMergeConfigs_NoMutation: verifies dst and src are not mutated by MergeConfigs.
func TestMergeConfigs_NoMutation(t *testing.T) {
	dst := makeConfig(
		withAlias("a", "original-a"),
		withTier("high", "High", provs("anthropic", "original-model")),
	)
	src := makeConfig(
		withAlias("a", "overridden-a"),
		withTier("high", "", provs("anthropic", "new-model")),
	)

	// Deep copy originals for comparison
	dstAlias := dst.ModelAliases["a"]
	dstModel := dst.Tiers["high"].Providers["anthropic"].Model
	srcAlias := src.ModelAliases["a"]

	MergeConfigs(dst, src)

	if dst.ModelAliases["a"] != dstAlias {
		t.Errorf("dst was mutated: ModelAliases[a] = %q, want %q", dst.ModelAliases["a"], dstAlias)
	}
	if dst.Tiers["high"].Providers["anthropic"].Model != dstModel {
		t.Errorf("dst was mutated: model = %q, want %q", dst.Tiers["high"].Providers["anthropic"].Model, dstModel)
	}
	if src.ModelAliases["a"] != srcAlias {
		t.Errorf("src was mutated: ModelAliases[a] = %q, want %q", src.ModelAliases["a"], srcAlias)
	}
}
