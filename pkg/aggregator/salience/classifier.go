package salience

// Classifier maps a drift Kind to its default Tier. Production callers use
// DefaultClassifier; tests can substitute a fixed map.
type Classifier interface {
	Classify(k Kind) Tier
}

// MapClassifier is the trivial Classifier implementation: a Kind→Tier
// table plus a Fallback for kinds not in the table.
type MapClassifier struct {
	Table    map[Kind]Tier
	Fallback Tier
}

// Classify returns the table entry for k, or Fallback when k is missing.
func (m MapClassifier) Classify(k Kind) Tier {
	if t, ok := m.Table[k]; ok {
		return t
	}
	return m.Fallback
}

// defaultTierTable is the hard-coded policy: which kinds are worth
// interrupting a human for. Edit this table to retune; each row is one
// design decision.
var defaultTierTable = map[Kind]Tier{
	KindBuildFailure:   TierCritical, // ship-blocking
	KindTestFailure:    TierHigh,     // probably ship-blocking
	KindTemplateSkip:   TierHigh,     // agent ignored a project rule
	KindLintFailure:    TierMedium,   // mergeable but worth fixing
	KindDependencyBump: TierLow,      // routine, batchable
	KindNaming:         TierLow,      // taste-y, not urgent
	KindDocOnly:        TierLow,      // safe to bundle
	KindCosmetic:       TierNoise,    // never interrupt for these
	KindFormatting:     TierNoise,
}

// DefaultClassifier returns the policy used by the aggregator when no
// Classifier is configured. Returned by value so callers can wrap or
// override the table without mutating shared state.
func DefaultClassifier() Classifier {
	table := make(map[Kind]Tier, len(defaultTierTable))
	for k, v := range defaultTierTable {
		table[k] = v
	}
	return MapClassifier{Table: table, Fallback: TierLow}
}
