package costtrack

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// BudgetChecker reads JSONL cost logs and checks spending against budget limits.
type BudgetChecker struct {
	config  *BudgetConfig
	logPath string
}

// NewBudgetChecker creates a checker with the given config and cost log path.
func NewBudgetChecker(config *BudgetConfig, logPath string) *BudgetChecker {
	return &BudgetChecker{
		config:  config,
		logPath: logPath,
	}
}

// CheckBudget checks all budget periods for the given project/model.
// Returns a slice of BudgetStatus (one per period) and whether the request is allowed.
func (c *BudgetChecker) CheckBudget(project, model string) ([]BudgetStatus, bool, error) {
	return c.CheckBudgetAt(project, model, time.Now())
}

// CheckBudgetAt checks budgets relative to a specific time (for testing).
func (c *BudgetChecker) CheckBudgetAt(project, model string, now time.Time) ([]BudgetStatus, bool, error) {
	if isOverrideEnabled() {
		return nil, true, nil
	}

	limits := c.config.GetLimits(project, model)

	periods := []struct {
		period BudgetPeriod
		limit  float64
	}{
		{BudgetDaily, limits.Daily},
		{BudgetWeekly, limits.Weekly},
		{BudgetMonthly, limits.Monthly},
	}

	var statuses []BudgetStatus
	allowed := true

	for _, p := range periods {
		if p.limit <= 0 {
			continue // No limit configured for this period
		}

		start, end := PeriodBounds(p.period, now)
		spent, err := c.sumCosts(start, end, project, model)
		if err != nil {
			return nil, false, fmt.Errorf("sum costs for %s: %w", p.period, err)
		}

		remaining := p.limit - spent
		if remaining < 0 {
			remaining = 0
		}

		percent := 0.0
		if p.limit > 0 {
			percent = (spent / p.limit) * 100
		}

		// Get per-component breakdown
		componentMap, _ := c.CostByComponent(start, end)
		var componentCosts []ComponentCost
		for comp, compSpent := range componentMap {
			pct := 0.0
			if spent > 0 {
				pct = (compSpent / spent) * 100
			}
			componentCosts = append(componentCosts, ComponentCost{
				Component: comp,
				Spent:     compSpent,
				Percent:   pct,
			})
		}

		status := BudgetStatus{
			Period:         p.period,
			Limit:          p.limit,
			Spent:          spent,
			Remaining:      remaining,
			Percent:        percent,
			Allowed:        spent < p.limit,
			ComponentCosts: componentCosts,
		}

		statuses = append(statuses, status)
		if !status.Allowed {
			allowed = false
		}
	}

	return statuses, allowed, nil
}

// costLogEntry matches the JSONL format written by FileSink.
type costLogEntry struct {
	Timestamp string          `json:"timestamp"`
	Operation string          `json:"operation"`
	Provider  string          `json:"provider"`
	Model     string          `json:"model"`
	Component string          `json:"component"`
	Context   string          `json:"context"`
	Cost      json.RawMessage `json:"cost"`
}

type costValues struct {
	Total float64 `json:"total"`
}

// sumCosts reads the JSONL log and sums total cost within the time window.
func (c *BudgetChecker) sumCosts(start, end time.Time, project, model string) (float64, error) {
	f, err := os.Open(c.logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil // No log yet = no spending
		}
		return 0, err
	}
	defer f.Close()

	var total float64
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB line buffer

	for scanner.Scan() {
		var entry costLogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue // Skip malformed lines
		}

		ts, err := time.Parse("2006-01-02T15:04:05.000Z07:00", entry.Timestamp)
		if err != nil {
			continue
		}

		if ts.Before(start) || !ts.Before(end) {
			continue
		}

		// Filter by model if specified (empty model = all models)
		if model != "" && entry.Model != model {
			continue
		}

		// Filter by project context if specified
		if project != "" && entry.Context != "" && entry.Context != project {
			continue
		}

		var cv costValues
		if err := json.Unmarshal(entry.Cost, &cv); err != nil {
			continue
		}

		total += cv.Total
	}

	return total, scanner.Err()
}

// CostByComponent returns a map of component name to total cost within the given time window.
func (c *BudgetChecker) CostByComponent(start, end time.Time) (map[string]float64, error) {
	f, err := os.Open(c.logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	result := make(map[string]float64)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		var entry costLogEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}

		ts, err := time.Parse("2006-01-02T15:04:05.000Z07:00", entry.Timestamp)
		if err != nil {
			continue
		}

		if ts.Before(start) || !ts.Before(end) {
			continue
		}

		var cv costValues
		if err := json.Unmarshal(entry.Cost, &cv); err != nil {
			continue
		}

		component := entry.Component
		if component == "" {
			component = "untagged"
		}
		result[component] += cv.Total
	}

	return result, scanner.Err()
}

// isOverrideEnabled checks if the emergency budget override is active.
func isOverrideEnabled() bool {
	return os.Getenv("ENGRAM_BUDGET_OVERRIDE") == "true"
}
