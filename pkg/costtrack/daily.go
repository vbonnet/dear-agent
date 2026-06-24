package costtrack

import "time"

// DailyBudget implements the weekly-smoothed daily budget formula.
//
// Formula: on day N of the week (1-indexed), the available budget is:
//
//	available = (N * weeklyLimit / 7) - totalSpentInWeekSoFar
//
// This means unused budget from prior days carries forward automatically.
// If prior days overspent, today's budget shrinks accordingly.
// The total never exceeds weeklyLimit.
type DailyBudget struct {
	WeeklyLimit float64    // Total USD budget for the 7-day period
	WeekStart   time.Time  // Midnight of the first day (Monday or configured start)
	DailySpend  [7]float64 // Actual USD spent on each day of the week (index 0 = day 1)
}

// DayIndex returns the zero-based day index (0–6) within the week for t.
// Returns -1 if t is before WeekStart.
func (b *DailyBudget) DayIndex(t time.Time) int {
	if t.Before(b.WeekStart) {
		return -1
	}
	end := b.WeekStart.AddDate(0, 0, 7)
	if !t.Before(end) {
		return -1
	}
	return int(t.Sub(b.WeekStart).Hours() / 24)
}

// AvailableToday returns how much USD can be spent on the given day.
// It is clamped to [0, weeklyLimit - spent-so-far].
func (b *DailyBudget) AvailableToday(now time.Time) float64 {
	dayIdx := b.DayIndex(now)
	if dayIdx < 0 {
		return 0
	}
	// Days elapsed including today = dayIdx+1
	allocated := float64(dayIdx+1) * b.WeeklyLimit / 7
	if allocated > b.WeeklyLimit {
		allocated = b.WeeklyLimit
	}
	// Include today's recorded spend so this returns the remaining budget (not the total).
	var spentSoFar float64
	for i := 0; i <= dayIdx; i++ {
		spentSoFar += b.DailySpend[i]
	}
	avail := allocated - spentSoFar
	if avail < 0 {
		return 0
	}
	return avail
}

// SpentToday returns the recorded spend for the day containing now.
func (b *DailyBudget) SpentToday(now time.Time) float64 {
	idx := b.DayIndex(now)
	if idx < 0 || idx >= 7 {
		return 0
	}
	return b.DailySpend[idx]
}

// TotalSpent returns the sum of all daily spend in the week so far.
func (b *DailyBudget) TotalSpent() float64 {
	var total float64
	for _, v := range b.DailySpend {
		total += v
	}
	return total
}

// PercentUsedToday returns the fraction of today's budget already spent (0–100).
func (b *DailyBudget) PercentUsedToday(now time.Time) float64 {
	avail := b.AvailableToday(now)
	todaySpend := b.SpentToday(now)
	fullBudget := avail + todaySpend
	if fullBudget <= 0 {
		return 100
	}
	return todaySpend / fullBudget * 100
}
