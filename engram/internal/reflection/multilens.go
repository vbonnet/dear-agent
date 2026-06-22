package reflection

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Multi-lens retro aggregation (ce-bi19).
//
// Instead of analyzing retrospective artifacts through a single uniform lens
// (the "what went wrong" extraction performed by RetrospectiveParser), this
// file applies four DISTINCT analytical lenses to a corpus of DEAR retro
// documents and synthesizes their findings into one structured knowledge-base
// report. The four lenses are:
//
//	(1) LensRootCause      — root-cause pattern extraction
//	(2) LensRecurrence     — recurrence frequency scoring
//	(3) LensRemediation    — remediation recommendation generation
//	(4) LensClassification — systemic-vs-one-off classification
//
// A Supervisor runs all four lenses over a shared, deterministic clustering of
// the corpus and joins their per-pattern findings into a SynthesisReport. This
// is the dogfood realization of the engram pipeline: retro artifacts ->
// structured patterns -> persistent KB.

// Lens identifies one analytical perspective applied to retrospective artifacts.
type Lens string

// The four analytical lenses assigned to retro workers.
const (
	// LensRootCause extracts the underlying cause patterns behind failures.
	LensRootCause Lens = "root_cause"
	// LensRecurrence scores how frequently each pattern recurs across the corpus.
	LensRecurrence Lens = "recurrence"
	// LensRemediation gathers the remediation recommendations for each pattern.
	LensRemediation Lens = "remediation"
	// LensClassification labels each pattern as systemic or one-off.
	LensClassification Lens = "classification"
)

// Classification labels for the systemic-vs-one-off lens.
const (
	ClassSystemic = "systemic"
	ClassOneOff   = "one_off"
)

// RetroArtifact is a parsed DEAR retrospective document. The DEAR format
// (Define -> Execute -> Audit -> Retro) maps cleanly onto the lenses: the Audit
// section yields root causes, the Retro section yields remediations.
type RetroArtifact struct {
	Path         string   // source file path
	Title        string   // first H1 heading
	Date         string   // value of the "Date:" metadata bullet, if present
	Severity     string   // value of the "Severity:" metadata bullet (e.g. "P1")
	RootCauses   []string // statements from the Audit section / "root cause" lines
	Remediations []string // statements from the Retro "what do we change" section
}

// LensFinding is a single per-pattern observation produced by one lens.
type LensFinding struct {
	Lens     Lens     // which lens produced this finding
	Pattern  string   // canonical pattern key (shared across lenses)
	Label    string   // human-readable representative label for the pattern
	Detail   string   // lens-specific detail
	Sources  []string // artifact titles contributing to this pattern
	Score    float64  // lens-specific score (recurrence count, severity weight, ...)
	Category string   // classification lens: ClassSystemic or ClassOneOff
}

// LensAnalyzer is one analytical worker. Each implementation views the same
// corpus through a different lens and emits independent findings.
type LensAnalyzer interface {
	Lens() Lens
	Analyze(clusters []*patternCluster) []LensFinding
}

// SynthesizedPattern joins all four lens findings for a single pattern.
type SynthesizedPattern struct {
	Pattern        string   // canonical pattern key
	Label          string   // representative label
	Classification string   // ClassSystemic or ClassOneOff
	Recurrence     int      // number of distinct artifacts exhibiting the pattern
	SeverityWeight float64  // aggregate severity weight across contributing artifacts
	RootCauses     []string // representative root-cause statements
	Remediations   []string // recommended remediations
	Sources        []string // contributing artifact titles
}

// SynthesisReport is the Supervisor's cross-lens synthesis over a corpus.
type SynthesisReport struct {
	ArtifactCount int
	Patterns      []SynthesizedPattern // sorted: systemic first, then by recurrence desc
}

// Supervisor orchestrates the four lenses and synthesizes their findings.
type Supervisor struct {
	lenses []LensAnalyzer
}

// NewSupervisor returns a Supervisor wired with the four standard lenses.
func NewSupervisor() *Supervisor {
	return &Supervisor{
		lenses: []LensAnalyzer{
			&rootCauseLens{},
			&recurrenceLens{},
			&remediationLens{},
			&classificationLens{},
		},
	}
}

// Synthesize clusters the corpus, runs every lens over the shared clustering,
// and joins their findings by pattern into a single report.
func (s *Supervisor) Synthesize(artifacts []*RetroArtifact) *SynthesisReport {
	clusters := buildClusters(artifacts)

	// Run each lens and index its findings by pattern key.
	byLens := make(map[Lens]map[string]LensFinding, len(s.lenses))
	for _, lens := range s.lenses {
		idx := make(map[string]LensFinding)
		for _, f := range lens.Analyze(clusters) {
			idx[f.Pattern] = f
		}
		byLens[lens.Lens()] = idx
	}

	report := &SynthesisReport{ArtifactCount: len(artifacts)}
	for _, c := range clusters {
		key := c.key
		sp := SynthesizedPattern{
			Pattern: key,
			Label:   c.label(),
			Sources: c.artifactTitles(),
		}
		if f, ok := byLens[LensRootCause][key]; ok {
			sp.RootCauses = splitDetail(f.Detail)
		}
		if f, ok := byLens[LensRecurrence][key]; ok {
			sp.Recurrence = int(f.Score)
		}
		if f, ok := byLens[LensRemediation][key]; ok {
			sp.Remediations = splitDetail(f.Detail)
		}
		if f, ok := byLens[LensClassification][key]; ok {
			sp.Classification = f.Category
			sp.SeverityWeight = f.Score
		}
		report.Patterns = append(report.Patterns, sp)
	}

	sortPatterns(report.Patterns)
	return report
}

// sortPatterns orders patterns deterministically: systemic before one-off,
// then by recurrence descending, then by label for stable output.
func sortPatterns(ps []SynthesizedPattern) {
	sort.SliceStable(ps, func(i, j int) bool {
		a, b := ps[i], ps[j]
		if (a.Classification == ClassSystemic) != (b.Classification == ClassSystemic) {
			return a.Classification == ClassSystemic
		}
		if a.Recurrence != b.Recurrence {
			return a.Recurrence > b.Recurrence
		}
		return a.Label < b.Label
	})
}

// --- Lens implementations -------------------------------------------------
//
// All four lenses operate over the SAME deterministic clustering so the
// Supervisor can join their findings by pattern key. Each lens contributes a
// genuinely different analytical view of every cluster.

// rootCauseLens (1) extracts representative root-cause statements per pattern.
type rootCauseLens struct{}

func (l *rootCauseLens) Lens() Lens { return LensRootCause }

func (l *rootCauseLens) Analyze(clusters []*patternCluster) []LensFinding {
	findings := make([]LensFinding, 0, len(clusters))
	for _, c := range clusters {
		causes := dedupeStrings(c.statements)
		findings = append(findings, LensFinding{
			Lens:    LensRootCause,
			Pattern: c.key,
			Label:   c.label(),
			Detail:  joinDetail(causes),
			Sources: c.artifactTitles(),
			Score:   float64(len(causes)),
		})
	}
	return findings
}

// recurrenceLens (2) scores how often each pattern recurs across the corpus.
// The score is the number of DISTINCT artifacts exhibiting the pattern — a
// pattern seen in one retro scores 1, one seen across five retros scores 5.
type recurrenceLens struct{}

func (l *recurrenceLens) Lens() Lens { return LensRecurrence }

func (l *recurrenceLens) Analyze(clusters []*patternCluster) []LensFinding {
	findings := make([]LensFinding, 0, len(clusters))
	for _, c := range clusters {
		recurrence := len(c.artifactSet())
		findings = append(findings, LensFinding{
			Lens:    LensRecurrence,
			Pattern: c.key,
			Label:   c.label(),
			Detail:  fmt.Sprintf("seen in %d artifact(s), %d total mention(s)", recurrence, len(c.statements)),
			Sources: c.artifactTitles(),
			Score:   float64(recurrence),
		})
	}
	return findings
}

// remediationLens (3) gathers the remediation recommendations attached to the
// artifacts contributing to each pattern.
type remediationLens struct{}

func (l *remediationLens) Lens() Lens { return LensRemediation }

func (l *remediationLens) Analyze(clusters []*patternCluster) []LensFinding {
	findings := make([]LensFinding, 0, len(clusters))
	for _, c := range clusters {
		var rems []string
		for _, a := range c.artifactSet() {
			rems = append(rems, a.Remediations...)
		}
		rems = dedupeStrings(rems)
		findings = append(findings, LensFinding{
			Lens:    LensRemediation,
			Pattern: c.key,
			Label:   c.label(),
			Detail:  joinDetail(rems),
			Sources: c.artifactTitles(),
			Score:   float64(len(rems)),
		})
	}
	return findings
}

// classificationLens (4) labels each pattern systemic vs one-off. A pattern is
// systemic when it recurs across multiple artifacts OR when a contributing
// artifact carries high severity (P0/P1) — a single high-severity failure is
// treated as systemic because the blast radius warrants structural attention.
type classificationLens struct{}

func (l *classificationLens) Lens() Lens { return LensClassification }

func (l *classificationLens) Analyze(clusters []*patternCluster) []LensFinding {
	findings := make([]LensFinding, 0, len(clusters))
	for _, c := range clusters {
		recurrence := len(c.artifactSet())
		weight := c.severityWeight()
		category := ClassOneOff
		if recurrence >= 2 || weight >= severityWeightP1 {
			category = ClassSystemic
		}
		findings = append(findings, LensFinding{
			Lens:     LensClassification,
			Pattern:  c.key,
			Label:    c.label(),
			Detail:   fmt.Sprintf("recurrence=%d severity_weight=%.1f", recurrence, weight),
			Sources:  c.artifactTitles(),
			Score:    weight,
			Category: category,
		})
	}
	return findings
}

// --- Clustering substrate -------------------------------------------------

// severityWeight values map a retro's "Severity: PN" metadata onto a numeric
// weight used by the classification lens.
const (
	severityWeightP0 = 4.0
	severityWeightP1 = 3.0
	severityWeightP2 = 2.0
	severityWeightP3 = 1.0
)

func severityWeight(severity string) float64 {
	switch {
	case strings.Contains(severity, "P0"):
		return severityWeightP0
	case strings.Contains(severity, "P1"):
		return severityWeightP1
	case strings.Contains(severity, "P2"):
		return severityWeightP2
	case strings.Contains(severity, "P3"):
		return severityWeightP3
	default:
		return severityWeightP3
	}
}

// statement is a single root-cause line tagged with its source artifact.
type statement struct {
	text     string
	keywords map[string]struct{}
	artifact *RetroArtifact
}

// patternCluster groups root-cause statements that share significant keywords.
// All four lenses key on cluster.key so the Supervisor can join their findings.
type patternCluster struct {
	key        string // canonical signature (sorted top keywords)
	statements []string
	sources    []*statement
}

// label returns a human-readable representative for the cluster: the shortest
// non-trivial statement, which tends to be the crispest root-cause summary.
func (c *patternCluster) label() string {
	if len(c.statements) == 0 {
		return c.key
	}
	best := c.statements[0]
	for _, s := range c.statements[1:] {
		if len(s) < len(best) {
			best = s
		}
	}
	return best
}

// artifactSet returns the distinct artifacts contributing to the cluster, in
// deterministic (sorted-by-title) order.
func (c *patternCluster) artifactSet() []*RetroArtifact {
	seen := make(map[string]*RetroArtifact)
	for _, s := range c.sources {
		seen[s.artifact.Path] = s.artifact
	}
	out := make([]*RetroArtifact, 0, len(seen))
	for _, a := range seen {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out
}

func (c *patternCluster) artifactTitles() []string {
	var titles []string
	for _, a := range c.artifactSet() {
		titles = append(titles, a.Title)
	}
	return titles
}

// severityWeight returns the maximum severity weight across contributing
// artifacts — the cluster is as severe as its worst contributing retro.
func (c *patternCluster) severityWeight() float64 {
	var maxWeight float64
	for _, a := range c.artifactSet() {
		if w := severityWeight(a.Severity); w > maxWeight {
			maxWeight = w
		}
	}
	return maxWeight
}

// minSharedKeywords is the keyword-overlap threshold for merging a statement
// into an existing cluster. Two root-cause lines that share at least this many
// significant keywords are treated as the same recurring pattern.
const minSharedKeywords = 2

// buildClusters performs a deterministic greedy single-pass clustering of all
// root-cause statements in the corpus. Statements sharing >= minSharedKeywords
// significant keywords with an existing cluster join it; otherwise they seed a
// new cluster. Processing order is sorted (by artifact title, then statement)
// so the result is stable across runs.
func buildClusters(artifacts []*RetroArtifact) []*patternCluster {
	sorted := append([]*RetroArtifact(nil), artifacts...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Title < sorted[j].Title })

	var stmts []*statement
	for _, a := range sorted {
		causes := append([]string(nil), a.RootCauses...)
		sort.Strings(causes)
		for _, text := range causes {
			kw := keywordSet(text)
			if len(kw) == 0 {
				continue
			}
			stmts = append(stmts, &statement{text: text, keywords: kw, artifact: a})
		}
	}

	var clusters []*patternCluster
	for _, s := range stmts {
		var target *patternCluster
		for _, c := range clusters {
			if sharedKeywordCount(c, s) >= minSharedKeywords {
				target = c
				break
			}
		}
		if target == nil {
			target = &patternCluster{}
			clusters = append(clusters, target)
		}
		target.statements = append(target.statements, s.text)
		target.sources = append(target.sources, s)
	}

	for _, c := range clusters {
		c.key = clusterKey(c)
	}
	return clusters
}

// sharedKeywordCount counts keywords shared between a statement and the union
// of keywords already in a cluster.
func sharedKeywordCount(c *patternCluster, s *statement) int {
	union := make(map[string]struct{})
	for _, existing := range c.sources {
		for k := range existing.keywords {
			union[k] = struct{}{}
		}
	}
	n := 0
	for k := range s.keywords {
		if _, ok := union[k]; ok {
			n++
		}
	}
	return n
}

// clusterKey builds a stable canonical signature from a cluster's most common
// keywords (top few, sorted), used as the join key across lenses.
func clusterKey(c *patternCluster) string {
	freq := make(map[string]int)
	for _, s := range c.sources {
		for k := range s.keywords {
			freq[k]++
		}
	}
	keys := make([]string, 0, len(freq))
	for k := range freq {
		keys = append(keys, k)
	}
	// Sort by frequency desc, then alphabetically for determinism.
	sort.Slice(keys, func(i, j int) bool {
		if freq[keys[i]] != freq[keys[j]] {
			return freq[keys[i]] > freq[keys[j]]
		}
		return keys[i] < keys[j]
	})
	if len(keys) > 4 {
		keys = keys[:4]
	}
	sort.Strings(keys)
	return strings.Join(keys, "-")
}

var keywordPattern = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9_-]+`)

// stopwords are common words excluded from keyword sets so clustering keys on
// meaningful, domain-specific tokens.
var stopwords = map[string]struct{}{
	"the": {}, "and": {}, "for": {}, "was": {}, "were": {}, "with": {}, "that": {},
	"this": {}, "from": {}, "have": {}, "has": {}, "had": {}, "are": {}, "but": {},
	"not": {}, "all": {}, "any": {}, "its": {}, "into": {}, "when": {}, "which": {},
	"would": {}, "could": {}, "should": {}, "did": {}, "does": {}, "been": {},
	"because": {}, "they": {}, "them": {}, "their": {}, "then": {}, "than": {},
	"each": {}, "some": {}, "more": {}, "most": {}, "such": {}, "only": {},
	"also": {}, "over": {}, "after": {}, "before": {}, "while": {}, "what": {},
	"how": {}, "why": {}, "who": {}, "where": {}, "root": {}, "cause": {},
}

// keywordSet extracts the significant lowercase keywords from a statement.
func keywordSet(text string) map[string]struct{} {
	out := make(map[string]struct{})
	for _, w := range keywordPattern.FindAllString(strings.ToLower(text), -1) {
		if len(w) < 4 {
			continue
		}
		if _, skip := stopwords[w]; skip {
			continue
		}
		out[w] = struct{}{}
	}
	return out
}

// --- small helpers --------------------------------------------------------

const detailSep = "\n"

func joinDetail(items []string) string { return strings.Join(items, detailSep) }

func splitDetail(detail string) []string {
	if detail == "" {
		return nil
	}
	return strings.Split(detail, detailSep)
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// --- corpus loading & rendering -------------------------------------------

// LoadCorpus reads every .md file in dir as a DEAR retro artifact.
func LoadCorpus(dir string) ([]*RetroArtifact, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read retro dir: %w", err)
	}
	var artifacts []*RetroArtifact
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		artifacts = append(artifacts, ParseRetroArtifact(path, string(data)))
	}
	return artifacts, nil
}

var (
	h1Pattern       = regexp.MustCompile(`^#\s+(.+)$`)
	h2Pattern       = regexp.MustCompile(`^#{2,3}\s+(.+)$`)
	metaDatePattern = regexp.MustCompile(`(?i)^\s*[-*]\s*\*\*Date:\*\*\s*(.+)$`)
	metaSevPattern  = regexp.MustCompile(`(?i)^\s*[-*]\s*\*\*Severity:\*\*\s*(.+)$`)
	rcInlinePattern = regexp.MustCompile(`(?i)root cause`)
	bulletLine      = regexp.MustCompile(`^\s*[-*]\s+(.+)$`)
)

// section classifies which DEAR section a heading introduces.
type section int

const (
	sectionNone section = iota
	sectionAudit
	sectionRetro
	sectionOther
)

func classifySection(heading string) section {
	h := strings.ToLower(heading)
	switch {
	case strings.HasPrefix(h, "audit"):
		return sectionAudit
	case strings.HasPrefix(h, "retro"):
		return sectionRetro
	default:
		return sectionOther
	}
}

// ParseRetroArtifact parses a DEAR retro document into a RetroArtifact. Root
// causes are drawn from the Audit section (and any "root cause" lines anywhere);
// remediations are drawn from bullet lines in the Retro section.
func ParseRetroArtifact(path, content string) *RetroArtifact {
	a := &RetroArtifact{Path: path, Title: strings.TrimSuffix(filepath.Base(path), ".md")}
	cur := sectionNone

	for line := range strings.SplitSeq(content, "\n") {
		if m := h1Pattern.FindStringSubmatch(line); m != nil {
			a.Title = strings.TrimSpace(m[1])
			continue
		}
		if m := metaDatePattern.FindStringSubmatch(line); m != nil {
			a.Date = strings.TrimSpace(m[1])
			continue
		}
		if m := metaSevPattern.FindStringSubmatch(line); m != nil {
			a.Severity = strings.TrimSpace(m[1])
			continue
		}
		if m := h2Pattern.FindStringSubmatch(line); m != nil {
			cur = classifySection(strings.TrimSpace(m[1]))
			// A "### Root cause" style subheading is itself a root-cause signal.
			if rcInlinePattern.MatchString(line) {
				if txt := cleanStatement(m[1]); txt != "" {
					a.RootCauses = append(a.RootCauses, txt)
				}
			}
			continue
		}

		// Bold inline "**Root cause:** ..." lines anywhere count as root causes.
		if rcInlinePattern.MatchString(line) {
			if txt := afterRootCause(line); txt != "" {
				a.RootCauses = append(a.RootCauses, txt)
				continue
			}
		}

		if m := bulletLine.FindStringSubmatch(line); m != nil {
			txt := cleanStatement(m[1])
			if txt == "" {
				continue
			}
			switch cur {
			case sectionAudit:
				a.RootCauses = append(a.RootCauses, txt)
			case sectionRetro:
				a.Remediations = append(a.Remediations, txt)
			case sectionNone, sectionOther:
				// bullets outside the Audit/Retro sections are not analyzed
			}
		}
	}

	a.RootCauses = dedupeStrings(a.RootCauses)
	a.Remediations = dedupeStrings(a.Remediations)
	return a
}

// leadingParen strips a single leading parenthetical qualifier like "(one line)".
var leadingParen = regexp.MustCompile(`^\s*\([^)]*\)\s*`)

// afterRootCause returns the text following a "root cause" marker on a line,
// e.g. "**Root cause (one line):** the loop spawned dupes" -> "the loop ...".
func afterRootCause(line string) string {
	idx := rcInlinePattern.FindStringIndex(strings.ToLower(line))
	if idx == nil {
		return ""
	}
	rest := line[idx[1]:]
	rest = leadingParen.ReplaceAllString(rest, "")
	rest = strings.TrimLeft(rest, " :*—-")
	return cleanStatement(rest)
}

// minStatementLen drops fragments too short to carry an analyzable root cause.
const minStatementLen = 12

// cleanStatement normalizes a markdown statement into plain prose: it removes
// emphasis markers, collapses whitespace, and rejects template placeholders,
// table rows, and fragments too short to be meaningful.
func cleanStatement(s string) string {
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "`", "")
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimLeft(s, "*_-—:. ")
	s = strings.TrimSpace(s)
	if s == "" || isTemplatePlaceholder(s) {
		return ""
	}
	if strings.Contains(s, " | ") { // markdown table row, not a prose statement
		return ""
	}
	if len(s) < minStatementLen {
		return ""
	}
	return s
}

// RenderMarkdown renders a SynthesisReport as a persistent-KB markdown document.
func (r *SynthesisReport) RenderMarkdown() string {
	var b strings.Builder
	b.WriteString("# Multi-Lens Retro Synthesis\n\n")
	fmt.Fprintf(&b, "_Synthesized across %d retro artifact(s) via 4 lenses: root-cause, recurrence, remediation, classification._\n\n", r.ArtifactCount)

	systemic := 0
	for _, p := range r.Patterns {
		if p.Classification == ClassSystemic {
			systemic++
		}
	}
	fmt.Fprintf(&b, "**Patterns:** %d total — %d systemic, %d one-off.\n\n", len(r.Patterns), systemic, len(r.Patterns)-systemic)

	for i, p := range r.Patterns {
		fmt.Fprintf(&b, "## %d. %s\n\n", i+1, p.Label)
		fmt.Fprintf(&b, "- **Classification:** %s\n", p.Classification)
		fmt.Fprintf(&b, "- **Recurrence:** %d artifact(s)\n", p.Recurrence)
		fmt.Fprintf(&b, "- **Severity weight:** %.1f\n", p.SeverityWeight)
		if len(p.Sources) > 0 {
			fmt.Fprintf(&b, "- **Sources:** %s\n", strings.Join(p.Sources, "; "))
		}
		if len(p.RootCauses) > 0 {
			b.WriteString("- **Root causes:**\n")
			for _, rc := range p.RootCauses {
				fmt.Fprintf(&b, "  - %s\n", rc)
			}
		}
		if len(p.Remediations) > 0 {
			b.WriteString("- **Recommended remediations:**\n")
			for _, rm := range p.Remediations {
				fmt.Fprintf(&b, "  - %s\n", rm)
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}
