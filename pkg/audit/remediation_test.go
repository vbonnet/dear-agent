package audit

import "testing"

func TestRemediationValidateAcceptsOptionalContext(t *testing.T) {
	for _, remediation := range []Remediation{
		{},
		{Strategy: StrategyAuto},
		{Strategy: StrategyPR},
		{Strategy: StrategyIssue},
		{Strategy: StrategyNoop},
		{Strategy: StrategyPR, Title: "Investigate", Body: "Details"},
	} {
		if err := remediation.Validate(); err != nil {
			t.Errorf("Validate(%+v): %v", remediation, err)
		}
	}
	if err := (Remediation{Strategy: Strategy("future")}).Validate(); err == nil {
		t.Fatal("Validate accepted an unknown strategy")
	}
}
