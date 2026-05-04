// Package capacity provides capacity-related functionality.
package capacity

import (
	"fmt"
	"os"
	"strconv"
)

const (
	// DefaultReservedRAMBytes is RAM reserved for the OS and other processes (4 GB).
	DefaultReservedRAMBytes uint64 = 4 * 1024 * 1024 * 1024

	// DefaultPerSessionRAMBytes is estimated RAM per Claude session (800 MB).
	DefaultPerSessionRAMBytes uint64 = 800 * 1024 * 1024

	// DefaultHardCap is the absolute maximum number of concurrent sessions.
	DefaultHardCap int = 20
)

// Zone represents a capacity zone indicating system pressure.
type Zone string

// Capacity zone values describing system pressure.
const (
	ZoneGreen    Zone = "GREEN"    // <60% RAM usage — launch freely
	ZoneYellow   Zone = "YELLOW"   // 60-80% — be cautious
	ZoneRed      Zone = "RED"      // >80% — stop launching
	ZoneCritical Zone = "CRITICAL" // >90% — alert human
)

// CapacityResult holds the output of a capacity calculation.
type CapacityResult struct {
	MaxSessions     int     // Recommended maximum concurrent sessions
	CurrentSessions int     // Currently running Claude sessions
	AvailableSlots  int     // MaxSessions - CurrentSessions (floored at 0)
	Zone            Zone    // Current capacity zone
	RAMUsagePercent float64 // Current RAM usage percentage
	RAMBasedMax     int     // Max sessions based on RAM alone
	CPUBasedMax     int     // Max sessions based on CPU alone
	HardCap         int     // Absolute hard cap applied
	EnvOverride     int     // Value from AGM_MAX_SESSIONS (0 = not set)
}

// Calculator computes session capacity from system info.
type Calculator struct {
	ReservedRAMBytes   uint64
	PerSessionRAMBytes uint64
	HardCap            int
}

// NewCalculator creates a Calculator with default parameters.
func NewCalculator() *Calculator {
	return &Calculator{
		ReservedRAMBytes:   DefaultReservedRAMBytes,
		PerSessionRAMBytes: DefaultPerSessionRAMBytes,
		HardCap:            DefaultHardCap,
	}
}

// Calculate computes the recommended capacity from system info.
func (c *Calculator) Calculate(info SystemInfo) CapacityResult {
	result := CapacityResult{
		CurrentSessions: info.ClaudeProcesses,
		RAMUsagePercent: info.RAMUsagePercent(),
		HardCap:         c.HardCap,
	}

	// RAM-based max: (availableRAM - reserved) / perSession
	result.RAMBasedMax = 0
	if info.AvailableRAMBytes > c.ReservedRAMBytes {
		// Bounded by perSessionRAMBytes (>= 1 byte) so the quotient fits in int
		// for any realistic RAM size; explicit conversion safe here.
		result.RAMBasedMax = int((info.AvailableRAMBytes - c.ReservedRAMBytes) / c.PerSessionRAMBytes) //nolint:gosec // bounded by per-session RAM, fits in int
	}

	// CPU-based max: numCPUs * 2
	result.CPUBasedMax = info.NumCPUs * 2

	// Take the minimum of all constraints
	result.MaxSessions = min3(result.RAMBasedMax, result.CPUBasedMax, c.HardCap)

	// Floor at 0
	if result.MaxSessions < 0 {
		result.MaxSessions = 0
	}

	// Check for AGM_MAX_SESSIONS env var override
	if envVal := os.Getenv("AGM_MAX_SESSIONS"); envVal != "" {
		if n, err := strconv.Atoi(envVal); err == nil && n >= 0 {
			result.EnvOverride = n
			result.MaxSessions = n
		}
	}

	// Available slots
	result.AvailableSlots = result.MaxSessions - result.CurrentSessions
	if result.AvailableSlots < 0 {
		result.AvailableSlots = 0
	}

	// Determine zone from current RAM usage
	result.Zone = classifyZone(result.RAMUsagePercent)

	return result
}

// classifyZone returns the capacity zone for a given RAM usage percentage.
func classifyZone(ramPercent float64) Zone {
	switch {
	case ramPercent > 90:
		return ZoneCritical
	case ramPercent > 80:
		return ZoneRed
	case ramPercent >= 60:
		return ZoneYellow
	default:
		return ZoneGreen
	}
}

// ZoneDescription returns a human-readable description of what a zone means.
func ZoneDescription(z Zone) string {
	switch z {
	case ZoneGreen:
		return "Launch freely"
	case ZoneYellow:
		return "Be cautious — memory pressure building"
	case ZoneRed:
		return "Stop launching new sessions"
	case ZoneCritical:
		return "ALERT — system under severe memory pressure"
	default:
		return "Unknown"
	}
}

// String returns a formatted summary of the capacity result.
func (r CapacityResult) String() string {
	return fmt.Sprintf("max=%d current=%d available=%d zone=%s ram=%.1f%%",
		r.MaxSessions, r.CurrentSessions, r.AvailableSlots, r.Zone, r.RAMUsagePercent)
}

func min3(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}
