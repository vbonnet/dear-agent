package audit

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryStore is an in-memory Store implementation for tests. It
// preserves the lifecycle and de-dup semantics of SQLiteStore so a
// test that exercises the runner with MemoryStore is testing the
// same logic SQLiteStore enforces. Not safe to share between tests
// without a fresh instance.
type MemoryStore struct {
	mu        sync.Mutex
	now       func() time.Time
	idGen     func() string
	runs      map[string]AuditRunRecord
	findings  map[string]Finding   // keyed by FindingID
	byFP      map[string]string    // (repo|fp) -> FindingID
	proposals map[string]Proposal
	propByKey map[string]string    // (run|layer|title) -> ProposalID
}

// NewMemoryStore returns an empty MemoryStore using time.Now and
// uuid.NewString. Tests that want deterministic ids set Now and
// IDGen on the returned struct.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		now:       time.Now,
		idGen:     uuid.NewString,
		runs:      make(map[string]AuditRunRecord),
		findings:  make(map[string]Finding),
		byFP:      make(map[string]string),
		proposals: make(map[string]Proposal),
		propByKey: make(map[string]string),
	}
}

// SetClock pins the store's clock for deterministic tests.
func (m *MemoryStore) SetClock(now func() time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.now = now
}

// SetIDGen pins the store's ID generator for deterministic tests.
func (m *MemoryStore) SetIDGen(idGen func() string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.idGen = idGen
}

// Close is a no-op for the in-memory store.
func (m *MemoryStore) Close() error { return nil }

// BeginAuditRun records the run record.
func (m *MemoryStore) BeginAuditRun(_ context.Context, rec AuditRunRecord) error {
	if rec.AuditRunID == "" {
		return fmt.Errorf("audit: BeginAuditRun: AuditRunID is empty")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rec.State = AuditRunRunning
	m.runs[rec.AuditRunID] = rec
	return nil
}

// FinishAuditRun updates the run record with final state.
func (m *MemoryStore) FinishAuditRun(_ context.Context, rec AuditRunRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.runs[rec.AuditRunID]; !ok {
		return fmt.Errorf("audit: FinishAuditRun: no run %s", rec.AuditRunID)
	}
	m.runs[rec.AuditRunID] = rec
	return nil
}

// UpsertFinding mirrors SQLiteStore.UpsertFinding.
func (m *MemoryStore) UpsertFinding(_ context.Context, f Finding) (Finding, error) {
	if err := f.Validate(); err != nil {
		return Finding{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	key := f.Repo + "|" + f.Fingerprint
	if existingID, ok := m.byFP[key]; ok {
		existing := m.findings[existingID]
		f.FindingID = existingID
		f.FirstSeen = existing.FirstSeen
		f.LastSeen = now
		newState := existing.State
		if newState == FindingResolved {
			newState = FindingReopened
		}
		f.State = newState
		f.ResolvedAt = existing.ResolvedAt
		m.findings[existingID] = f
		return f, nil
	}
	f.FindingID = m.idGen()
	f.State = FindingOpen
	f.FirstSeen = now
	f.LastSeen = now
	m.findings[f.FindingID] = f
	m.byFP[key] = f.FindingID
	return f, nil
}

// SetFindingState transitions a finding through the manual lifecycle.
func (m *MemoryStore) SetFindingState(_ context.Context, findingID string, state FindingState, note string) (Finding, error) {
	if !state.IsValid() {
		return Finding{}, fmt.Errorf("audit: SetFindingState: invalid state %q", state)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.findings[findingID]
	if !ok {
		return Finding{}, ErrNotFound
	}
	if !legalManualTransition(f.State, state) {
		return Finding{}, fmt.Errorf("audit: SetFindingState: illegal transition %s → %s", f.State, state)
	}
	f.State = state
	if state == FindingResolved {
		f.ResolvedAt = m.now()
	}
	_ = note
	m.findings[findingID] = f
	return f, nil
}

// CountFindings returns counts; New is "first_seen within last 24h".
func (m *MemoryStore) CountFindings(_ context.Context, repo string) (FindingCounts, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cutoff := m.now().Add(-24 * time.Hour)
	var c FindingCounts
	for _, f := range m.findings {
		if f.Repo != repo {
			continue
		}
		switch f.State {
		case FindingOpen, FindingReopened, FindingAcknowledged:
			c.Open++
		case FindingResolved:
			c.Resolved++
		}
		if !f.FirstSeen.Before(cutoff) {
			c.New++
		}
	}
	return c, nil
}

// ListFindings filters and sorts (severity asc, last_seen desc).
func (m *MemoryStore) ListFindings(_ context.Context, filter FindingFilter) ([]Finding, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Finding, 0, len(m.findings))
	for _, f := range m.findings {
		if filter.Repo != "" && f.Repo != filter.Repo {
			continue
		}
		if filter.State != "" && f.State != filter.State {
			continue
		}
		if filter.Severity != "" && f.Severity != filter.Severity {
			continue
		}
		if filter.CheckID != "" && f.CheckID != filter.CheckID {
			continue
		}
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Severity.Rank() != out[j].Severity.Rank() {
			return out[i].Severity.Rank() < out[j].Severity.Rank()
		}
		return out[i].LastSeen.After(out[j].LastSeen)
	})
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}

// GetFinding returns one by id.
func (m *MemoryStore) GetFinding(_ context.Context, findingID string) (Finding, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f, ok := m.findings[findingID]
	if !ok {
		return Finding{}, ErrNotFound
	}
	return f, nil
}

// UpsertProposal returns existing id when (run, layer, title) collides.
func (m *MemoryStore) UpsertProposal(_ context.Context, p Proposal) (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	if p.AuditRunID == "" {
		return "", fmt.Errorf("audit: UpsertProposal: AuditRunID is empty")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := p.AuditRunID + "|" + string(p.Layer) + "|" + p.Title
	if id, ok := m.propByKey[key]; ok {
		return id, nil
	}
	id := m.idGen()
	p.ProposalID = id
	p.State = ProposalProposed
	m.proposals[id] = p
	m.propByKey[key] = id
	return id, nil
}

// ListProposals filters and sorts by proposed_at desc.
func (m *MemoryStore) ListProposals(_ context.Context, filter ProposalFilter) ([]Proposal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Proposal, 0, len(m.proposals))
	for _, p := range m.proposals {
		if filter.AuditRunID != "" && p.AuditRunID != filter.AuditRunID {
			continue
		}
		if filter.Layer != "" && p.Layer != filter.Layer {
			continue
		}
		if filter.State != "" && p.State != filter.State {
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ProposedAt.After(out[j].ProposedAt) })
	if filter.Limit > 0 && len(out) > filter.Limit {
		out = out[:filter.Limit]
	}
	return out, nil
}

// SetProposalState records a decision.
func (m *MemoryStore) SetProposalState(_ context.Context, proposalID string, state ProposalState, decidedBy, note string) error {
	if !state.IsValid() {
		return fmt.Errorf("audit: SetProposalState: invalid state %q", state)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.proposals[proposalID]
	if !ok {
		return ErrNotFound
	}
	p.State = state
	p.DecidedAt = m.now()
	p.DecidedBy = decidedBy
	p.Decision = note
	m.proposals[proposalID] = p
	return nil
}
