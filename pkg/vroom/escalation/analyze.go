package escalation

import "sort"

// EscalationSummary folds all events for one escalation into the single record
// the three analyses run over. It is built by [Summarize] from the append-only
// event log, so an analysis never has to reason about per-event phase ordering.
type EscalationSummary struct {
	EscalationID    string
	Kind            Kind
	Question        string
	QuestionHash    string
	Topic           string
	OriginSessionID string

	Answered       bool
	Answer         string
	AnsweredByRole string

	// Outcome / Misalignment are the adjudicator's backfilled verdict (empty if
	// the escalation was never answered or never adjudicated).
	Outcome      Outcome
	Misalignment string
}

// Summarize folds the event log into one summary per escalation, keyed by
// EscalationID and ordered by first appearance (stable, so output is
// deterministic). Identity fields (question, origin, kind, topic) are taken from
// the earliest event that carries them — the raised event in practice; answer
// and outcome from the answered event.
func Summarize(events []EscalationEvent) []EscalationSummary {
	order := make([]string, 0)
	byID := make(map[string]*EscalationSummary)

	for i := range events {
		ev := events[i]
		if ev.EscalationID == "" {
			continue
		}
		s, ok := byID[ev.EscalationID]
		if !ok {
			s = &EscalationSummary{EscalationID: ev.EscalationID}
			byID[ev.EscalationID] = s
			order = append(order, ev.EscalationID)
		}
		s.fold(ev)
	}

	out := make([]EscalationSummary, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	return out
}

// fold merges one event into the summary: identity fields are taken from the
// first event that carries them (first-write-wins); resolution fields from the
// answered/auto-resolved event.
func (s *EscalationSummary) fold(ev EscalationEvent) {
	firstWrite(&s.Question, ev.Question)
	firstWrite(&s.QuestionHash, ev.QuestionHash)
	firstWrite((*string)(&s.Kind), string(ev.Kind))
	firstWrite(&s.Topic, ev.Topic)
	firstWrite(&s.OriginSessionID, ev.OriginSessionID)

	if ev.Phase == PhaseAnswered || ev.Phase == PhaseAutoResolved {
		s.Answered = true
		lastWrite(&s.Answer, ev.Answer)
		lastWrite(&s.AnsweredByRole, ev.AnsweredByRole)
	}
	lastWrite((*string)(&s.Outcome), ev.Outcome)
	lastWrite(&s.Misalignment, ev.Misalignment)
}

// firstWrite sets *dst to v only if *dst is empty and v is not (first-write-wins).
func firstWrite(dst *string, v string) {
	if *dst == "" && v != "" {
		*dst = v
	}
}

// lastWrite sets *dst to v whenever v is non-empty (last non-empty wins).
func lastWrite(dst *string, v string) {
	if v != "" {
		*dst = v
	}
}

// MisalignedAnswer is one answered escalation the adjudicator flagged as not
// correct/aligned — analysis (1).
type MisalignedAnswer struct {
	EscalationID   string  `json:"escalation_id"`
	Outcome        Outcome `json:"outcome"`
	Question       string  `json:"question"`
	Answer         string  `json:"answer"`
	AnsweredByRole string  `json:"answered_by_role,omitempty"`
	Misalignment   string  `json:"misalignment,omitempty"`
	Topic          string  `json:"topic,omitempty"`
	Kind           Kind    `json:"kind,omitempty"`
}

// AnalyzeMisaligned returns the answered escalations whose adjudicated outcome is
// incorrect or misaligned — `WHERE outcome IN ('incorrect', 'misaligned')`.
// Ordered incorrect-before-misaligned, then by escalation id for stability.
func AnalyzeMisaligned(events []EscalationEvent) []MisalignedAnswer {
	out := make([]MisalignedAnswer, 0)
	for _, s := range Summarize(events) {
		if s.Outcome != OutcomeIncorrect && s.Outcome != OutcomeMisaligned {
			continue
		}
		out = append(out, MisalignedAnswer{
			EscalationID:   s.EscalationID,
			Outcome:        s.Outcome,
			Question:       s.Question,
			Answer:         s.Answer,
			AnsweredByRole: s.AnsweredByRole,
			Misalignment:   s.Misalignment,
			Topic:          s.Topic,
			Kind:           s.Kind,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Outcome != out[j].Outcome {
			return out[i].Outcome == OutcomeIncorrect // incorrect first
		}
		return out[i].EscalationID < out[j].EscalationID
	})
	return out
}

// QuestionGroup aggregates escalations that share a normalised question (same
// question_hash) — the unit of analyses (2) and (3).
type QuestionGroup struct {
	QuestionHash    string   `json:"question_hash"`
	Question        string   `json:"question"`
	Topic           string   `json:"topic,omitempty"`
	Kind            Kind     `json:"kind,omitempty"`
	Count           int      `json:"count"`            // total escalations with this question
	DistinctOrigins int      `json:"distinct_origins"` // distinct origin sessions that asked it
	OriginSessions  []string `json:"origin_sessions,omitempty"`
}

// groupByQuestion folds summaries into QuestionGroups keyed by question_hash.
// Escalations with no question_hash (none raised, or pre-hash logs) are skipped.
func groupByQuestion(summaries []EscalationSummary) []*QuestionGroup {
	order := make([]string, 0)
	byHash := make(map[string]*QuestionGroup)
	origins := make(map[string]map[string]bool)

	for _, s := range summaries {
		if s.QuestionHash == "" {
			continue
		}
		g, ok := byHash[s.QuestionHash]
		if !ok {
			g = &QuestionGroup{
				QuestionHash: s.QuestionHash,
				Question:     s.Question,
				Topic:        s.Topic,
				Kind:         s.Kind,
			}
			byHash[s.QuestionHash] = g
			origins[s.QuestionHash] = make(map[string]bool)
			order = append(order, s.QuestionHash)
		}
		g.Count++
		if s.OriginSessionID != "" && !origins[s.QuestionHash][s.OriginSessionID] {
			origins[s.QuestionHash][s.OriginSessionID] = true
			g.OriginSessions = append(g.OriginSessions, s.OriginSessionID)
		}
	}
	out := make([]*QuestionGroup, 0, len(order))
	for _, h := range order {
		g := byHash[h]
		g.DistinctOrigins = len(g.OriginSessions)
		out = append(out, g)
	}
	return out
}

// sortGroups orders groups by Count desc, then DistinctOrigins desc, then hash
// for stability.
func sortGroups(groups []*QuestionGroup) {
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].Count != groups[j].Count {
			return groups[i].Count > groups[j].Count
		}
		if groups[i].DistinctOrigins != groups[j].DistinctOrigins {
			return groups[i].DistinctOrigins > groups[j].DistinctOrigins
		}
		return groups[i].QuestionHash < groups[j].QuestionHash
	})
}

// AnalyzeFrequentQuestions groups escalations by question_hash and returns the
// groups asked at least minCount times, most-frequent first — analysis (2),
// `GROUP BY question_hash`. minCount <= 0 defaults to 2 (a question asked once
// is not "frequent").
func AnalyzeFrequentQuestions(events []EscalationEvent, minCount int) []QuestionGroup {
	if minCount <= 0 {
		minCount = 2
	}
	groups := groupByQuestion(Summarize(events))
	sortGroups(groups)
	out := make([]QuestionGroup, 0, len(groups))
	for _, g := range groups {
		if g.Count >= minCount {
			out = append(out, *g)
		}
	}
	return out
}

// AnalyzeManyAgents groups escalations by question_hash and returns the groups
// asked by at least minDistinctOrigins *distinct* origin sessions — analysis
// (3), the missing-prompt-context signal:
// `GROUP BY question_hash HAVING COUNT(DISTINCT origin_session_id) >= N`.
// minDistinctOrigins <= 0 defaults to 2.
func AnalyzeManyAgents(events []EscalationEvent, minDistinctOrigins int) []QuestionGroup {
	if minDistinctOrigins <= 0 {
		minDistinctOrigins = 2
	}
	groups := groupByQuestion(Summarize(events))
	// Order by distinct origins first for this analysis.
	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].DistinctOrigins != groups[j].DistinctOrigins {
			return groups[i].DistinctOrigins > groups[j].DistinctOrigins
		}
		if groups[i].Count != groups[j].Count {
			return groups[i].Count > groups[j].Count
		}
		return groups[i].QuestionHash < groups[j].QuestionHash
	})
	out := make([]QuestionGroup, 0, len(groups))
	for _, g := range groups {
		if g.DistinctOrigins >= minDistinctOrigins {
			out = append(out, *g)
		}
	}
	return out
}
