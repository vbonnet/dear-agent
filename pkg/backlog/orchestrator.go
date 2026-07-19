package backlog

import (
	"time"

	"github.com/google/uuid"

	"github.com/vbonnet/dear-agent/pkg/vroom/vroom"
)

// DefaultOrchestratorRole is the VROOM role identity a backlog suggestion
// is published under.
const DefaultOrchestratorRole = "orchestrator"

// OrchestratorNotifier records a backlog pickup as a VROOM
// decision.dispatched event so the choice lands on the decision trail
// (ADR-022) so decision-trail consumers can see it.
//
// It publishes synchronously and deliberately does NOT use vroom.Emitter:
// Emitter.emit spawns a goroutine and a short-lived CLI would os.Exit
// before it flushes, dropping the audit row. Synchronous publish is the
// correct choice for a fire-once CLI and matches selfimprove.VROOMNotifier.
type OrchestratorNotifier struct {
	pub    vroom.EventPublisher
	role   string
	worker string
}

// NewOrchestratorNotifier wraps a vroom.EventPublisher. A nil publisher
// turns Dispatch into a no-op (same contract as selfimprove.VROOMNotifier),
// so callers need not branch on whether eventing is configured.
func NewOrchestratorNotifier(pub vroom.EventPublisher) *OrchestratorNotifier {
	return &OrchestratorNotifier{pub: pub, role: DefaultOrchestratorRole}
}

// WithWorker sets the worker hint recorded on the dispatch event (e.g. the
// harness or role that will pick the item up). Returns the notifier for
// chaining.
func (n *OrchestratorNotifier) WithWorker(worker string) *OrchestratorNotifier {
	n.worker = worker
	return n
}

// Dispatch publishes a vroom.decision.dispatched event for the chosen
// suggestion. The payload is the canonical vroom.DispatchedPayload plus the
// same event_id/role/timestamp envelope vroom.Emitter adds, so existing
// decision-trail consumers cannot tell the two apart. A nil publisher is a
// no-op returning nil.
func (n *OrchestratorNotifier) Dispatch(s Suggestion) error {
	if n.pub == nil {
		return nil
	}
	data := map[string]interface{}{
		"session_id": "",
		"task_id":    s.Item.ID,
		"worker":     n.worker,
		"rationale":  dispatchRationale(s),
		"event_id":   uuid.New().String(),
		"role":       n.role,
		"timestamp":  time.Now().UTC().Format(time.RFC3339Nano),
	}
	return n.pub.Publish(vroom.TopicDecisionDispatched, data)
}

// dispatchRationale renders a human-readable reason for the decision trail.
func dispatchRationale(s Suggestion) string {
	return s.Item.Title + " — " + s.Reason
}
