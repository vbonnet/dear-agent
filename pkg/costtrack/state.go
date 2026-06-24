package costtrack

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// BudgetState is the persisted budget ledger. It survives process restart
// and is the single source of truth for what has been spent this week.
type BudgetState struct {
	WeeklyLimitUSD float64           `json:"weekly_limit_usd"`
	WeekStart      time.Time         `json:"week_start"`
	DailySpend     [7]float64        `json:"daily_spend"`
	SessionTokens  map[string]int64  `json:"session_tokens"`  // sessionID -> total tokens consumed
	SessionHarness map[string]string `json:"session_harness"` // sessionID -> harness name
	UpdatedAt      time.Time         `json:"updated_at"`

	path string
	mu   sync.Mutex
}

// currentWeekStart returns midnight UTC of the Monday of the week containing t.
func currentWeekStart(t time.Time) time.Time {
	t = t.UTC()
	weekday := int(t.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday → 7 so Monday = 1
	}
	return time.Date(t.Year(), t.Month(), t.Day()-(weekday-1), 0, 0, 0, 0, time.UTC)
}

// LoadBudgetState loads state from path, creating a fresh state if the file
// doesn't exist or belongs to a prior week.
func LoadBudgetState(path string, weeklyLimitUSD float64) (*BudgetState, error) {
	s := &BudgetState{path: path}

	data, err := os.ReadFile(path)
	if err == nil {
		if jsonErr := json.Unmarshal(data, s); jsonErr != nil {
			return nil, fmt.Errorf("parse budget state: %w", jsonErr)
		}
		s.path = path
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read budget state: %w", err)
	}

	now := time.Now().UTC()
	thisWeek := currentWeekStart(now)

	// Reset if the stored week has expired or limit changed.
	if s.WeekStart.IsZero() || s.WeekStart.Before(thisWeek) || s.WeeklyLimitUSD != weeklyLimitUSD {
		s.WeeklyLimitUSD = weeklyLimitUSD
		s.WeekStart = thisWeek
		s.DailySpend = [7]float64{}
		s.SessionTokens = nil
		s.SessionHarness = nil
	}

	if s.SessionTokens == nil {
		s.SessionTokens = make(map[string]int64)
	}
	if s.SessionHarness == nil {
		s.SessionHarness = make(map[string]string)
	}

	return s, nil
}

// Save writes the state to disk atomically.
func (s *BudgetState) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked()
}

func (s *BudgetState) saveLocked() error {
	s.UpdatedAt = time.Now().UTC()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal budget state: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create budget state dir: %w", err)
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write budget state temp: %w", err)
	}
	// If Rename fails, clean up the orphaned .tmp file.
	defer os.Remove(tmp) // no-op on success (file was renamed away); removes on failure
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("rename budget state: %w", err)
	}
	return nil
}

// RecordSpend adds usd to today's daily spend and saves state.
func (s *BudgetState) RecordSpend(now time.Time, usd float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	b := DailyBudget{WeeklyLimit: s.WeeklyLimitUSD, WeekStart: s.WeekStart, DailySpend: s.DailySpend}
	idx := b.DayIndex(now.UTC())
	if idx >= 0 && idx < 7 {
		s.DailySpend[idx] += usd
	}
	return s.saveLocked()
}

// RecordSpendAndTokens records daily spend and session tokens in a single atomic
// disk write, avoiding the double-write overhead of calling RecordSpend and
// RecordSessionTokens separately.
func (s *BudgetState) RecordSpendAndTokens(now time.Time, usd float64, sessionID, harness string, tokens int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	b := DailyBudget{WeeklyLimit: s.WeeklyLimitUSD, WeekStart: s.WeekStart, DailySpend: s.DailySpend}
	idx := b.DayIndex(now.UTC())
	if idx >= 0 && idx < 7 {
		s.DailySpend[idx] += usd
	}
	s.SessionTokens[sessionID] += tokens
	if harness != "" {
		s.SessionHarness[sessionID] = harness
	}
	return s.saveLocked()
}

// RecordSessionTokens adds tokens to the session's running total and saves state.
func (s *BudgetState) RecordSessionTokens(sessionID, harness string, tokens int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.SessionTokens[sessionID] += tokens
	if harness != "" {
		s.SessionHarness[sessionID] = harness
	}
	return s.saveLocked()
}

// SessionTokensUsed returns total tokens consumed by sessionID.
func (s *BudgetState) SessionTokensUsed(sessionID string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.SessionTokens[sessionID]
}

// TodayBudget returns the computed available budget for today.
func (s *BudgetState) TodayBudget(now time.Time) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := DailyBudget{WeeklyLimit: s.WeeklyLimitUSD, WeekStart: s.WeekStart, DailySpend: s.DailySpend}
	return b.AvailableToday(now.UTC())
}

// TodaySpend returns what has already been spent today.
func (s *BudgetState) TodaySpend(now time.Time) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := DailyBudget{WeeklyLimit: s.WeeklyLimitUSD, WeekStart: s.WeekStart, DailySpend: s.DailySpend}
	return b.SpentToday(now.UTC())
}

// Snapshot returns a read-only copy of the daily budget for status display.
func (s *BudgetState) Snapshot() DailyBudget {
	s.mu.Lock()
	defer s.mu.Unlock()
	return DailyBudget{
		WeeklyLimit: s.WeeklyLimitUSD,
		WeekStart:   s.WeekStart,
		DailySpend:  s.DailySpend,
	}
}
