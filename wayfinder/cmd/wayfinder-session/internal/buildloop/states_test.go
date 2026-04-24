package buildloop

import (
	"testing"
)

func TestStateIsValid(t *testing.T) {
	tests := []struct {
		name  string
		state State
		want  bool
	}{
		{"valid TEST_FIRST", StateTestFirst, true},
		{"valid CODING", StateCoding, true},
		{"valid GREEN", StateGreen, true},
		{"valid REFACTOR", StateRefactor, true},
		{"valid VALIDATION", StateValidation, true},
		{"valid DEPLOY", StateDeploy, true},
		{"valid MONITORING", StateMonitoring, true},
		{"valid COMPLETE", StateComplete, true},
		{"valid TIMEOUT", StateTimeout, true},
		{"valid REVIEW_FAILED", StateReviewFailed, true},
		{"valid INTEGRATE_FAIL", StateIntegrateFail, true},
		{"invalid state", State("INVALID"), false},
		{"empty state", State(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.IsValid(); got != tt.want {
				t.Errorf("State.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStateIsErrorState(t *testing.T) {
	tests := []struct {
		name  string
		state State
		want  bool
	}{
		{"TIMEOUT is error", StateTimeout, true},
		{"REVIEW_FAILED is error", StateReviewFailed, true},
		{"INTEGRATE_FAIL is error", StateIntegrateFail, true},
		{"TEST_FIRST not error", StateTestFirst, false},
		{"CODING not error", StateCoding, false},
		{"GREEN not error", StateGreen, false},
		{"COMPLETE not error", StateComplete, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.IsErrorState(); got != tt.want {
				t.Errorf("State.IsErrorState() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStateIsTerminal(t *testing.T) {
	tests := []struct {
		name  string
		state State
		want  bool
	}{
		{"COMPLETE is terminal", StateComplete, true},
		{"TEST_FIRST not terminal", StateTestFirst, false},
		{"CODING not terminal", StateCoding, false},
		{"GREEN not terminal", StateGreen, false},
		{"TIMEOUT not terminal", StateTimeout, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.IsTerminal(); got != tt.want {
				t.Errorf("State.IsTerminal() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateTransition(t *testing.T) {
	tests := []struct {
		name      string
		from      State
		to        State
		wantValid bool
	}{
		// Valid transitions
		{"TEST_FIRST to CODING", StateTestFirst, StateCoding, true},
		{"TEST_FIRST to TIMEOUT", StateTestFirst, StateTimeout, true},
		{"CODING to GREEN", StateCoding, StateGreen, true},
		{"CODING to TEST_FIRST", StateCoding, StateTestFirst, true},
		{"GREEN to REFACTOR", StateGreen, StateRefactor, true},
		{"GREEN to VALIDATION", StateGreen, StateValidation, true},
		{"GREEN to COMPLETE", StateGreen, StateComplete, true},
		{"REFACTOR to VALIDATION", StateRefactor, StateValidation, true},
		{"VALIDATION to COMPLETE", StateValidation, StateComplete, true},
		{"VALIDATION to REVIEW_FAILED", StateValidation, StateReviewFailed, true},
		{"COMPLETE to TEST_FIRST", StateComplete, StateTestFirst, true},
		{"TIMEOUT to CODING", StateTimeout, StateCoding, true},

		// Invalid transitions
		{"TEST_FIRST to GREEN", StateTestFirst, StateGreen, false},
		{"TEST_FIRST to COMPLETE", StateTestFirst, StateComplete, false},
		{"CODING to VALIDATION", StateCoding, StateValidation, false},
		{"GREEN to TIMEOUT", StateGreen, StateTimeout, false},
		{"COMPLETE to CODING", StateComplete, StateCoding, false},
		{"COMPLETE to GREEN", StateComplete, StateGreen, false},

		// Invalid states
		{"invalid from state", State("INVALID"), StateTestFirst, false},
		{"invalid to state", StateTestFirst, State("INVALID"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateTransition(tt.from, tt.to)
			if got.Valid != tt.wantValid {
				t.Errorf("ValidateTransition(%v, %v).Valid = %v, want %v (reason: %s)",
					tt.from, tt.to, got.Valid, tt.wantValid, got.Reason)
			}
		})
	}
}

func TestGetExitCriteria(t *testing.T) {
	tests := []struct {
		name  string
		state State
	}{
		{"TEST_FIRST criteria", StateTestFirst},
		{"CODING criteria", StateCoding},
		{"GREEN criteria", StateGreen},
		{"REFACTOR criteria", StateRefactor},
		{"VALIDATION criteria", StateValidation},
		{"DEPLOY criteria", StateDeploy},
		{"MONITORING criteria", StateMonitoring},
		{"COMPLETE criteria", StateComplete},
		{"error state criteria", StateTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			criteria := GetExitCriteria(tt.state)
			if criteria.State != tt.state {
				t.Errorf("GetExitCriteria(%v).State = %v, want %v",
					tt.state, criteria.State, tt.state)
			}
			if criteria.Description == "" {
				t.Errorf("GetExitCriteria(%v).Description is empty", tt.state)
			}
			if criteria.Validator == nil {
				t.Errorf("GetExitCriteria(%v).Validator is nil", tt.state)
			}
		})
	}
}

func TestValidateTestFirstExit(t *testing.T) {
	tests := []struct {
		name    string
		ctx     *BuildContext
		wantOk  bool
		wantErr bool
	}{
		{
			name: "valid - tests failing",
			ctx: &BuildContext{
				TestResult: &TestResult{
					HasFailures:  true,
					FailureCount: 1,
				},
			},
			wantOk:  true,
			wantErr: false,
		},
		{
			name: "invalid - no test results",
			ctx: &BuildContext{
				TestResult: nil,
			},
			wantOk:  false,
			wantErr: true,
		},
		{
			name: "invalid - TDD violation (tests passing)",
			ctx: &BuildContext{
				TestResult: &TestResult{
					HasFailures: false,
				},
			},
			wantOk:  false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, err := validateTestFirstExit(tt.ctx)
			if ok != tt.wantOk {
				t.Errorf("validateTestFirstExit() ok = %v, want %v", ok, tt.wantOk)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTestFirstExit() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateCodingExit(t *testing.T) {
	tests := []struct {
		name    string
		ctx     *BuildContext
		wantOk  bool
		wantErr bool
	}{
		{
			name: "valid - code changes made",
			ctx: &BuildContext{
				CodeChanges: 5,
			},
			wantOk:  true,
			wantErr: false,
		},
		{
			name: "invalid - no code changes",
			ctx: &BuildContext{
				CodeChanges: 0,
			},
			wantOk:  false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, err := validateCodingExit(tt.ctx)
			if ok != tt.wantOk {
				t.Errorf("validateCodingExit() ok = %v, want %v", ok, tt.wantOk)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("validateCodingExit() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateGreenExit(t *testing.T) {
	tests := []struct {
		name    string
		ctx     *BuildContext
		wantOk  bool
		wantErr bool
	}{
		{
			name: "valid - tests pass and quality evaluated",
			ctx: &BuildContext{
				TestResult: &TestResult{
					HasFailures: false,
					PassCount:   5,
				},
				QualityResult: &QualityResult{
					Passes:           true,
					AssertionDensity: 0.6,
					CoveragePercent:  85.0,
				},
			},
			wantOk:  true,
			wantErr: false,
		},
		{
			name: "invalid - tests failing",
			ctx: &BuildContext{
				TestResult: &TestResult{
					HasFailures: true,
				},
				QualityResult: &QualityResult{
					Passes: true,
				},
			},
			wantOk:  false,
			wantErr: true,
		},
		{
			name: "invalid - no quality result",
			ctx: &BuildContext{
				TestResult: &TestResult{
					HasFailures: false,
				},
				QualityResult: nil,
			},
			wantOk:  false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, err := validateGreenExit(tt.ctx)
			if ok != tt.wantOk {
				t.Errorf("validateGreenExit() ok = %v, want %v", ok, tt.wantOk)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("validateGreenExit() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRiskLevelRequiresPerTaskReview(t *testing.T) {
	tests := []struct {
		name      string
		riskLevel RiskLevel
		want      bool
	}{
		{"XS no review", RiskXS, false},
		{"S no review", RiskS, false},
		{"M no review", RiskM, false},
		{"L requires review", RiskL, true},
		{"XL requires review", RiskXL, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.riskLevel.RequiresPerTaskReview(); got != tt.want {
				t.Errorf("RiskLevel.RequiresPerTaskReview() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStateTransitionsCoverage(t *testing.T) {
	// Ensure all states have defined transitions
	expectedStates := []State{
		StateTestFirst,
		StateCoding,
		StateGreen,
		StateRefactor,
		StateValidation,
		StateDeploy,
		StateMonitoring,
		StateComplete,
		StateTimeout,
		StateReviewFailed,
		StateIntegrateFail,
	}

	for _, state := range expectedStates {
		t.Run(string(state), func(t *testing.T) {
			transitions, exists := StateTransitions[state]
			if !exists {
				t.Errorf("No transitions defined for state %v", state)
				return
			}
			if len(transitions) == 0 {
				t.Errorf("State %v has zero allowed transitions", state)
			}
		})
	}
}

func TestTransitionSymmetry(t *testing.T) {
	// Test important bi-directional transitions
	tests := []struct {
		name string
		from State
		to   State
		back bool // whether reverse should also be valid
	}{
		{"TEST_FIRST <-> CODING", StateTestFirst, StateCoding, true},
		{"CODING <-> GREEN", StateCoding, StateGreen, false}, // GREEN doesn't go back to CODING directly
		{"GREEN -> REFACTOR (one-way)", StateGreen, StateRefactor, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Forward transition
			fwd := ValidateTransition(tt.from, tt.to)
			if !fwd.Valid {
				t.Errorf("Forward transition %v -> %v should be valid", tt.from, tt.to)
			}

			// Reverse transition
			rev := ValidateTransition(tt.to, tt.from)
			if tt.back && !rev.Valid {
				t.Errorf("Reverse transition %v -> %v should be valid", tt.to, tt.from)
			}
			if !tt.back && rev.Valid {
				t.Logf("Note: Reverse transition %v -> %v is valid (asymmetric)", tt.to, tt.from)
			}
		})
	}
}
