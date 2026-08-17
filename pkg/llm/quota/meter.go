package quota

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/vbonnet/dear-agent/pkg/llm/provider"
)

// DefaultRefreshInterval is how long a cached reading is served before
// the meter refreshes it in the background.
const DefaultRefreshInterval = 5 * time.Minute

// FamilyResolver maps a model id to a provider family. It is the same
// contract as provider.Resolver.Resolve; the seam exists so this package
// can be tested without constructing provider clients.
type FamilyResolver interface {
	Resolve(id string) (family, model string, err error)
}

// Options configures a Meter.
type Options struct {
	// Reader produces snapshots. Nil disables the meter: every decision
	// is ClassUnknown and routing is unchanged.
	Reader Reader

	// Policy sets the thresholds. The zero value uses the defaults.
	Policy Policy

	// Families maps model ids to provider families. Nil uses
	// provider.NewResolver().
	Families FamilyResolver

	// RefreshInterval is how long a reading is served before a
	// background refresh is triggered. Zero uses
	// DefaultRefreshInterval; negative disables background refresh, so
	// only explicit Refresh calls update the cache.
	RefreshInterval time.Duration

	// Now overrides the clock. Nil uses time.Now.
	Now func() time.Time
}

// Meter caches one Snapshot and answers routing questions from it.
//
// Reads on the routing path never block on the underlying source: the
// meter serves whatever it has and refreshes in the background. That
// matters because the CodexBar dashboard call takes seconds — it
// refreshes providers over the network — which is far too slow to sit in
// front of a model call.
//
// A nil *Meter is valid and behaves as a disabled meter, so callers can
// hold an unconditional field without nil checks at every use.
type Meter struct {
	reader          Reader
	policy          Policy
	families        FamilyResolver
	refreshInterval time.Duration
	now             func() time.Time

	mu         sync.Mutex
	snapshot   *Snapshot
	readAt     time.Time
	readErr    error
	refreshing bool
}

// New builds a Meter. It performs no I/O; the first reading arrives from
// Refresh or from the first background refresh.
func New(opts Options) *Meter {
	families := opts.Families
	if families == nil {
		families = provider.NewResolver()
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	interval := opts.RefreshInterval
	if interval == 0 {
		interval = DefaultRefreshInterval
	}
	return &Meter{
		reader:          opts.Reader,
		policy:          opts.Policy,
		families:        families,
		refreshInterval: interval,
		now:             now,
	}
}

// Enabled reports whether the meter has a source to read from.
func (m *Meter) Enabled() bool { return m != nil && m.reader != nil }

// Refresh reads a new snapshot synchronously and replaces the cache.
// Callers that want a reading before routing — a CLI, or a process
// warming up — use this; the routing path does not.
//
// On failure the previous snapshot is retained: a transient read error
// should not discard a reading that is still within its max age.
func (m *Meter) Refresh(ctx context.Context) (*Snapshot, error) {
	if !m.Enabled() {
		return nil, nil
	}
	snapshot, err := m.reader.Read(ctx)

	m.mu.Lock()
	defer m.mu.Unlock()
	m.readAt = m.now()
	m.readErr = err
	if err == nil {
		m.snapshot = snapshot
	}
	return m.snapshot, err
}

// Snapshot returns the cached reading without blocking, triggering a
// background refresh when the cache is empty or older than the refresh
// interval. The second result is the error from the most recent read
// attempt, retained so a caller can report why a reading is missing.
func (m *Meter) Snapshot() (*Snapshot, error) {
	if !m.Enabled() {
		return nil, nil
	}
	m.mu.Lock()
	snapshot, err := m.snapshot, m.readErr
	stale := m.readAt.IsZero() || m.now().Sub(m.readAt) >= m.refreshInterval
	shouldRefresh := stale && !m.refreshing && m.refreshInterval > 0
	if shouldRefresh {
		m.refreshing = true
	}
	m.mu.Unlock()

	if shouldRefresh {
		go m.refreshInBackground()
	}
	return snapshot, err
}

// refreshInBackground performs an off-path refresh. It carries no caller
// context on purpose: the refresh outlives the request that noticed the
// cache was stale, and the reader applies its own timeout.
func (m *Meter) refreshInBackground() {
	defer func() {
		m.mu.Lock()
		m.refreshing = false
		m.mu.Unlock()
	}()
	// The error is deliberately dropped here: Refresh has already stored it
	// on the meter, where Snapshot's caller can see and report it. There is
	// no caller on this path to return it to.
	_, _ = m.Refresh(context.Background())
}

// disabledDecision is the verdict a meter with no source returns: the
// same "route as configured" answer as any other unknown.
func disabledDecision() Decision {
	return Decision{
		Class:        ClassUnknown,
		Availability: AvailabilityUnavailable,
		Reason:       "quota metering disabled",
	}
}

// DecisionFor returns the routing verdict for a provider family.
func (m *Meter) DecisionFor(family string) Decision {
	if !m.Enabled() {
		d := disabledDecision()
		d.Family = family
		return d
	}
	snapshot, err := m.Snapshot()
	decision := Evaluate(snapshot, family, m.now(), m.policy)
	if err != nil && decision.Class == ClassUnknown {
		decision.Reason = "quota unreadable: " + err.Error()
	}
	return decision
}

// DecisionForModel resolves a model id to its family and returns that
// family's verdict. A model the resolver cannot place yields
// ClassUnknown, so an unrecognised model routes unchanged.
func (m *Meter) DecisionForModel(modelID string) Decision {
	if !m.Enabled() {
		return disabledDecision()
	}
	snapshot, err := m.Snapshot()
	decision := m.decide(snapshot, m.now(), modelID)
	if err != nil && decision.Class == ClassUnknown {
		decision.Reason = "quota unreadable: " + err.Error()
	}
	return decision
}

// decide evaluates one model against an already-taken reading.
func (m *Meter) decide(snapshot *Snapshot, now time.Time, modelID string) Decision {
	family, _, err := m.families.Resolve(modelID)
	if err != nil {
		return Decision{
			Class:        ClassUnknown,
			Availability: AvailabilityUnavailable,
			Reason:       "model is not mapped to a provider family",
		}
	}
	return Evaluate(snapshot, family, now, m.policy)
}

// OrderModels returns the candidates reordered so that providers with
// more headroom are tried first, along with the verdict behind each
// position. The ordering is stable within a band, so a role's configured
// vendor preference survives whenever quota does not distinguish the
// candidates.
//
// No candidate is ever dropped. A role stays routable even when every
// provider it can reach is out of quota — the request then fails at the
// provider, which is a clearer signal than a router that refuses to try.
func (m *Meter) OrderModels(models []string) ([]string, []Decision) {
	ordered := append([]string(nil), models...)
	decisions := make([]Decision, len(ordered))
	if !m.Enabled() {
		for i := range ordered {
			decisions[i] = disabledDecision()
		}
		return ordered, decisions
	}

	// One reading for the whole ordering. Evaluating each candidate
	// against its own Snapshot call could straddle a background refresh
	// and rank two candidates against different readings.
	snapshot, err := m.Snapshot()
	now := m.now()
	for i, id := range ordered {
		decisions[i] = m.decide(snapshot, now, id)
		if err != nil && decisions[i].Class == ClassUnknown {
			decisions[i].Reason = "quota unreadable: " + err.Error()
		}
	}

	index := make([]int, len(ordered))
	for i := range index {
		index[i] = i
	}
	sort.SliceStable(index, func(a, b int) bool {
		return Band(decisions[index[a]]) < Band(decisions[index[b]])
	})

	sortedModels := make([]string, len(ordered))
	sortedDecisions := make([]Decision, len(ordered))
	for position, original := range index {
		sortedModels[position] = ordered[original]
		sortedDecisions[position] = decisions[original]
	}
	return sortedModels, sortedDecisions
}

// HasCapacity implements the capacity filter that the workflow role
// resolver plugs in (pkg/workflow/roles.CapacityChecker).
//
// It rejects a model only on a positive reading that the family is at or
// below the avoid floor. Unknown, stale, disabled, and unauthenticated
// readings all report capacity, so a meter that cannot see a provider
// never takes that provider away.
func (m *Meter) HasCapacity(modelID string) bool {
	if !m.Enabled() {
		return true
	}
	return m.DecisionForModel(modelID).Class != ClassAvoid
}
