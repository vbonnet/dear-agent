package modelrouter

import "strings"

// TaskType classifies the kind of work a session will do.
type TaskType string

// Task type constants map to cost tiers: cheap, mid, or expensive.
const (
	TaskHealthCheck    TaskType = "health_check"    // → cheap
	TaskFormatting     TaskType = "formatting"      // → cheap
	TaskExtraction     TaskType = "extraction"      // → cheap
	TaskSimpleCode     TaskType = "simple_code"     // → cheap
	TaskCodeGen        TaskType = "code_gen"        // → mid
	TaskPRReview       TaskType = "pr_review"       // → mid
	TaskReportAnalysis TaskType = "report_analysis" // → mid
	TaskArchitecture   TaskType = "architecture"    // → expensive
	TaskSupervisor     TaskType = "supervisor"      // → expensive
	TaskUnknown        TaskType = "unknown"         // → mid (default)
)

// taskKeywords maps lowercased keywords to task types. The classifier checks
// the prompt for any match and returns the highest-priority task type found.
// Cheap keywords use two-word phrases to avoid colliding with expensive
// contexts; expensive keywords are checked before mid so they win on overlap.
var taskKeywords = []struct {
	keywords []string
	task     TaskType
}{
	// cheap signals — two-word phrases to avoid colliding with expensive contexts
	{[]string{"health check", "healthcheck", "liveness probe", "readiness probe", "is alive", "ping endpoint"}, TaskHealthCheck},
	{[]string{"auto-format", "gofmt ", "run prettier", "style fix", "fix formatting", "black format"}, TaskFormatting},
	{[]string{"extract data", "parse csv", "parse json", "extract text", "scrape the", "run etl"}, TaskExtraction},
	{[]string{"rename variable", "add comment", "fix typo", "update import", "bump version", "update changelog"}, TaskSimpleCode},

	// expensive signals — evaluated before mid so they win on overlap
	{[]string{"architecture decision", "system design", "write adr", "design doc", "wayfinder", "refactor large", "migration plan", "multi-agent"}, TaskArchitecture},
	{[]string{"vroom supervisor", "meta-o", "orchestrator loop", "mission control", "supervisory mesh", "governance gate"}, TaskSupervisor},

	// mid signals
	{[]string{"code review", "pr review", "review pr", "review pull request", "code audit"}, TaskPRReview},
	{[]string{"implement ", "write tests", "add feature", "fix bug", "debug ", "generate code", "write function", "write class"}, TaskCodeGen},
	{[]string{"analysis report", "summarize", "write summary", "analyze the", "evaluate the"}, TaskReportAnalysis},
}

// taskTier maps task types to their cost tier.
var taskTier = map[TaskType]Tier{
	TaskHealthCheck:    TierCheap,
	TaskFormatting:     TierCheap,
	TaskExtraction:     TierCheap,
	TaskSimpleCode:     TierCheap,
	TaskCodeGen:        TierMid,
	TaskPRReview:       TierMid,
	TaskReportAnalysis: TierMid,
	TaskArchitecture:   TierExpensive,
	TaskSupervisor:     TierExpensive,
	TaskUnknown:        TierMid,
}

// tierPriority ranks tiers so the highest-cost signal wins when multiple task
// types are found in a single prompt.
var tierPriority = map[Tier]int{
	TierCheap:     1,
	TierMid:       2,
	TierExpensive: 3,
}

// ClassifyPrompt returns the task type inferred from the prompt text. It uses
// a simple keyword scan; the result is intentionally coarse — it drives the
// default tier when no explicit --model-tier flag was given.
func ClassifyPrompt(prompt string) TaskType {
	lower := strings.ToLower(prompt)
	best := TaskUnknown
	bestPriority := 0

	for _, rule := range taskKeywords {
		for _, kw := range rule.keywords {
			if strings.Contains(lower, kw) {
				t := taskTier[rule.task]
				p := tierPriority[t]
				if p > bestPriority {
					bestPriority = p
					best = rule.task
				}
				break // one keyword match per rule is enough
			}
		}
	}
	return best
}

// TierForPrompt returns the cost tier inferred from a prompt. It wraps
// ClassifyPrompt and maps the result to a Tier.
func TierForPrompt(prompt string) Tier {
	task := ClassifyPrompt(prompt)
	if t, ok := taskTier[task]; ok {
		return t
	}
	return TierMid
}
