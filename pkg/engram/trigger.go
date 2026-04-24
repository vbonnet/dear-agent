package engram

// TriggerSpec defines when an engram should be injected into context.
type TriggerSpec struct {
	// On is the event type that activates this trigger.
	// Values: "phase.started", "phase.completed", "task.assigned", "task.started", "event"
	On string `yaml:"on"`

	// Match defines conditions that must be true for the trigger to fire.
	Match map[string]interface{} `yaml:"match,omitempty"`

	// Scope limits where this trigger is active: "global", "project", "session"
	Scope string `yaml:"scope,omitempty"`

	// Priority influences injection order. Higher = injected first. Default: 50.
	Priority int `yaml:"priority,omitempty"`

	// Cooldown prevents re-injection within this duration (e.g., "1h", "30m").
	Cooldown string `yaml:"cooldown,omitempty"`
}
