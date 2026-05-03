// Package hippocampus implements the memory subsystem for engram.
package hippocampus

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"regexp"
	"strings"
	"time"
)

// MemoryAgeDays returns the age of a memory file in days.
// It first tries to parse an "observed: YYYY-MM-DD" date from YAML frontmatter.
// If not found, it falls back to the file's modification time.
// The result is floor-rounded and clamped to >= 0.
func MemoryAgeDays(path string) (int, error) {
	dur, err := MemoryAge(path)
	if err != nil {
		return 0, err
	}
	days := int(math.Floor(dur.Hours() / 24))
	if days < 0 {
		return 0, nil
	}
	return days, nil
}

// MemoryAge returns the duration since the memory was last observed or modified.
// It first checks for an "observed: YYYY-MM-DD" field in YAML frontmatter.
// If not found, it uses the file's modification time.
func MemoryAge(path string) (time.Duration, error) {
	observed, err := parseFrontmatterDate(path)
	if err != nil {
		return 0, fmt.Errorf("reading memory file: %w", err)
	}
	if !observed.IsZero() {
		return time.Since(observed), nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("stat memory file: %w", err)
	}
	return time.Since(info.ModTime()), nil
}

// MemoryFreshnessText returns a staleness caveat for memories older than 1 day.
// Returns empty string if the memory is fresh (<= 1 day old).
func MemoryFreshnessText(ageDays int) string {
	if ageDays <= 1 {
		return ""
	}
	return fmt.Sprintf(
		"This memory is %d days old. Memories are point-in-time observations, "+
			"not live state — claims about code behavior or file:line citations "+
			"may be outdated. Verify against current code before asserting as fact.",
		ageDays,
	)
}

// SurfaceMemoryWithFreshness reads a memory file and wraps it with a staleness
// caveat if the memory is older than 1 day. The caveat is wrapped in
// <system-reminder> tags so the agent sees it alongside the memory content.
// Returns the original content unchanged if the memory is fresh.
func SurfaceMemoryWithFreshness(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read memory file: %w", err)
	}

	days, err := MemoryAgeDays(path)
	if err != nil {
		// If we can't determine age, return content without caveat
		return string(content), nil //nolint:nilerr // intentional: caller signals via separate bool/optional
	}

	caveat := MemoryFreshnessText(days)
	if caveat == "" {
		return string(content), nil
	}

	return fmt.Sprintf("<system-reminder>\n%s\n</system-reminder>\n%s", caveat, string(content)), nil
}

// inlineDatePattern matches dates in autodream's inline format: "- content (YYYY-MM-DD)"
var inlineDatePattern = regexp.MustCompile(`\((\d{4}-\d{2}-\d{2})\)\s*$`)

// parseFrontmatterDate extracts a date from a memory file.
// It first checks for "observed: YYYY-MM-DD" in YAML frontmatter.
// If not found, it scans the file body for autodream's inline date format
// "(YYYY-MM-DD)" at the end of lines, returning the most recent date found.
// Returns zero time if no date is found.
func parseFrontmatterDate(path string) (time.Time, error) {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	inFrontmatter := false
	pastFrontmatter := false
	var latestInline time.Time

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == "---" {
			if !inFrontmatter && !pastFrontmatter {
				inFrontmatter = true
				continue
			}
			if inFrontmatter {
				// End of frontmatter
				inFrontmatter = false
				pastFrontmatter = true
				continue
			}
		}

		if inFrontmatter {
			if strings.HasPrefix(line, "observed:") {
				dateStr := strings.TrimSpace(strings.TrimPrefix(line, "observed:"))
				// Parse in local zone — date-only strings authored by humans
				// represent "that day" wherever they are, not UTC midnight. Without
				// this, age calculations skew by the local timezone offset and
				// flip "10 days old" to "11 days old" west of UTC.
				t, err := time.ParseInLocation("2006-01-02", dateStr, time.Local)
				if err != nil {
					continue // malformed date, skip
				}
				return t, nil
			}
			continue
		}

		// Scan body for autodream inline dates: "- content (YYYY-MM-DD)"
		if matches := inlineDatePattern.FindStringSubmatch(line); len(matches) == 2 {
			t, err := time.ParseInLocation("2006-01-02", matches[1], time.Local)
			if err != nil {
				continue
			}
			if t.After(latestInline) {
				latestInline = t
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return time.Time{}, err
	}

	return latestInline, nil
}
