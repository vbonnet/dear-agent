package costtrack

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// BudgetPeriod represents the time window for a budget limit.
type BudgetPeriod string

// Recognized BudgetPeriod values.
const (
	BudgetDaily   BudgetPeriod = "daily"
	BudgetWeekly  BudgetPeriod = "weekly"
	BudgetMonthly BudgetPeriod = "monthly"
)

// BudgetConfig is the top-level budget configuration loaded from YAML.
type BudgetConfig struct {
	Defaults *BudgetLimits            `yaml:"defaults"`
	Projects map[string]*BudgetLimits `yaml:"projects"`
	Models   map[string]*BudgetLimits `yaml:"models"`
}

// BudgetLimits defines spending limits for a budget scope.
type BudgetLimits struct {
	Daily   float64 `yaml:"daily"`   // Max USD per day
	Weekly  float64 `yaml:"weekly"`  // Max USD per week
	Monthly float64 `yaml:"monthly"` // Max USD per month
}

// ComponentCost tracks spending for a single component within a budget period.
type ComponentCost struct {
	Component string  `json:"component"`
	Spent     float64 `json:"spent"`
	Percent   float64 `json:"percent"` // Percentage of total spent
}

// BudgetStatus is returned by CheckBudget with current spend vs limits.
type BudgetStatus struct {
	Period         BudgetPeriod    `json:"period"`
	Limit          float64         `json:"limit"`
	Spent          float64         `json:"spent"`
	Remaining      float64         `json:"remaining"`
	Percent        float64         `json:"percent"` // 0-100
	Allowed        bool            `json:"allowed"`
	ComponentCosts []ComponentCost `json:"component_costs,omitempty"`
}

// LoadBudgetConfig reads budget configuration from a YAML file.
func LoadBudgetConfig(path string) (*BudgetConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read budget config: %w", err)
	}

	var cfg BudgetConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse budget config: %w", err)
	}

	if cfg.Defaults == nil {
		cfg.Defaults = &BudgetLimits{}
	}

	return &cfg, nil
}

// GetLimits returns the effective limits for a project/model combination.
// Model-specific limits override project limits, which override defaults.
func (c *BudgetConfig) GetLimits(project, model string) *BudgetLimits {
	result := &BudgetLimits{}

	// Start with defaults
	if c.Defaults != nil {
		*result = *c.Defaults
	}

	// Override with project-specific
	if p, ok := c.Projects[project]; ok && p != nil {
		if p.Daily > 0 {
			result.Daily = p.Daily
		}
		if p.Weekly > 0 {
			result.Weekly = p.Weekly
		}
		if p.Monthly > 0 {
			result.Monthly = p.Monthly
		}
	}

	// Override with model-specific
	if m, ok := c.Models[model]; ok && m != nil {
		if m.Daily > 0 {
			result.Daily = m.Daily
		}
		if m.Weekly > 0 {
			result.Weekly = m.Weekly
		}
		if m.Monthly > 0 {
			result.Monthly = m.Monthly
		}
	}

	return result
}

// PeriodBounds returns the start and end time for a budget period relative to now.
func PeriodBounds(period BudgetPeriod, now time.Time) (start, end time.Time) {
	switch period {
	case BudgetDaily:
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		end = start.AddDate(0, 0, 1)
	case BudgetWeekly:
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7 // Sunday = 7
		}
		start = time.Date(now.Year(), now.Month(), now.Day()-weekday+1, 0, 0, 0, 0, now.Location())
		end = start.AddDate(0, 0, 7)
	case BudgetMonthly:
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		end = start.AddDate(0, 1, 0)
	default:
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		end = start.AddDate(0, 0, 1)
	}
	return
}
