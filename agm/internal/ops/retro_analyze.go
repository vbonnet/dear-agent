package ops

import (
	"os"
	"regexp"
	"sort"
	"strings"
)

// AnalyzeRetroRequest defines the input for multi-lens retrospective analysis.
//
// The analysis is pure static analysis of a retrospective markdown FILE: it
// parses the existing retro content and extracts structured insights per lens.
// It does NOT re-run the session or call out to an LLM — the DEAR retro format
// (Define / Enforce / Audit / Refine) is structured enough to mine directly.
type AnalyzeRetroRequest struct {
	// InputPath is the path to the retrospective markdown file to analyze.
	InputPath string `json:"input_path"`
	// Lens selects a single lens to run. Empty (with AllLenses=false) defaults
	// to running all lenses. One of: root-cause, recurrence, remediation,
	// systemic-vs-oneoff.
	Lens string `json:"lens,omitempty"`
	// AllLenses runs all four lenses regardless of Lens.
	AllLenses bool `json:"all_lenses,omitempty"`
}

// Lens identifiers. These are the four analytical lenses a supervisor can
// synthesize across multiple worker retros.
const (
	LensRootCause   = "root-cause"
	LensRecurrence  = "recurrence"
	LensRemediation = "remediation"
	LensSystemic    = "systemic-vs-oneoff"
)

// AllLensIDs is the canonical ordered list of lens identifiers.
var AllLensIDs = []string{LensRootCause, LensRecurrence, LensRemediation, LensSystemic}

// RootCauseLens extracts the root-cause chain from a retro: what failed, why,
// and the contributing factors.
type RootCauseLens struct {
	Summary string   `json:"summary"`
	Chain   []string `json:"chain"`
}

// RecurrenceLens scores whether the pattern has appeared before.
//
// Score: 0 = novel, 1 = seen-once, 2 = recurring. PriorInstances lists bead
// IDs referenced as earlier occurrences of the same pattern.
type RecurrenceLens struct {
	Score          int      `json:"score"`
	Label          string   `json:"label"`
	PriorInstances []string `json:"prior_instances"`
	Signals        []string `json:"signals,omitempty"`
}

// RemediationAction is one concrete follow-up extracted from a retro.
type RemediationAction struct {
	Bead        string `json:"bead,omitempty"`
	PR          string `json:"pr,omitempty"`
	Description string `json:"description"`
}

// RemediationLens collects concrete follow-up actions: bead IDs, PR numbers,
// and process changes.
type RemediationLens struct {
	Actions []RemediationAction `json:"actions"`
}

// SystemicLens classifies the issue as systemic (structural, will recur) or
// one-off (context-specific, unlikely to repeat).
type SystemicLens struct {
	Classification string `json:"classification"`
	Reason         string `json:"reason"`
	SystemicScore  int    `json:"systemic_score"`
	OneOffScore    int    `json:"oneoff_score"`
}

// RetroLenses bundles the findings of each lens that was run. Lenses not run
// are nil so agent-mode JSON stays compact.
type RetroLenses struct {
	RootCause   *RootCauseLens   `json:"root_cause,omitempty"`
	Recurrence  *RecurrenceLens  `json:"recurrence,omitempty"`
	Remediation *RemediationLens `json:"remediation,omitempty"`
	Systemic    *SystemicLens    `json:"systemic,omitempty"`
}

// AnalyzeRetroResult is the output of AnalyzeRetro.
type AnalyzeRetroResult struct {
	Operation string      `json:"operation"`
	RetroFile string      `json:"retro_file"`
	Title     string      `json:"title,omitempty"`
	Lenses    RetroLenses `json:"lenses"`
	// Synthesis is the supervisor's cross-lens verdict. It is only populated
	// when all four lenses ran, since the synthesis reasons across every lens.
	Synthesis *RetroSynthesis `json:"synthesis,omitempty"`
}

var (
	beadRe    = regexp.MustCompile(`\bce-[a-z0-9]{3,}\b`)
	prRe      = regexp.MustCompile(`(?i)\bPR[ -]?#?(\d{2,})\b|#(\d{2,})\b`)
	headingRe = regexp.MustCompile(`^#{1,4}\s+(.*\S)\s*$`)
	// numbered or bulleted list item: "1. foo", "- foo", "* foo"
	listItemRe = regexp.MustCompile(`^\s*(?:\d+\.|[-*])\s+(.*\S)\s*$`)
	boldKVRe   = regexp.MustCompile(`^\*\*([^*]+):\*\*\s*(.*)$`)
)

// AnalyzeRetro runs the requested analytical lenses over a retrospective file.
func AnalyzeRetro(req *AnalyzeRetroRequest) (*AnalyzeRetroResult, error) {
	if strings.TrimSpace(req.InputPath) == "" {
		return nil, ErrInvalidInput("input", "an --input retrospective file path is required")
	}

	raw, err := os.ReadFile(req.InputPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrInvalidInput("input", "retrospective file not found: "+req.InputPath)
		}
		return nil, ErrInvalidInput("input", "cannot read retrospective file: "+err.Error())
	}

	content := string(raw)
	doc := parseRetro(content)

	// Decide which lenses to run.
	run := map[string]bool{}
	if req.AllLenses || strings.TrimSpace(req.Lens) == "" {
		for _, id := range AllLensIDs {
			run[id] = true
		}
	} else {
		lens := normalizeLens(req.Lens)
		if lens == "" {
			return nil, ErrInvalidInput("lens",
				"unknown lens "+req.Lens+"; expected one of: "+strings.Join(AllLensIDs, ", "))
		}
		run[lens] = true
	}

	result := &AnalyzeRetroResult{
		Operation: "retro_analyze",
		RetroFile: req.InputPath,
		Title:     doc.title,
	}
	if run[LensRootCause] {
		result.Lenses.RootCause = analyzeRootCause(doc)
	}
	if run[LensRecurrence] {
		result.Lenses.Recurrence = analyzeRecurrence(content)
	}
	if run[LensRemediation] {
		result.Lenses.Remediation = analyzeRemediation(doc)
	}
	if run[LensSystemic] {
		result.Lenses.Systemic = analyzeSystemic(content)
	}

	// Supervisor step: when every lens ran, synthesize the four findings into a
	// single prioritized verdict. synthesizeLenses returns nil for partial sets.
	result.Synthesis = synthesizeLenses(&result.Lenses)

	return result, nil
}

// normalizeLens maps user-supplied lens aliases to canonical IDs.
func normalizeLens(lens string) string {
	switch strings.ToLower(strings.TrimSpace(lens)) {
	case "root-cause", "root_cause", "rootcause", "root":
		return LensRootCause
	case "recurrence", "recur", "frequency":
		return LensRecurrence
	case "remediation", "remediate", "actions", "followup", "follow-up":
		return LensRemediation
	case "systemic-vs-oneoff", "systemic", "systemic_vs_oneoff", "classification", "class":
		return LensSystemic
	default:
		return ""
	}
}

// retroSection is one heading-delimited block of a retro document.
type retroSection struct {
	heading string // heading text without leading #'s
	level   int    // heading level (number of #'s)
	body    []string
}

// retroDoc is the parsed structure of a retrospective markdown file.
type retroDoc struct {
	title    string
	meta     map[string]string // **Key:** value lines near the top
	sections []retroSection
}

// parseRetro splits a retro markdown file into title, metadata, and sections.
func parseRetro(content string) retroDoc {
	doc := retroDoc{meta: map[string]string{}}
	var cur *retroSection
	for line := range strings.SplitSeq(content, "\n") {
		if m := headingRe.FindStringSubmatch(line); m != nil {
			level := len(line) - len(strings.TrimLeft(line, "#"))
			if level == 1 && doc.title == "" {
				doc.title = strings.TrimSpace(m[1])
				continue
			}
			doc.sections = append(doc.sections, retroSection{heading: m[1], level: level})
			cur = &doc.sections[len(doc.sections)-1]
			continue
		}
		if kv := boldKVRe.FindStringSubmatch(strings.TrimSpace(line)); kv != nil {
			doc.meta[strings.ToLower(strings.TrimSpace(kv[1]))] = strings.TrimSpace(kv[2])
		}
		if cur != nil {
			cur.body = append(cur.body, line)
		}
	}
	return doc
}

// findSection returns the first section whose heading contains any of the
// given keywords (case-insensitive), or nil.
func (d retroDoc) findSection(keywords ...string) *retroSection {
	for i := range d.sections {
		h := strings.ToLower(d.sections[i].heading)
		for _, kw := range keywords {
			if strings.Contains(h, kw) {
				return &d.sections[i]
			}
		}
	}
	return nil
}

// firstParagraph returns the first non-empty prose paragraph from body lines,
// skipping blank lines, list markers, and code fences.
func firstParagraph(body []string) string {
	var para []string
	inFence := false
	for _, line := range body {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if t == "" {
			if len(para) > 0 {
				break
			}
			continue
		}
		// Skip standalone bold-label lines like "**The invariant violated:**"
		// and horizontal rules so the summary is prose, not a section label.
		if t == "---" || (boldKVRe.MatchString(t) && strings.TrimSpace(boldKVRe.FindStringSubmatch(t)[2]) == "") {
			continue
		}
		para = append(para, t)
	}
	return strings.TrimSpace(strings.Join(para, " "))
}

// analyzeRootCause mines the Enforce/root-cause section for the failure chain.
func analyzeRootCause(doc retroDoc) *RootCauseLens {
	lens := &RootCauseLens{Chain: []string{}}

	// Summary: prefer the Define section's invariant/prose; fall back to a
	// Summary section or the document title.
	if s := doc.findSection("define", "summary", "what happened", "central finding"); s != nil {
		lens.Summary = firstParagraph(s.body)
	}
	if lens.Summary == "" {
		lens.Summary = doc.title
	}

	// Chain: prefer the Enforce / root-cause section. Collect its sub-headings
	// (### items) as the ordered chain of contributing factors; if there are
	// none, fall back to top-level numbered/bulleted list items.
	src := doc.findSection("enforce", "root cause", "root-cause", "why this happened")
	if src != nil {
		lens.Chain = append(lens.Chain, subHeadingsAfter(doc, src)...)
		if len(lens.Chain) == 0 {
			lens.Chain = listItems(src.body)
		}
	}
	if len(lens.Chain) == 0 {
		// Last resort: any section mentioning "cause" contributes its list items.
		if s := doc.findSection("cause", "failure", "gap"); s != nil {
			lens.Chain = listItems(s.body)
		}
	}
	return lens
}

// subHeadingsAfter returns the heading texts of sections that are nested under
// the given section (deeper level) until a sibling/parent heading appears.
func subHeadingsAfter(doc retroDoc, parent *retroSection) []string {
	var out []string
	started := false
	for i := range doc.sections {
		s := &doc.sections[i]
		if s == parent {
			started = true
			continue
		}
		if !started {
			continue
		}
		if s.level <= parent.level {
			break
		}
		out = append(out, strings.TrimSpace(s.heading))
	}
	return out
}

// listItems extracts the text of numbered/bulleted list items from body lines,
// skipping code fences and nested table rows.
func listItems(body []string) []string {
	var out []string
	inFence := false
	for _, line := range body {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if m := listItemRe.FindStringSubmatch(line); m != nil {
			item := strings.TrimSpace(stripMarkdownEmphasis(m[1]))
			if item != "" {
				out = append(out, item)
			}
		}
	}
	return out
}

// stripMarkdownEmphasis removes surrounding ** and ` for cleaner extraction.
func stripMarkdownEmphasis(s string) string {
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "`", "")
	return strings.TrimSpace(s)
}

// recurrenceKeywords are signals that a pattern has recurred. Each match adds
// weight toward a higher recurrence score.
var recurrenceKeywords = []string{
	"recur", "again", "still red", "still broken", "compounding", "consecutive",
	"keeps", "repeated", "repeatedly", "previous retro", "prior retro", "as before",
	"same root", "same pattern", "happened before", "the recurring", "third time",
	"second time", "every merge", "every pr", "yet again", "once more",
}

// analyzeRecurrence scores how often this pattern has appeared. PriorInstances
// are bead IDs referenced in the retro (candidate earlier occurrences).
func analyzeRecurrence(content string) *RecurrenceLens {
	lens := &RecurrenceLens{PriorInstances: []string{}, Signals: []string{}}

	lower := strings.ToLower(content)
	seen := map[string]bool{}
	for _, kw := range recurrenceKeywords {
		if strings.Contains(lower, kw) && !seen[kw] {
			seen[kw] = true
			lens.Signals = append(lens.Signals, kw)
		}
	}

	lens.PriorInstances = uniqueSorted(beadRe.FindAllString(content, -1))

	// Scoring: 0 novel, 1 seen-once, 2 recurring.
	//   - Strong recurrence language OR ≥2 prior bead references → 2
	//   - Any recurrence signal OR ≥1 prior bead reference → 1
	//   - Otherwise → 0
	switch {
	case len(lens.Signals) >= 2 || len(lens.PriorInstances) >= 2:
		lens.Score = 2
	case len(lens.Signals) >= 1 || len(lens.PriorInstances) >= 1:
		lens.Score = 1
	default:
		lens.Score = 0
	}
	lens.Label = recurrenceLabel(lens.Score)
	return lens
}

func recurrenceLabel(score int) string {
	switch score {
	case 2:
		return "recurring"
	case 1:
		return "seen-once"
	default:
		return "novel"
	}
}

// remediationHeadings mark sections that hold follow-up actions.
var remediationHeadings = []string{
	"refine", "action item", "follow-up", "follow up", "followup", "next step",
	"remediation", "recommended", "prevention", "resolve", "open follow",
	"todo", "fixes", "execute",
}

// analyzeRemediation extracts concrete follow-up actions from the retro.
func analyzeRemediation(doc retroDoc) *RemediationLens {
	lens := &RemediationLens{Actions: []RemediationAction{}}
	seen := map[string]bool{}

	add := func(desc string) {
		desc = strings.TrimSpace(stripMarkdownEmphasis(desc))
		if desc == "" {
			return
		}
		key := strings.ToLower(desc)
		if seen[key] {
			return
		}
		seen[key] = true
		lens.Actions = append(lens.Actions, RemediationAction{
			Bead:        firstMatch(beadRe, desc),
			PR:          firstPR(desc),
			Description: desc,
		})
	}

	for i := range doc.sections {
		s := &doc.sections[i]
		// Whole-section scan when the heading itself is remediation-flavored
		// (e.g. "## Refine", "## Follow-ups").
		if headingMatchesAny(s.heading, remediationHeadings) {
			for _, item := range listItems(s.body) {
				add(item)
			}
			for _, row := range tableActionRows(s.body) {
				add(row)
			}
		} else {
			// Otherwise look for a remediation-flavored bold lead-in inside the
			// body (e.g. "**Proposed fixes (prioritized):**" under "## Retro")
			// and collect the list that follows it.
			for _, item := range leadInActionItems(s.body) {
				add(item)
			}
		}
		// Checklist items ("- [ ] ...") are actionable regardless of section.
		for _, item := range checklistItems(s.body) {
			add(item)
		}
	}
	return lens
}

// tableActionRows extracts a meaningful cell from markdown table rows. It skips
// header/separator rows and returns the longest descriptive cell per row.
func tableActionRows(body []string) []string {
	var out []string
	for _, line := range body {
		t := strings.TrimSpace(line)
		if !strings.HasPrefix(t, "|") {
			continue
		}
		if strings.Contains(t, "---") {
			continue
		}
		cells := strings.Split(strings.Trim(t, "|"), "|")
		var best string
		headerish := false
		for _, c := range cells {
			c = strings.TrimSpace(c)
			lc := strings.ToLower(c)
			if lc == "action" || lc == "owner" || lc == "status" || lc == "#" {
				headerish = true
			}
			if len(c) > len(best) {
				best = c
			}
		}
		if headerish {
			continue
		}
		if best != "" {
			out = append(out, best)
		}
	}
	return out
}

// remediationLeadIns are bold lead-in phrases that introduce an action list
// inside an otherwise non-remediation section.
var remediationLeadIns = []string{
	"proposed fix", "proposed change", "fix", "action item", "follow-up",
	"follow up", "next step", "remediation", "recommended", "prevention",
	"todo", "to do", "what to do", "mitigation",
}

// leadInActionItems collects list items that immediately follow a bold
// remediation lead-in line within a section body. Collection stays active
// across blank lines (numbered lists are often spaced out) and stops at the
// next bold lead-in that is not remediation-flavored.
func leadInActionItems(body []string) []string {
	var out []string
	active := false
	inFence := false
	for _, line := range body {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if boldKVRe.MatchString(t) {
			active = headingMatchesAny(t, remediationLeadIns)
			continue
		}
		if active {
			if m := listItemRe.FindStringSubmatch(line); m != nil {
				if item := strings.TrimSpace(stripMarkdownEmphasis(m[1])); item != "" {
					out = append(out, item)
				}
			}
		}
	}
	return out
}

// checklistItems returns the text of GitHub-style task list items.
func checklistItems(body []string) []string {
	var out []string
	re := regexp.MustCompile(`^\s*[-*]\s+\[[ xX]\]\s+(.*\S)\s*$`)
	for _, line := range body {
		if m := re.FindStringSubmatch(line); m != nil {
			out = append(out, strings.TrimSpace(m[1]))
		}
	}
	return out
}

// systemicKeywords / oneOffKeywords drive the systemic-vs-one-off classifier.
var systemicKeywords = []string{
	"systemic", "structural", "will recur", "policy", "process gap", "process change",
	"by default", "every", "design flaw", "architectural", "compounding", "recurring",
	"class of", "whole class", "pattern", "guardrail", "invariant", "enforce",
}

var oneOffKeywords = []string{
	"one-off", "one off", "context-specific", "context specific", "specific to",
	"isolated", "emergency", "unlikely to repeat", "unlikely to recur", "edge case",
	"typo", "manual error", "fat-finger", "fat finger", "transient", "fluke",
}

// analyzeSystemic classifies the retro as systemic or one-off via weighted
// keyword signals.
func analyzeSystemic(content string) *SystemicLens {
	lower := strings.ToLower(content)
	lens := &SystemicLens{}

	var sysHits, oneHits []string
	for _, kw := range systemicKeywords {
		if strings.Contains(lower, kw) {
			lens.SystemicScore += strings.Count(lower, kw)
			sysHits = append(sysHits, kw)
		}
	}
	for _, kw := range oneOffKeywords {
		if strings.Contains(lower, kw) {
			lens.OneOffScore += strings.Count(lower, kw)
			oneHits = append(oneHits, kw)
		}
	}

	switch {
	case lens.SystemicScore == 0 && lens.OneOffScore == 0:
		lens.Classification = "indeterminate"
		lens.Reason = "no clear systemic or one-off signals found in the retro text"
	case lens.SystemicScore >= lens.OneOffScore:
		lens.Classification = "systemic"
		lens.Reason = "structural/process signals dominate: " + strings.Join(uniqueSorted(sysHits), ", ")
	default:
		lens.Classification = "one-off"
		lens.Reason = "context-specific signals dominate: " + strings.Join(uniqueSorted(oneHits), ", ")
	}
	return lens
}

// --- small helpers ---

func headingMatchesAny(heading string, keywords []string) bool {
	h := strings.ToLower(heading)
	for _, kw := range keywords {
		if strings.Contains(h, kw) {
			return true
		}
	}
	return false
}

func firstMatch(re *regexp.Regexp, s string) string {
	return re.FindString(s)
}

// firstPR returns a normalized "PR #N" string for the first PR reference, or "".
func firstPR(s string) string {
	m := prRe.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	for _, g := range m[1:] {
		if g != "" {
			return "#" + g
		}
	}
	return ""
}

func uniqueSorted(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
