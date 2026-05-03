package interrupt

import (
	"fmt"
	"log/slog"
)

// FrictionLevel indicates the required friction for an interrupt
type FrictionLevel int

const (
	// FrictionFree — 1st interrupt per session: just log it
	FrictionFree FrictionLevel = iota
	// FrictionReason — 2nd+ interrupt: require --reason flag
	FrictionReason
	// FrictionWarn — 3rd+ interrupt: require --reason, log prominent warning
	FrictionWarn
)

// CheckFriction returns the friction level for the next interrupt to a recipient
func CheckFriction(recipient string) FrictionLevel {
	count := GetInterruptCount(recipient)
	switch count {
	case 0:
		return FrictionFree
	case 1:
		return FrictionReason
	default:
		return FrictionWarn
	}
}

// EnforceFriction checks friction level and returns an error if the interrupt
// should be blocked. Returns nil if the interrupt is allowed to proceed.
func EnforceFriction(recipient, reason string) error {
	level := CheckFriction(recipient)
	count := GetInterruptCount(recipient)

	switch level {
	case FrictionFree:
		return nil
	case FrictionReason:
		if reason == "" {
			return fmt.Errorf("this is interrupt #%d for session '%s'.\n"+
				"--reason is required: agm send msg %s --emergency-interrupt --reason \"why\" --prompt \"...\"",
				count+1, recipient, recipient)
		}
		return nil
	case FrictionWarn:
		if reason == "" {
			return fmt.Errorf("this is interrupt #%d for session '%s'.\n"+
				"--reason is required: agm send msg %s --emergency-interrupt --reason \"why\" --prompt \"...\"",
				count+1, recipient, recipient)
		}
		slog.Warn("high interrupt frequency",
			"interrupt_number", count+1, "session", recipient,
			"reason", reason, "total_interrupts", count)
		return nil
	}
	return nil
}
