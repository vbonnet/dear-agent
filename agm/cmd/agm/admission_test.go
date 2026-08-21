package main

import "testing"

// Pi authenticates independently of the CLI-subscription accounts CodexBar
// meters (agm/docs/PI-HARNESS.md), so the provider-quota gate must not
// attribute a Pi spawn to whatever family its model alias happens to
// resolve to — that would gate (or admit) it on an unrelated subscription's
// headroom (codex review on #1218).
func TestProviderQuotaFamilyResolverLeavesPiUnmapped(t *testing.T) {
	for _, harness := range []string{"pi", "pi-cli", "Pi-CLI"} {
		if got := providerQuotaFamilyResolver(harness)("sonnet"); got != "" {
			t.Errorf("providerQuotaFamilyResolver(%q)(\"sonnet\") = %q, want \"\" (Pi's billing identity is not established)", harness, got)
		}
	}
}

// Every other harness must keep resolving normally — this is a narrow
// exception for Pi's documented independent credentials, not a general
// weakening of the gate.
func TestProviderQuotaFamilyResolverResolvesOtherHarnessesNormally(t *testing.T) {
	tests := []struct {
		harness string
		model   string
		want    string
	}{
		{harness: "claude-code", model: "sonnet", want: "anthropic"},
		{harness: "codex-cli", model: "5.5", want: "openai"},
	}
	for _, tt := range tests {
		if got := providerQuotaFamilyResolver(tt.harness)(tt.model); got != tt.want {
			t.Errorf("providerQuotaFamilyResolver(%q)(%q) = %q, want %q", tt.harness, tt.model, got, tt.want)
		}
	}
}
