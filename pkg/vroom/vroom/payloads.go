package vroom

// DispatchedPayload is the payload for TopicDecisionDispatched events.
type DispatchedPayload struct {
	SessionID string `json:"session_id"`
	TaskID    string `json:"task_id"`
	Worker    string `json:"worker"`
	Rationale string `json:"rationale"`
}

// EscalatedPayload is the payload for TopicDecisionEscalated events.
type EscalatedPayload struct {
	SessionID string `json:"session_id"`
	Anomaly   string `json:"anomaly"`
	Severity  string `json:"severity"`
	Rationale string `json:"rationale"`
}

// EvaluatedPayload is the payload for TopicDecisionEvaluated events.
type EvaluatedPayload struct {
	SessionID string `json:"session_id"`
	OutputRef string `json:"output_ref"`
	Passed    bool   `json:"passed"`
	Rationale string `json:"rationale"`
}

// GatedPayload is the payload for TopicDecisionGated events.
type GatedPayload struct {
	FromState string `json:"from_state"`
	ToState   string `json:"to_state"`
	Approved  bool   `json:"approved"`
	Rationale string `json:"rationale"`
}

// HandedOffPayload is the payload for TopicDecisionHandedOff events. It
// records who handed context to whom and — crucially — how much the
// sender trusted that context, so a reviewer can later find every
// low-confidence handoff in the append-only decision trail.
type HandedOffPayload struct {
	SessionID       string   `json:"session_id"`
	FromRole        string   `json:"from_role"`
	ToRole          string   `json:"to_role"`
	ConfidenceLevel string   `json:"confidence_level"`
	ConfidenceScore float64  `json:"confidence_score"`
	Rationale       string   `json:"rationale"`
	Gaps            []string `json:"gaps,omitempty"`
}
