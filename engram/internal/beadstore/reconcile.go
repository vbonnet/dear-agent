package beadstore

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// BackfillLabelPrefix marks a bead as backfilled from a legacy JSONL store;
// the suffix is the legacy bead ID. It makes reconciliation idempotent.
const BackfillLabelPrefix = "backfill-src:"

// LegacyBead is one row of the legacy JSONL store the pre-fix beads_create
// tool wrote to (e.g. ~/.beads/issues.jsonl).
type LegacyBead struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	Description      string   `json:"description"`
	Priority         int      `json:"priority"` // legacy range 0-5
	Labels           []string `json:"labels"`
	EstimatedMinutes int      `json:"estimated_minutes"`
	Status           string   `json:"status"`
	CreatedAt        string   `json:"created_at"`
}

// ReconcileEntry describes the disposition of one legacy bead.
type ReconcileEntry struct {
	LegacyID string `json:"legacy_id"`
	Title    string `json:"title"`
	BeadID   string `json:"bead_id,omitempty"` // ID in the bd store (created or pre-existing)
	Reason   string `json:"reason,omitempty"`  // why it was skipped / would-create
}

// ReconcileResult summarizes a reconcile run.
type ReconcileResult struct {
	StorePath  string           `json:"store_path"`
	LegacyPath string           `json:"legacy_path"`
	DryRun     bool             `json:"dry_run"`
	Created    []ReconcileEntry `json:"created"`
	Skipped    []ReconcileEntry `json:"skipped"`
}

// LoadLegacyJSONL reads a legacy JSONL bead store, skipping malformed lines.
func LoadLegacyJSONL(path string) ([]LegacyBead, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open legacy store: %w", err)
	}
	defer func() { _ = f.Close() }()

	var beads []LegacyBead
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var b LegacyBead
		if err := json.Unmarshal([]byte(line), &b); err != nil || b.ID == "" {
			continue // malformed line; not fatal for reconciliation
		}
		beads = append(beads, b)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read legacy store: %w", err)
	}
	return beads, nil
}

// Reconcile backfills open beads from a legacy JSONL store that never landed
// in this bd store (the ce-ctsi silent-loss scenario). A legacy bead is
// skipped when it is not open, when a bead already carries its
// backfill-src:<id> label (previous reconcile), or when a bead with the same
// title already exists (e.g. it was re-filed manually). Backfilled beads are
// created via VerifiedCreate, so every write is read back before being
// reported as created.
func (s *Store) Reconcile(ctx context.Context, legacyPath string, dryRun bool) (*ReconcileResult, error) {
	if s.DBPath == "" {
		return nil, ErrStoreNotConfigured
	}
	legacy, err := LoadLegacyJSONL(legacyPath)
	if err != nil {
		return nil, err
	}

	existing, err := s.List(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("list store beads: %w", err)
	}
	byBackfillSrc := map[string]string{} // legacy ID -> bead ID
	byTitle := map[string]string{}       // lowercase title -> bead ID
	for _, b := range existing {
		byTitle[strings.ToLower(strings.TrimSpace(b.Title))] = b.ID
		for _, l := range b.Labels {
			if src, ok := strings.CutPrefix(l, BackfillLabelPrefix); ok {
				byBackfillSrc[src] = b.ID
			}
		}
	}

	res := &ReconcileResult{
		StorePath:  s.DBPath,
		LegacyPath: legacyPath,
		DryRun:     dryRun,
		Created:    []ReconcileEntry{},
		Skipped:    []ReconcileEntry{},
	}
	for _, lb := range legacy {
		entry := ReconcileEntry{LegacyID: lb.ID, Title: lb.Title}
		switch {
		case lb.Status != "open":
			entry.Reason = fmt.Sprintf("legacy status %q, only open beads are backfilled", lb.Status)
			res.Skipped = append(res.Skipped, entry)
		case byBackfillSrc[lb.ID] != "":
			entry.BeadID = byBackfillSrc[lb.ID]
			entry.Reason = "already backfilled"
			res.Skipped = append(res.Skipped, entry)
		case byTitle[strings.ToLower(strings.TrimSpace(lb.Title))] != "":
			entry.BeadID = byTitle[strings.ToLower(strings.TrimSpace(lb.Title))]
			entry.Reason = "bead with same title already in store"
			res.Skipped = append(res.Skipped, entry)
		case dryRun:
			entry.Reason = "would create (dry run)"
			res.Created = append(res.Created, entry)
		default:
			bead, err := s.VerifiedCreate(ctx, CreateRequest{
				Title:       lb.Title,
				Description: backfillDescription(lb, legacyPath),
				Priority:    clampPriority(lb.Priority),
				Labels: append(append([]string{}, lb.Labels...),
					BackfillLabelPrefix+lb.ID),
				EstimatedMinutes: lb.EstimatedMinutes,
			})
			if err != nil {
				return nil, fmt.Errorf("backfill of legacy bead %s failed: %w", lb.ID, err)
			}
			entry.BeadID = bead.ID
			res.Created = append(res.Created, entry)
		}
	}
	return res, nil
}

// clampPriority maps the legacy 0-5 priority range onto bd's 0-4.
func clampPriority(p int) int {
	if p < 0 {
		return 0
	}
	if p > 4 {
		return 4
	}
	return p
}

func backfillDescription(lb LegacyBead, legacyPath string) string {
	return fmt.Sprintf(
		"%s\n\n---\nBackfilled from legacy JSONL store %s (legacy id %s, created %s) — "+
			"originally acknowledged by the pre-fix engram MCP beads_create but never landed in the bd store (ce-ctsi).",
		lb.Description, legacyPath, lb.ID, lb.CreatedAt)
}
