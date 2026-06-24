package costtrack

import (
	"testing"
	"time"
)

func weekStart(year, month, day int) time.Time {
	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}

func TestDailyBudget_AvailableToday(t *testing.T) {
	ws := weekStart(2026, 6, 16) // Monday

	t.Run("day1_no_spend", func(t *testing.T) {
		b := DailyBudget{WeeklyLimit: 70, WeekStart: ws}
		day1 := ws.Add(time.Hour * 12) // noon day 1
		got := b.AvailableToday(day1)
		if got != 10 {
			t.Errorf("want 10, got %.2f", got)
		}
	})

	t.Run("day2_unused_from_day1_carries_forward", func(t *testing.T) {
		b := DailyBudget{WeeklyLimit: 70, WeekStart: ws}
		b.DailySpend[0] = 5 // spent $5 on day 1 (unused = $5)
		day2 := ws.AddDate(0, 0, 1).Add(time.Hour * 12)
		got := b.AvailableToday(day2)
		// allocated through day 2 = 2/7 * 70 = 20; spent so far = 5 → available = 15
		if got != 15 {
			t.Errorf("want 15, got %.2f", got)
		}
	})

	t.Run("day2_overspent_day1_reduces_budget", func(t *testing.T) {
		b := DailyBudget{WeeklyLimit: 70, WeekStart: ws}
		b.DailySpend[0] = 18 // overspent day 1 (over by $8)
		day2 := ws.AddDate(0, 0, 1).Add(time.Hour * 12)
		got := b.AvailableToday(day2)
		// allocated through day 2 = 20; spent day 1 = 18; available = 20 - 18 = 2
		if got != 2 {
			t.Errorf("want 2, got %.2f", got)
		}
	})

	t.Run("day7_gets_all_remaining", func(t *testing.T) {
		b := DailyBudget{WeeklyLimit: 70, WeekStart: ws}
		// Spend exactly 10/day for days 1-6 = 60 total
		for i := range 6 {
			b.DailySpend[i] = 10
		}
		day7 := ws.AddDate(0, 0, 6).Add(time.Hour * 12)
		got := b.AvailableToday(day7)
		// allocated = 70; spent so far = 60 → available = 10
		if got != 10 {
			t.Errorf("want 10, got %.2f", got)
		}
	})

	t.Run("day7_with_full_carryover", func(t *testing.T) {
		b := DailyBudget{WeeklyLimit: 70, WeekStart: ws}
		// Spent nothing for days 1-6
		day7 := ws.AddDate(0, 0, 6).Add(time.Hour * 12)
		got := b.AvailableToday(day7)
		// allocated = 70; spent = 0 → available = 70
		if got != 70 {
			t.Errorf("want 70, got %.2f", got)
		}
	})

	t.Run("before_week_start_returns_zero", func(t *testing.T) {
		b := DailyBudget{WeeklyLimit: 70, WeekStart: ws}
		before := ws.Add(-time.Hour)
		got := b.AvailableToday(before)
		if got != 0 {
			t.Errorf("want 0, got %.2f", got)
		}
	})

	t.Run("after_week_end_returns_zero", func(t *testing.T) {
		b := DailyBudget{WeeklyLimit: 70, WeekStart: ws}
		after := ws.AddDate(0, 0, 7).Add(time.Hour)
		got := b.AvailableToday(after)
		if got != 0 {
			t.Errorf("want 0, got %.2f", got)
		}
	})

	t.Run("clamped_to_zero_when_overspent_all_days", func(t *testing.T) {
		b := DailyBudget{WeeklyLimit: 70, WeekStart: ws}
		// Overspent across first 6 days
		for i := range 6 {
			b.DailySpend[i] = 15 // total = 90, over weekly limit
		}
		day7 := ws.AddDate(0, 0, 6).Add(time.Hour * 12)
		got := b.AvailableToday(day7)
		if got != 0 {
			t.Errorf("want 0 (clamped), got %.2f", got)
		}
	})
}

func TestDailyBudget_DayIndex(t *testing.T) {
	ws := weekStart(2026, 6, 16)
	b := DailyBudget{WeeklyLimit: 70, WeekStart: ws}

	tests := []struct {
		offset time.Duration
		want   int
	}{
		{0, 0},
		{23*time.Hour + 59*time.Minute, 0},
		{24 * time.Hour, 1},
		{6*24*time.Hour + 23*time.Hour, 6},
		{7 * 24 * time.Hour, -1},
		{-time.Hour, -1},
	}
	for _, tc := range tests {
		got := b.DayIndex(ws.Add(tc.offset))
		if got != tc.want {
			t.Errorf("offset %v: want %d, got %d", tc.offset, tc.want, got)
		}
	}
}

func TestDailyBudget_PercentUsedToday(t *testing.T) {
	ws := weekStart(2026, 6, 16)
	b := DailyBudget{WeeklyLimit: 70, WeekStart: ws}
	b.DailySpend[0] = 5 // spent $5 of the $10 day-1 slot

	day1 := ws.Add(12 * time.Hour)
	pct := b.PercentUsedToday(day1)
	// today budget = 10; today spend = 5 → 50%
	if pct < 49.9 || pct > 50.1 {
		t.Errorf("want ~50%%, got %.2f%%", pct)
	}
}
