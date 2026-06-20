package supervisor

import "fmt"

// MeshMode describes how many of the three supervisors are currently reachable,
// from the perspective of one supervisor observing its two peers. The mesh is
// NOT all-or-nothing (retro Gap 4: "No quorum or degraded-mode concept"): it
// operates in defined modes with a formal capability set per mode (retro R.4).
//
// The observing supervisor always counts itself as alive, so the modes map to
// the 3/3, 2/3, 1/3 fractions named in the retro:
//
//	ModeNormal    = 3/3 — both peers reachable, full capability.
//	ModeDegraded  = 2/3 — one peer dark, reduced capability + elevated monitoring.
//	ModeEmergency = 1/3 — both peers dark, commit state + self-terminate + notify.
type MeshMode int

const (
	// ModeNormal is 3/3: the observing supervisor sees both peers alive.
	ModeNormal MeshMode = iota
	// ModeDegraded is 2/3: exactly one peer is dark.
	ModeDegraded
	// ModeEmergency is 1/3: both peers are dark.
	ModeEmergency
)

// String renders the mode as its fraction-and-name form for the decision trail.
func (m MeshMode) String() string {
	switch m {
	case ModeNormal:
		return "3/3 normal"
	case ModeDegraded:
		return "2/3 degraded"
	case ModeEmergency:
		return "1/3 emergency"
	default:
		return fmt.Sprintf("MeshMode(%d)", int(m))
	}
}

// ModeForAlivePeers maps the number of *peers* observed alive (0..2) to a mode.
// The observing supervisor counts itself, so two alive peers means 3/3.
// Values above 2 are clamped to ModeNormal; the three-supervisor doctrine has
// no fourth peer to observe.
func ModeForAlivePeers(alivePeers int) MeshMode {
	switch {
	case alivePeers >= 2:
		return ModeNormal
	case alivePeers == 1:
		return ModeDegraded
	default:
		return ModeEmergency
	}
}

// Capabilities is the formal capability set a supervisor operates under in a
// given MeshMode (retro R.4: "Define degraded-mode capabilities"). It is a
// plain value so it can be recorded to the decision trail on every mode
// transition and consulted by a supervisor's Tick before taking an action.
type Capabilities struct {
	// AddRoadmapItems reports whether this supervisor may admit work to the
	// roadmap. In normal operation only the Meta-Orchestrator holds this; in
	// degraded mode a supervisor that has assumed Meta-O authority via failover
	// holds it too (retro R.5, roadmap delegation). It is always false in
	// emergency mode.
	AddRoadmapItems bool

	// SpawnWorkers reports whether new workers may be dispatched. False in
	// emergency mode (the survivor is winding down, not starting new work).
	SpawnWorkers bool

	// SpawnRateLimit is a 0..1 multiplier on the normal worker-spawn rate.
	// 1.0 in normal mode; reduced in degraded mode so a shrunken mesh does not
	// over-commit; 0 in emergency mode.
	SpawnRateLimit float64

	// ElevatedMonitoring reports whether peer checks should run at a shorter
	// interval with faster escalation (retro R.2). True whenever the mesh is
	// not at full strength.
	ElevatedMonitoring bool

	// SelfTerminate reports whether the supervisor should commit its state and
	// exit cleanly (retro R.3 / R.4 emergency mode). True only in emergency.
	SelfTerminate bool

	// NotifyHuman reports whether a human should be paged. True in emergency
	// mode, where automated recovery is no longer possible.
	NotifyHuman bool
}

// CapabilitiesFor returns the capability set for mode. roadmapAuthority reports
// whether this supervisor currently holds roadmap-admission authority — either
// because it IS the Meta-Orchestrator, or because it has assumed Meta-O
// authority under failover (retro R.5). The flag only matters in normal and
// degraded modes; emergency mode never admits new roadmap work.
func CapabilitiesFor(mode MeshMode, roadmapAuthority bool) Capabilities {
	switch mode {
	case ModeNormal:
		return Capabilities{
			AddRoadmapItems:    roadmapAuthority,
			SpawnWorkers:       true,
			SpawnRateLimit:     1.0,
			ElevatedMonitoring: false,
			SelfTerminate:      false,
			NotifyHuman:        false,
		}
	case ModeDegraded:
		return Capabilities{
			AddRoadmapItems:    roadmapAuthority,
			SpawnWorkers:       true,
			SpawnRateLimit:     0.5,
			ElevatedMonitoring: true,
			SelfTerminate:      false,
			NotifyHuman:        false,
		}
	case ModeEmergency:
		return Capabilities{
			AddRoadmapItems:    false,
			SpawnWorkers:       false,
			SpawnRateLimit:     0,
			ElevatedMonitoring: true,
			SelfTerminate:      true,
			NotifyHuman:        true,
		}
	default:
		return Capabilities{}
	}
}

// asPayload renders capabilities as a decision-trail payload fragment.
func (c Capabilities) asPayload() map[string]any {
	return map[string]any{
		"add_roadmap_items":   c.AddRoadmapItems,
		"spawn_workers":       c.SpawnWorkers,
		"spawn_rate_limit":    c.SpawnRateLimit,
		"elevated_monitoring": c.ElevatedMonitoring,
		"self_terminate":      c.SelfTerminate,
		"notify_human":        c.NotifyHuman,
	}
}
