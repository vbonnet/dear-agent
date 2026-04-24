// Package events defines Wayfinder event schemas for EventBus integration.
// All events follow the pattern: wayfinder.<category>.<action>
//
// Categories:
//   - persona: Persona invocation and decision events
//   - phase: Phase lifecycle events
//   - feature: Feature lifecycle events
//   - w0: W0 project framing events
package events

// EventBase contains common fields for all Wayfinder events.
type EventBase struct {
	TraceID   string `json:"traceId"`
	Timestamp int64  `json:"timestamp"`
}

// PersonaInvokedEvent is emitted when a persona is invoked for review.
type PersonaInvokedEvent struct {
	EventBase
	PersonaID string `json:"personaId"`
	Tier      string `json:"tier"` // "core", "domain", "company", "team", "user"
	Phase     string `json:"phase,omitempty"`
	FeatureID string `json:"featureId,omitempty"`
}

// PersonaDecisionEvent is emitted when a persona completes review.
type PersonaDecisionEvent struct {
	EventBase
	PersonaID   string `json:"personaId"`
	Decision    string `json:"decision"` // "approve", "block", "warning"
	Score       int    `json:"score,omitempty"`
	Phase       string `json:"phase,omitempty"`
	FeatureID   string `json:"featureId,omitempty"`
	IssuesFound int    `json:"issuesFound,omitempty"`
}

// PhaseStartedEvent is emitted when a Wayfinder phase begins.
type PhaseStartedEvent struct {
	EventBase
	Phase         string `json:"phase"`
	FeatureID     string `json:"featureId"`
	PreviousPhase string `json:"previousPhase,omitempty"`
}

// PhaseCompletedEvent is emitted when a Wayfinder phase completes.
type PhaseCompletedEvent struct {
	EventBase
	Phase           string `json:"phase"`
	FeatureID       string `json:"featureId"`
	DurationMs      int64  `json:"durationMs"`
	Outcome         string `json:"outcome"` // "success", "failure", "partial", "skipped"
	PersonasInvoked int    `json:"personasInvoked,omitempty"`
	Approvals       int    `json:"approvals,omitempty"`
	Blocks          int    `json:"blocks,omitempty"`
	Warnings        int    `json:"warnings,omitempty"`
}

// FeatureStartedEvent is emitted when feature development begins.
type FeatureStartedEvent struct {
	EventBase
	FeatureID     string `json:"featureId"`
	Title         string `json:"title"`
	StartingPhase string `json:"startingPhase,omitempty"`
}

// FeatureCompletedEvent is emitted when feature development completes.
type FeatureCompletedEvent struct {
	EventBase
	FeatureID            string   `json:"featureId"`
	TotalDurationMs      int64    `json:"totalDurationMs"`
	PhasesCompleted      int      `json:"phasesCompleted"`
	Outcome              string   `json:"outcome"` // "success", "failure", "abandoned"
	TotalPersonasInvoked int      `json:"totalPersonasInvoked,omitempty"`
	TotalIssuesFound     int      `json:"totalIssuesFound,omitempty"`
	SkippedPhases        []string `json:"skippedPhases,omitempty"`
}

// GuidanceOptimizedEvent is emitted when guidance is optimized.
type GuidanceOptimizedEvent struct {
	EventBase
	GuidanceFile     string `json:"guidanceFile"`
	OptimizationType string `json:"optimizationType"` // "auto-promotion", "frequency-boost", "pattern-match"
	OldValue         string `json:"oldValue,omitempty"`
	NewValue         string `json:"newValue,omitempty"`
	Reason           string `json:"reason,omitempty"`
}

// W0StartedEvent is emitted when W0 project framing begins.
type W0StartedEvent struct {
	EventBase
	SessionID               string   `json:"sessionId"`
	ProjectPath             string   `json:"projectPath"`
	TriggeredBy             string   `json:"triggeredBy"` // "vague_request", "manual"
	InitialRequest          string   `json:"initialRequest"`
	InitialRequestWordCount int      `json:"initialRequestWordCount"`
	VaguenessSignals        []string `json:"vaguenessSignals"`
	VaguenessScore          float64  `json:"vaguenessScore"`
}

// W0QuestionAskedEvent is emitted when a question is asked.
type W0QuestionAskedEvent struct {
	EventBase
	SessionID              string   `json:"sessionId"`
	Round                  int      `json:"round"`
	QuestionNumber         int      `json:"questionNumber"`
	QuestionType           string   `json:"questionType"`
	QuestionText           string   `json:"questionText"`
	IncludesMultipleChoice bool     `json:"includesMultipleChoice"`
	MultipleChoiceOptions  []string `json:"multipleChoiceOptions,omitempty"`
}

// W0UserResponseEvent is emitted when user responds.
type W0UserResponseEvent struct {
	EventBase
	SessionID              string   `json:"sessionId"`
	Round                  int      `json:"round"`
	QuestionNumber         int      `json:"questionNumber"`
	QuestionType           string   `json:"questionType"`
	ResponseType           string   `json:"responseType"` // "multiple_choice", "free_text", "both"
	MultipleChoiceSelected string   `json:"multipleChoiceSelected,omitempty"`
	FreeTextLength         int      `json:"freeTextLength"`
	FreeTextWordCount      int      `json:"freeTextWordCount"`
	TechnicalTermsDetected []string `json:"technicalTermsDetected"`
	ClarityScore           float64  `json:"clarityScore"`
	ResponseTime           float64  `json:"responseTime"`
}

// W0CharterSynthesizedEvent is emitted when charter synthesis completes.
type W0CharterSynthesizedEvent struct {
	EventBase
	SessionID               string   `json:"sessionId"`
	Round                   int      `json:"round"`
	SynthesisMethod         string   `json:"synthesisMethod"` // "simple", "few_shot_cot"
	SynthesisTimeMs         int64    `json:"synthesisTimeMs"`
	CharterWordCount        int      `json:"charterWordCount"`
	CharterSections         []string `json:"charterSections"`
	MissingRequiredSections []string `json:"missingRequiredSections"`
	ValidationPassed        bool     `json:"validationPassed"`
	SynthesisAttempt        int      `json:"synthesisAttempt"`
}

// W0ApprovedEvent is emitted when user approves the charter.
type W0ApprovedEvent struct {
	EventBase
	SessionID     string `json:"sessionId"`
	Round         int    `json:"round"`
	UserAction    string `json:"userAction"` // "approve", "edit", "ask_more"
	EditRequested bool   `json:"editRequested"`
	EditCount     int    `json:"editCount"`
	FinalApproval bool   `json:"finalApproval"`
}

// W0CompletedEvent is emitted when W0 process completes.
type W0CompletedEvent struct {
	EventBase
	SessionID                 string  `json:"sessionId"`
	TotalRounds               int     `json:"totalRounds"`
	TotalQuestionsAsked       int     `json:"totalQuestionsAsked"`
	TotalDurationMs           int64   `json:"totalDurationMs"`
	CharterSaved              bool    `json:"charterSaved"`
	CharterPath               string  `json:"charterPath"`
	CharterQualityEstimate    float64 `json:"charterQualityEstimate"`
	ProceedingToPhase         string  `json:"proceedingToPhase"`
	UserSatisfactionIndicator *int    `json:"userSatisfactionIndicator"`
}

// ValidPhases lists all valid Wayfinder phase names.
var ValidPhases = []string{
	"D1-problem-validation", "D2-existing-solutions", "D3-approach-decision",
	"D4-solution-requirements", "S4-stakeholder-alignment", "S5-research",
	"S6-design", "S7-plan", "S8-implementation", "S9-validation",
	"S10-deploy", "S11-retrospective",
}

// Event name constants.
const (
	EventPersonaInvoked       = "wayfinder.persona.invoked"
	EventPersonaDecision      = "wayfinder.persona.decision"
	EventPhaseStarted         = "wayfinder.phase.started"
	EventPhaseCompleted       = "wayfinder.phase.completed"
	EventFeatureStarted       = "wayfinder.feature.started"
	EventFeatureCompleted     = "wayfinder.feature.completed"
	EventGuidanceOptimized    = "wayfinder.guidance.optimized"
	EventW0Started            = "wayfinder.w0.started"
	EventW0QuestionAsked      = "wayfinder.w0.question_asked"
	EventW0UserResponse       = "wayfinder.w0.user_response"
	EventW0CharterSynthesized = "wayfinder.w0.charter_synthesized"
	EventW0Approved           = "wayfinder.w0.approved"
	EventW0Completed          = "wayfinder.w0.completed"
)
