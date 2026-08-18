package messages

import "fmt"

// Priority is the delivery order assigned to a queued message.
type Priority string

// Queue priority values, ordered from most to least urgent.
const (
	PriorityCritical Priority = "CRITICAL"
	PriorityHigh     Priority = "HIGH"
	PriorityMedium   Priority = "MEDIUM"
	PriorityLow      Priority = "LOW"
)

// IsValid reports whether p is a declared queue priority.
func (p Priority) IsValid() bool {
	switch p {
	case PriorityCritical, PriorityHigh, PriorityMedium, PriorityLow:
		return true
	default:
		return false
	}
}

// ParsePriority converts an exact queue wire value to Priority.
func ParsePriority(raw string) (Priority, error) {
	priority := Priority(raw)
	if !priority.IsValid() {
		return "", fmt.Errorf("invalid message priority %q (valid: CRITICAL, HIGH, MEDIUM, LOW)", raw)
	}

	return priority, nil
}
