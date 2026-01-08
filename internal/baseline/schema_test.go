package baseline

import (
	"testing"
	"time"
)

func TestNewBaseline(t *testing.T) {
	b := NewBaseline()

	if b.SchemaVersion != CurrentSchemaVersion {
		t.Errorf("schema_version = %q, want %q", b.SchemaVersion, CurrentSchemaVersion)
	}

	if b.Scenarios == nil {
		t.Error("scenarios map should be initialized")
	}

	if b.CreatedAt.IsZero() {
		t.Error("created_at should be set")
	}

	if b.UpdatedAt.IsZero() {
		t.Error("updated_at should be set")
	}
}

func TestNewScenarioBaseline(t *testing.T) {
	sb := NewScenarioBaseline("small")

	if sb.Scenario != "small" {
		t.Errorf("scenario = %q, want %q", sb.Scenario, "small")
	}

	if sb.Thresholds == nil {
		t.Fatal("thresholds should be initialized")
	}

	if sb.Thresholds.LocalMultiplier != 2.0 {
		t.Errorf("local_multiplier = %.2f, want 2.0", sb.Thresholds.LocalMultiplier)
	}

	if sb.Thresholds.CIPercentage != 15.0 {
		t.Errorf("ci_percentage = %.2f, want 15.0", sb.Thresholds.CIPercentage)
	}

	if sb.Thresholds.WarningCV != 20.0 {
		t.Errorf("warning_cv = %.2f, want 20.0", sb.Thresholds.WarningCV)
	}

	if sb.History == nil {
		t.Error("history should be initialized")
	}
}

func TestValidateSchema_Valid(t *testing.T) {
	b := NewBaseline()
	b.GitCommit = "abc123"
	b.GitBranch = "main"
	b.Scenarios["small"] = NewScenarioBaseline("small")
	b.Scenarios["small"].MedianMS = 10.5
	b.Scenarios["small"].Runs = 10

	err := ValidateSchema(b)
	if err != nil {
		t.Errorf("ValidateSchema() should succeed, got error: %v", err)
	}
}

func TestValidateSchema_NilBaseline(t *testing.T) {
	err := ValidateSchema(nil)
	if err == nil {
		t.Error("ValidateSchema(nil) should fail")
	}
}

func TestValidateSchema_MissingSchemaVersion(t *testing.T) {
	b := NewBaseline()
	b.SchemaVersion = ""

	err := ValidateSchema(b)
	if err == nil {
		t.Error("ValidateSchema() should fail with empty schema_version")
	}
}

func TestValidateSchema_UnsupportedVersion(t *testing.T) {
	b := NewBaseline()
	b.SchemaVersion = "2.0"

	err := ValidateSchema(b)
	if err == nil {
		t.Error("ValidateSchema() should fail with unsupported schema version")
	}
}

func TestValidateSchema_MissingCreatedAt(t *testing.T) {
	b := NewBaseline()
	b.CreatedAt = time.Time{}

	err := ValidateSchema(b)
	if err == nil {
		t.Error("ValidateSchema() should fail with missing created_at")
	}
}

func TestValidateSchema_NilScenarios(t *testing.T) {
	b := NewBaseline()
	b.Scenarios = nil

	err := ValidateSchema(b)
	if err == nil {
		t.Error("ValidateSchema() should fail with nil scenarios")
	}
}

func TestValidateSchema_NilScenario(t *testing.T) {
	b := NewBaseline()
	b.Scenarios["small"] = nil

	err := ValidateSchema(b)
	if err == nil {
		t.Error("ValidateSchema() should fail with nil scenario")
	}
}

func TestValidateSchema_EmptyScenarioName(t *testing.T) {
	b := NewBaseline()
	sb := NewScenarioBaseline("")
	sb.Runs = 10
	b.Scenarios["small"] = sb

	err := ValidateSchema(b)
	if err == nil {
		t.Error("ValidateSchema() should fail with empty scenario name")
	}
}

func TestValidateSchema_NegativeMedian(t *testing.T) {
	b := NewBaseline()
	sb := NewScenarioBaseline("small")
	sb.MedianMS = -10.0
	sb.Runs = 10
	b.Scenarios["small"] = sb

	err := ValidateSchema(b)
	if err == nil {
		t.Error("ValidateSchema() should fail with negative median")
	}
}

func TestValidateSchema_InvalidRuns(t *testing.T) {
	b := NewBaseline()
	sb := NewScenarioBaseline("small")
	sb.MedianMS = 10.0
	sb.Runs = 0
	b.Scenarios["small"] = sb

	err := ValidateSchema(b)
	if err == nil {
		t.Error("ValidateSchema() should fail with runs < 1")
	}
}

func TestValidateSchema_NilThresholds(t *testing.T) {
	b := NewBaseline()
	sb := NewScenarioBaseline("small")
	sb.MedianMS = 10.0
	sb.Runs = 10
	sb.Thresholds = nil
	b.Scenarios["small"] = sb

	err := ValidateSchema(b)
	if err == nil {
		t.Error("ValidateSchema() should fail with nil thresholds")
	}
}

func TestValidateThresholds_InvalidLocalMultiplier(t *testing.T) {
	b := NewBaseline()
	sb := NewScenarioBaseline("small")
	sb.MedianMS = 10.0
	sb.Runs = 10
	sb.Thresholds.LocalMultiplier = 0.5 // Invalid: <= 1.0
	b.Scenarios["small"] = sb

	err := ValidateSchema(b)
	if err == nil {
		t.Error("ValidateSchema() should fail with local_multiplier <= 1.0")
	}
}

func TestValidateThresholds_InvalidCIPercentage(t *testing.T) {
	tests := []struct {
		name       string
		percentage float64
	}{
		{"negative", -10.0},
		{"zero", 0.0},
		{"over 100", 150.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBaseline()
			sb := NewScenarioBaseline("small")
			sb.MedianMS = 10.0
			sb.Runs = 10
			sb.Thresholds.CIPercentage = tt.percentage
			b.Scenarios["small"] = sb

			err := ValidateSchema(b)
			if err == nil {
				t.Errorf("ValidateSchema() should fail with ci_percentage = %.2f", tt.percentage)
			}
		})
	}
}

func TestValidateThresholds_InvalidWarningCV(t *testing.T) {
	tests := []struct {
		name string
		cv   float64
	}{
		{"negative", -5.0},
		{"zero", 0.0},
		{"over 100", 200.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBaseline()
			sb := NewScenarioBaseline("small")
			sb.MedianMS = 10.0
			sb.Runs = 10
			sb.Thresholds.WarningCV = tt.cv
			b.Scenarios["small"] = sb

			err := ValidateSchema(b)
			if err == nil {
				t.Errorf("ValidateSchema() should fail with warning_cv = %.2f", tt.cv)
			}
		})
	}
}
