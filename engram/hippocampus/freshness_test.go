package hippocampus

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMemoryAgeDays_WithFrontmatterDate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.md")

	// Write a memory observed 10 days ago
	observed := time.Now().AddDate(0, 0, -10).Format("2006-01-02")
	content := "---\nname: test\nobserved: " + observed + "\ntype: project\n---\nSome memory content.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	days, err := MemoryAgeDays(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if days != 10 {
		t.Errorf("expected 10 days, got %d", days)
	}
}

func TestMemoryAgeDays_FallbackToMtime(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.md")

	// No frontmatter date
	content := "---\nname: test\ntype: user\n---\nNo observed date here.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// Set mtime to 7 days ago
	mtime := time.Now().Add(-7 * 24 * time.Hour)
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}

	days, err := MemoryAgeDays(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if days != 7 {
		t.Errorf("expected 7 days, got %d", days)
	}
}

func TestMemoryAgeDays_Today(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.md")

	// Observed today
	observed := time.Now().Format("2006-01-02")
	content := "---\nobserved: " + observed + "\n---\nFresh memory.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	days, err := MemoryAgeDays(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if days != 0 {
		t.Errorf("expected 0 days, got %d", days)
	}
}

func TestMemoryAgeDays_NoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.md")

	// No frontmatter at all, just plain content
	content := "Just plain text, no frontmatter.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// File just created, so mtime is ~now
	days, err := MemoryAgeDays(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if days != 0 {
		t.Errorf("expected 0 days for freshly created file, got %d", days)
	}
}

func TestMemoryAgeDays_FileNotFound(t *testing.T) {
	_, err := MemoryAgeDays("/nonexistent/path/memory.md")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestMemoryAgeDays_MalformedDate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.md")

	// Malformed date should fall back to mtime
	content := "---\nobserved: not-a-date\n---\nContent.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	days, err := MemoryAgeDays(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if days != 0 {
		t.Errorf("expected 0 days for freshly created file (malformed date fallback), got %d", days)
	}
}

func TestMemoryAge_ReturnsPositiveDuration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.md")

	observed := time.Now().AddDate(0, 0, -5).Format("2006-01-02")
	content := "---\nobserved: " + observed + "\n---\nContent.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	dur, err := MemoryAge(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dur < 4*24*time.Hour || dur > 6*24*time.Hour {
		t.Errorf("expected ~5 days duration, got %v", dur)
	}
}

func TestMemoryFreshnessText_ZeroDays(t *testing.T) {
	text := MemoryFreshnessText(0)
	if text != "" {
		t.Errorf("expected empty string for 0 days, got %q", text)
	}
}

func TestMemoryFreshnessText_OneDay(t *testing.T) {
	text := MemoryFreshnessText(1)
	if text != "" {
		t.Errorf("expected empty string for 1 day, got %q", text)
	}
}

func TestMemoryFreshnessText_SevenDays(t *testing.T) {
	text := MemoryFreshnessText(7)
	if text == "" {
		t.Fatal("expected non-empty caveat for 7 days")
	}
	expected := "This memory is 7 days old."
	if len(text) < len(expected) || text[:len(expected)] != expected {
		t.Errorf("expected caveat starting with %q, got %q", expected, text)
	}
}

func TestMemoryFreshnessText_ThirtyDays(t *testing.T) {
	text := MemoryFreshnessText(30)
	if text == "" {
		t.Fatal("expected non-empty caveat for 30 days")
	}
	expected := "This memory is 30 days old."
	if len(text) < len(expected) || text[:len(expected)] != expected {
		t.Errorf("expected caveat starting with %q, got %q", expected, text)
	}
}

func TestMemoryFreshnessText_NinetyDays(t *testing.T) {
	text := MemoryFreshnessText(90)
	if text == "" {
		t.Fatal("expected non-empty caveat for 90 days")
	}
	expected := "This memory is 90 days old."
	if len(text) < len(expected) || text[:len(expected)] != expected {
		t.Errorf("expected caveat starting with %q, got %q", expected, text)
	}
}

func TestParseFrontmatterDate_ValidDate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.md")

	content := "---\nname: test\nobserved: 2026-03-15\ntype: project\n---\nContent.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := parseFrontmatterDate(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := time.Date(2026, 3, 15, 0, 0, 0, 0, time.Local)
	if !got.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, got)
	}
}

func TestParseFrontmatterDate_NoDate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.md")

	content := "---\nname: test\ntype: user\n---\nContent.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := parseFrontmatterDate(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.IsZero() {
		t.Errorf("expected zero time, got %v", got)
	}
}

func TestParseFrontmatterDate_NoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.md")

	content := "Just plain text.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := parseFrontmatterDate(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.IsZero() {
		t.Errorf("expected zero time for no frontmatter, got %v", got)
	}
}

func TestParseFrontmatterDate_InlineDateFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.md")

	// Autodream writes dates inline: "- content (YYYY-MM-DD)"
	content := "---\nname: test\ntype: project\n---\n- Some correction (2026-03-20)\n- Another thing (2026-03-25)\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := parseFrontmatterDate(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should return the most recent inline date
	expected := time.Date(2026, 3, 25, 0, 0, 0, 0, time.Local)
	if !got.Equal(expected) {
		t.Errorf("expected %v (latest inline date), got %v", expected, got)
	}
}

func TestParseFrontmatterDate_InlineDateNoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.md")

	// No frontmatter at all, just autodream inline entries
	content := "- Use snake_case (2026-03-15)\n- Prefer short names (2026-03-10)\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := parseFrontmatterDate(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := time.Date(2026, 3, 15, 0, 0, 0, 0, time.Local)
	if !got.Equal(expected) {
		t.Errorf("expected %v (latest inline date), got %v", expected, got)
	}
}

func TestParseFrontmatterDate_FrontmatterDateTakesPrecedence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.md")

	// Both frontmatter observed: and inline dates — frontmatter wins
	content := "---\nobserved: 2026-04-01\n---\n- Some entry (2026-03-25)\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := parseFrontmatterDate(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := time.Date(2026, 4, 1, 0, 0, 0, 0, time.Local)
	if !got.Equal(expected) {
		t.Errorf("expected frontmatter date %v to take precedence, got %v", expected, got)
	}
}

func TestParseFrontmatterDate_InlineDateMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.md")

	// Malformed inline dates should be skipped
	content := "- Some entry (not-a-date)\n- Another entry (2026-13-45)\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := parseFrontmatterDate(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.IsZero() {
		t.Errorf("expected zero time for malformed inline dates, got %v", got)
	}
}

func TestSurfaceMemoryWithFreshness_StaleFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.md")

	observed := time.Now().AddDate(0, 0, -10).Format("2006-01-02")
	content := "---\nobserved: " + observed + "\n---\nSome old memory.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := SurfaceMemoryWithFreshness(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "<system-reminder>") {
		t.Error("expected system-reminder wrapper for stale memory")
	}
	if !strings.Contains(result, "10 days old") {
		t.Error("expected age caveat mentioning 10 days")
	}
	if !strings.Contains(result, "Some old memory.") {
		t.Error("expected original content to be preserved")
	}
}

func TestSurfaceMemoryWithFreshness_FreshFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "memory.md")

	observed := time.Now().Format("2006-01-02")
	content := "---\nobserved: " + observed + "\n---\nFresh memory.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := SurfaceMemoryWithFreshness(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(result, "<system-reminder>") {
		t.Error("fresh memory should not have system-reminder wrapper")
	}
	if result != content {
		t.Errorf("expected unchanged content for fresh memory")
	}
}
