package comparison

// ComparisonResult contains the result of comparing benchmark against baseline
type ComparisonResult struct {
	Scenario           string  `json:"scenario"`
	BaselineMedianMS   float64 `json:"baseline_median_ms"`
	CurrentMedianMS    float64 `json:"current_median_ms"`
	ChangePercent      float64 `json:"change_percent"`       // (current - baseline) / baseline * 100
	ChangeMultiplier   float64 `json:"change_multiplier"`    // current / baseline
	LocalThreshold     float64 `json:"local_threshold"`      // multiplier threshold (e.g., 2.0)
	CIThreshold        float64 `json:"ci_threshold"`         // percentage threshold (e.g., 15.0)
	IsRegression       bool    `json:"is_regression"`        // true if exceeds any threshold
	LocalViolation     bool    `json:"local_violation"`      // true if exceeds local threshold
	CIViolation        bool    `json:"ci_violation"`         // true if exceeds CI threshold
	HighVariance       bool    `json:"high_variance"`        // true if CV% exceeds warning threshold
	CurrentCVPercent   float64 `json:"current_cv_percent"`   // current coefficient of variation
	WarningCVThreshold float64 `json:"warning_cv_threshold"` // CV% warning threshold
}

// DetectionMode specifies which thresholds to apply
type DetectionMode string

const (
	// ModeLocal uses local multiplier threshold (e.g., >2x blocks commit)
	ModeLocal DetectionMode = "local"

	// ModeCI uses CI percentage threshold (e.g., >15% fails CI)
	ModeCI DetectionMode = "ci"

	// ModeBoth checks both local and CI thresholds
	ModeBoth DetectionMode = "both"
)
