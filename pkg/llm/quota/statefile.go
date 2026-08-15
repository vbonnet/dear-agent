package quota

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// StateFileVersion is the schema version of the published state file.
// Consumers should refuse a version they do not know rather than guess.
const StateFileVersion = 1

// DefaultStateFileRelPath is the state file's location under the user's
// state directory. It sits beside the other dear-agent state so an
// operator finds it where they already look.
const DefaultStateFileRelPath = "dear-agent/quota/latest.json"

// State is the published quota reading: the stable contract between
// whatever refreshes the meter and everything that consumes it.
//
// It exists because reading CodexBar takes seconds. An orchestrator that
// wants to check headroom before dispatching work cannot pay that on
// every decision, and a spawn gate certainly cannot. One writer refreshes
// this file on a schedule; every reader gets an O(1) local read with an
// explicit age it can judge for itself.
type State struct {
	// Version is StateFileVersion. A reader that does not recognise it
	// must treat the file as unusable rather than misinterpret it.
	Version int `json:"version"`

	// GeneratedAt is when the underlying meter produced the reading.
	GeneratedAt time.Time `json:"generatedAt"`

	// WrittenAt is when this file was written. It differs from
	// GeneratedAt by however long the read took.
	WrittenAt time.Time `json:"writtenAt"`

	// Source and SourceVersion identify the meter behind the reading.
	Source        string `json:"source"`
	SourceVersion string `json:"sourceVersion,omitempty"`

	// Providers is one entry per provider family.
	Providers []ProviderState `json:"providers"`
}

// ProviderState is the published reading for one provider family.
type ProviderState struct {
	// Family is the dear-agent provider family, the key callers join on.
	Family string `json:"family"`

	// SourceID is the meter's own provider name, for diagnostics.
	SourceID string `json:"sourceId"`

	// Plan is the subscription tier, when reported.
	Plan string `json:"plan,omitempty"`

	// Readable is false when no usable reading exists. Consumers must
	// check this before trusting RemainingPercent: an unreadable provider
	// is not an exhausted one.
	Readable bool `json:"readable"`

	// Availability explains an unreadable provider: "auth_required",
	// "disabled", or "unavailable".
	Availability string `json:"availability"`

	// RoutingClass is the router's verdict: healthy, deprioritized,
	// avoid, or unknown.
	RoutingClass string `json:"routingClass"`

	// BreakerState is the cost guardrail's position: closed, throttled,
	// or open.
	BreakerState string `json:"breakerState"`

	// RemainingPercent is the most constrained window's headroom.
	// Meaningful only when Readable is true.
	RemainingPercent float64 `json:"remainingPercent"`

	// ConstrainedWindow labels the binding sub-budget.
	ConstrainedWindow string `json:"constrainedWindow,omitempty"`

	// ResetsAt is when the binding window refills.
	ResetsAt *time.Time `json:"resetsAt,omitempty"`

	// Overspending is true when the burn rate will not reach the reset.
	Overspending bool `json:"overspending"`

	// PaceSummary is the meter's own burn-rate sentence, when it has one.
	PaceSummary string `json:"paceSummary,omitempty"`

	// Reason is the one-line explanation behind BreakerState.
	Reason string `json:"reason"`

	// Windows is every sub-budget, so a consumer can render detail
	// without a second read.
	Windows []WindowState `json:"windows"`
}

// WindowState is one published sub-budget.
type WindowState struct {
	ID               string     `json:"id,omitempty"`
	Label            string     `json:"label,omitempty"`
	RemainingPercent float64    `json:"remainingPercent"`
	UsedPercent      float64    `json:"usedPercent"`
	ResetAt          *time.Time `json:"resetAt,omitempty"`
}

// Age reports how stale the reading is. Consumers decide their own
// tolerance; the file does not expire itself.
func (s *State) Age(now time.Time) time.Duration {
	if s == nil || s.GeneratedAt.IsZero() {
		return 0
	}
	if age := now.Sub(s.GeneratedAt); age > 0 {
		return age
	}
	return 0
}

// Provider returns one family's published reading.
func (s *State) Provider(family string) (ProviderState, bool) {
	if s == nil {
		return ProviderState{}, false
	}
	for _, p := range s.Providers {
		if p.Family == family {
			return p, true
		}
	}
	return ProviderState{}, false
}

// DefaultStateFilePath resolves the published state file's location,
// honouring XDG_STATE_HOME and falling back to ~/.local/state.
func DefaultStateFilePath() (string, error) {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, DefaultStateFileRelPath), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("quota: resolve home directory: %w", err)
	}
	return filepath.Join(home, ".local", "state", DefaultStateFileRelPath), nil
}

// BuildState renders a snapshot plus the two policies into the published
// shape, so the file records the verdicts rather than making every reader
// re-derive them from raw percentages.
func BuildState(snapshot *Snapshot, meter *Meter, breaker *Breaker, now time.Time) *State {
	state := &State{
		Version:   StateFileVersion,
		WrittenAt: now,
	}
	if snapshot == nil {
		return state
	}
	state.GeneratedAt = snapshot.GeneratedAt
	state.Source = snapshot.Source
	state.SourceVersion = snapshot.SourceVersion

	for _, p := range snapshot.Providers {
		decision := meter.DecisionFor(p.Family)
		verdict := breaker.Evaluate(p.Family)
		entry := ProviderState{
			Family:            p.Family,
			SourceID:          p.SourceID,
			Plan:              p.Plan,
			Readable:          p.Availability.Known(),
			Availability:      string(p.Availability),
			RoutingClass:      string(decision.Class),
			BreakerState:      string(verdict.State),
			RemainingPercent:  decision.RemainingPercent,
			ConstrainedWindow: decision.ConstrainedWindow,
			Overspending:      p.Pace.Overspending(),
			Reason:            verdict.Reason,
		}
		if p.Pace != nil {
			entry.PaceSummary = p.Pace.Summary
		}
		if !verdict.ResetsAt.IsZero() {
			resets := verdict.ResetsAt
			entry.ResetsAt = &resets
		}
		for _, w := range p.Windows {
			ws := WindowState{
				ID:               w.ID,
				Label:            w.Label,
				RemainingPercent: w.RemainingPercent,
				UsedPercent:      w.UsedPercent,
			}
			if !w.ResetAt.IsZero() {
				reset := w.ResetAt
				ws.ResetAt = &reset
			}
			entry.Windows = append(entry.Windows, ws)
		}
		state.Providers = append(state.Providers, entry)
	}
	return state
}

// WriteStateFile publishes state to path, creating parent directories.
//
// The write is atomic — a temp file in the same directory followed by a
// rename — because the whole point of this file is that other processes
// read it at arbitrary moments. A reader must never see a half-written
// budget.
func WriteStateFile(path string, state *State) error {
	if state == nil {
		return errors.New("quota: refusing to write a nil state")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("quota: create state directory %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("quota: encode state: %w", err)
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(dir, ".quota-state-*.json")
	if err != nil {
		return fmt.Errorf("quota: create temp state file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("quota: write temp state file: %w", err)
	}
	// Owner-only: the reading names the account's subscription plan, and
	// every consumer runs as the same user. Chmod the open file descriptor,
	// not the path, so a symlink swapped in at tmpName between create and
	// chmod cannot redirect the permission change onto another file
	// (CWE-377/CWE-59).
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("quota: set state file mode: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("quota: close temp state file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("quota: publish state file %s: %w", path, err)
	}
	return nil
}

// ErrNoStateFile reports that no reading has been published yet. It is
// distinct from a corrupt file: nothing published is a normal condition
// before the first refresh, and callers treat it as "unknown", not
// "broken".
var ErrNoStateFile = errors.New("quota: no published state file")

// ReadStateFile loads a published reading.
//
// It returns ErrNoStateFile when nothing has been published, and a
// wrapped error for a corrupt or unrecognised file. Every one of those
// outcomes means "unknown" to a fail-safe caller.
func ReadStateFile(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNoStateFile
	}
	if err != nil {
		return nil, fmt.Errorf("quota: read state file %s: %w", path, err)
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("quota: parse state file %s: %w", path, err)
	}
	if state.Version != StateFileVersion {
		return nil, fmt.Errorf("quota: state file %s has version %d, this build reads %d",
			path, state.Version, StateFileVersion)
	}
	return &state, nil
}
