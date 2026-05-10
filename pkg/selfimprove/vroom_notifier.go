package selfimprove

import (
	"github.com/vbonnet/dear-agent/pkg/vroom/vroom"
)

// VROOMNotifier publishes self-improvement phase events through the
// pkg/vroom event bus. The topic is "selfimprove.<phase>" so that a
// supervisor subscribed to "selfimprove.*" sees the entire loop lifecycle.
type VROOMNotifier struct {
	publisher vroom.EventPublisher
	role      string
}

// NewVROOMNotifier wraps a vroom.EventPublisher. role identifies the loop
// (e.g. "selfimprove-swe-lite") and is included in every published event.
func NewVROOMNotifier(p vroom.EventPublisher, role string) *VROOMNotifier {
	return &VROOMNotifier{publisher: p, role: role}
}

// Phase publishes a single phase event. Errors are dropped — the notifier
// is observability, not control flow.
func (n *VROOMNotifier) Phase(name string, fields map[string]any) {
	if n.publisher == nil {
		return
	}
	payload := make(map[string]interface{}, len(fields)+2)
	for k, v := range fields {
		payload[k] = v
	}
	payload["role"] = n.role
	payload["phase"] = name
	_ = n.publisher.Publish("selfimprove."+name, payload)
}
