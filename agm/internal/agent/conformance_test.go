package agent

import "testing"

func TestActiveHarnessesSatisfyConformanceSuite(t *testing.T) {
	t.Parallel()
	for _, finding := range ValidateActiveHarnessConformance() {
		t.Error(finding.Error())
	}
}

func TestHarnessConformanceReportsBrokenAdapter(t *testing.T) {
	t.Parallel()
	broken := &mockHarness{
		name:         "wrong",
		version:      "",
		capabilities: Capabilities{},
	}
	findings := ValidateHarnessConformance("claude-code", broken)
	if len(findings) == 0 {
		t.Fatal("expected conformance findings for broken adapter")
	}
	wantRequirements := map[string]bool{
		"canonical name": false,
		"version":        false,
		"capabilities":   false,
	}
	for _, finding := range findings {
		if _, ok := wantRequirements[finding.Requirement]; ok {
			wantRequirements[finding.Requirement] = true
		}
	}
	for requirement, found := range wantRequirements {
		if !found {
			t.Fatalf("missing conformance finding for %q in %v", requirement, findings)
		}
	}
}
