package commands

import "testing"

func TestValidateStartMetadataRejectsInvalidEnums(t *testing.T) {
	for _, test := range []struct {
		projectType string
		riskLevel   string
	}{
		{projectType: "typo", riskLevel: "M"},
		{projectType: "feature", riskLevel: "typo"},
	} {
		if err := validateStartMetadata(test.projectType, test.riskLevel); err == nil {
			t.Fatalf("validateStartMetadata(%q, %q) accepted invalid input", test.projectType, test.riskLevel)
		}
	}
	if err := validateStartMetadata("feature", "M"); err != nil {
		t.Fatalf("valid start metadata rejected: %v", err)
	}
}

func TestPhaseOutcomeValidation(t *testing.T) {
	if !validPhaseOutcome("success") || !validPhaseOutcome("partial") || !validPhaseOutcome("skipped") {
		t.Fatal("documented outcomes must be valid")
	}
	if validPhaseOutcome("typo") {
		t.Fatal("invalid outcome accepted")
	}
}

func TestLifecycleMetadataRequiresActionableDetails(t *testing.T) {
	for _, test := range []struct {
		state, blockedOn, errorMessage, inputNeeded string
		wantErr                                     bool
	}{
		{state: "input-required", wantErr: true},
		{state: "input-required", inputNeeded: "choose API"},
		{state: "dependency-blocked", wantErr: true},
		{state: "dependency-blocked", blockedOn: "worker-1"},
		{state: "failed", wantErr: true},
		{state: "failed", errorMessage: "build failed"},
	} {
		err := validateLifecycleMetadata(test.state, test.blockedOn, test.errorMessage, test.inputNeeded)
		if (err != nil) != test.wantErr {
			t.Fatalf("validateLifecycleMetadata(%q) error = %v, wantErr=%v", test.state, err, test.wantErr)
		}
	}
}
