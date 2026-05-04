package costtrack

import (
	"fmt"
	"log/slog"
	"sync"
)

// AlertThresholds are the budget-utilization percentages at which alerts fire.
var AlertThresholds = []float64{50, 75, 90, 100}

// AlertManager tracks which alerts have been sent to avoid duplicates.
type AlertManager struct {
	mu     sync.Mutex
	sent   map[string]bool // key: "project:model:period:threshold"
	logger *slog.Logger
}

// NewAlertManager creates an alert manager with the given logger.
func NewAlertManager(logger *slog.Logger) *AlertManager {
	return &AlertManager{
		sent:   make(map[string]bool),
		logger: logger,
	}
}

// CheckAndAlert evaluates budget statuses and emits alerts at threshold crossings.
// Returns the list of alerts that were newly fired.
func (a *AlertManager) CheckAndAlert(project, model string, statuses []BudgetStatus) []string {
	a.mu.Lock()
	defer a.mu.Unlock()

	var fired []string

	for _, status := range statuses {
		for _, threshold := range AlertThresholds {
			if status.Percent < threshold {
				continue
			}

			key := fmt.Sprintf("%s:%s:%s:%.0f", project, model, status.Period, threshold)
			if a.sent[key] {
				continue // Already sent this alert
			}

			a.sent[key] = true

			msg := fmt.Sprintf(
				"[BUDGET_ALERT] %s budget %.0f%% threshold reached for project=%q model=%q: $%.2f / $%.2f (%.1f%%)",
				status.Period, threshold, project, model, status.Spent, status.Limit, status.Percent,
			)

			switch {
			case threshold >= 100:
				a.logger.Error(msg)
			case threshold >= 90:
				a.logger.Warn(msg)
			default:
				a.logger.Info(msg)
			}

			fired = append(fired, msg)
		}
	}

	return fired
}

// Reset clears all sent alert state (e.g., at period boundaries or for testing).
func (a *AlertManager) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sent = make(map[string]bool)
}

// ResetForPeriod clears alerts for a specific period (for period rollovers).
func (a *AlertManager) ResetForPeriod(period BudgetPeriod) {
	a.mu.Lock()
	defer a.mu.Unlock()

	for key := range a.sent {
		// Keys contain the period: "project:model:period:threshold"
		if containsPeriod(key, period) {
			delete(a.sent, key)
		}
	}
}

func containsPeriod(key string, period BudgetPeriod) bool {
	// Simple substring match — period is always the third colon-delimited field
	target := ":" + string(period) + ":"
	for i := 0; i <= len(key)-len(target); i++ {
		if key[i:i+len(target)] == target {
			return true
		}
	}
	return false
}
