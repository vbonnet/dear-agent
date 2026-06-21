package safegit

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// makeReviewerJSON builds gh pr view JSON with one commit (the head push) and
// the given reviews.
func makeReviewerJSON(pushedAt time.Time, reviews []struct {
	login       string
	submittedAt time.Time
}) []byte {
	type commitEntry struct {
		CommittedDate time.Time `json:"committedDate"`
	}
	type reviewEntry struct {
		Author struct {
			Login string `json:"login"`
		} `json:"author"`
		State       string    `json:"state"`
		SubmittedAt time.Time `json:"submittedAt"`
	}
	type doc struct {
		HeadRefOid string        `json:"headRefOid"`
		Commits    []commitEntry `json:"commits"`
		Reviews    []reviewEntry `json:"reviews"`
	}
	d := doc{HeadRefOid: "abc123"}
	if !pushedAt.IsZero() {
		d.Commits = []commitEntry{{CommittedDate: pushedAt}}
	}
	for _, r := range reviews {
		var re reviewEntry
		re.Author.Login = r.login
		re.State = "COMMENTED"
		re.SubmittedAt = r.submittedAt
		d.Reviews = append(d.Reviews, re)
	}
	b, _ := json.Marshal(d)
	return b
}

const geminiBot = "gemini-code-assist[bot]"

func reviewersFor() []ExpectedReviewer {
	return []ExpectedReviewer{{Login: geminiBot, TimeoutMinutes: 30, RequireNewerThanPush: true}}
}

func TestParseExpectedReviewers_FreshReviewPasses(t *testing.T) {
	now := time.Now()
	push := now.Add(-20 * time.Minute)
	data := makeReviewerJSON(push, []struct {
		login       string
		submittedAt time.Time
	}{
		{login: geminiBot, submittedAt: push.Add(2 * time.Minute)}, // after push
	})
	if err := parseExpectedReviewers(data, reviewersFor(), now); err != nil {
		t.Fatalf("fresh review should pass, got: %v", err)
	}
}

func TestParseExpectedReviewers_StaleReview(t *testing.T) {
	now := time.Now()
	push := now.Add(-20 * time.Minute)
	data := makeReviewerJSON(push, []struct {
		login       string
		submittedAt time.Time
	}{
		{login: geminiBot, submittedAt: push.Add(-5 * time.Minute)}, // before push
	})
	err := parseExpectedReviewers(data, reviewersFor(), now)
	if err == nil || !strings.Contains(err.Error(), "STALE") {
		t.Fatalf("expected STALE error, got: %v", err)
	}
}

func TestParseExpectedReviewers_NeverArrived(t *testing.T) {
	now := time.Now()
	push := now.Add(-45 * time.Minute) // past the 30m timeout
	data := makeReviewerJSON(push, nil)
	err := parseExpectedReviewers(data, reviewersFor(), now)
	if err == nil || !strings.Contains(err.Error(), "NEVER_ARRIVED") {
		t.Fatalf("expected NEVER_ARRIVED error, got: %v", err)
	}
}

func TestParseExpectedReviewers_PendingWithinTimeout(t *testing.T) {
	now := time.Now()
	push := now.Add(-5 * time.Minute) // still within 30m timeout
	data := makeReviewerJSON(push, nil)
	err := parseExpectedReviewers(data, reviewersFor(), now)
	if err == nil || !strings.Contains(err.Error(), "PENDING") {
		t.Fatalf("expected PENDING error (still waiting), got: %v", err)
	}
	// PENDING is never a silent pass.
	if err == nil {
		t.Fatal("absence of a review must never pass silently")
	}
}

func TestParseExpectedReviewers_OtherReviewerIgnored(t *testing.T) {
	now := time.Now()
	push := now.Add(-45 * time.Minute)
	data := makeReviewerJSON(push, []struct {
		login       string
		submittedAt time.Time
	}{
		{login: "some-human", submittedAt: now}, // not the expected bot
	})
	err := parseExpectedReviewers(data, reviewersFor(), now)
	if err == nil || !strings.Contains(err.Error(), "NEVER_ARRIVED") {
		t.Fatalf("a review from another login must not satisfy the bot, got: %v", err)
	}
}

func TestParseExpectedReviewers_FreshnessNotRequired(t *testing.T) {
	now := time.Now()
	push := now.Add(-20 * time.Minute)
	// Stale review, but require_newer_than_push is false → presence is enough.
	reviewers := []ExpectedReviewer{{Login: geminiBot, TimeoutMinutes: 30}}
	data := makeReviewerJSON(push, []struct {
		login       string
		submittedAt time.Time
	}{
		{login: geminiBot, submittedAt: push.Add(-10 * time.Minute)},
	})
	if err := parseExpectedReviewers(data, reviewers, now); err != nil {
		t.Fatalf("presence should pass when freshness not required, got: %v", err)
	}
}

func TestParseExpectedReviewers_NewestReviewWins(t *testing.T) {
	now := time.Now()
	push := now.Add(-20 * time.Minute)
	// An old stale review plus a newer fresh one → passes on the fresh one.
	data := makeReviewerJSON(push, []struct {
		login       string
		submittedAt time.Time
	}{
		{login: geminiBot, submittedAt: push.Add(-10 * time.Minute)},
		{login: geminiBot, submittedAt: push.Add(3 * time.Minute)},
	})
	if err := parseExpectedReviewers(data, reviewersFor(), now); err != nil {
		t.Fatalf("newest fresh review should pass, got: %v", err)
	}
}

func TestParseExpectedReviewers_NoReviewersConfigured(t *testing.T) {
	if err := parseExpectedReviewers([]byte(`{}`), nil, time.Now()); err != nil {
		t.Fatalf("no configured reviewers should pass, got: %v", err)
	}
}
