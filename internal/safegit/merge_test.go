package safegit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- parseCheckRuns ---

func TestParseCheckRuns_AllChecksPass(t *testing.T) {
	data := marshalJSON([]checkRun{
		{Name: "Build", State: "success"},
		{Name: "Lint", State: "pass"},
		{Name: "Optional", State: "success"},
	})
	if err := parseCheckRuns(data); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestParseCheckRuns_NonRequiredFails(t *testing.T) {
	// parseCheckRuns validates every check in its input.
	// The caller (checkAllCI) pre-filters to required checks in production, so
	// non-required fleet-wide failures never reach this function in practice.
	data := marshalJSON([]checkRun{
		{Name: "Build", State: "success"},
		{Name: "Optional", State: "failure"},
	})
	err := parseCheckRuns(data)
	if err == nil {
		t.Fatal("expected error: non-required failing check should also block merge")
	}
	if !strings.Contains(err.Error(), "Optional") {
		t.Errorf("error should mention failing check name, got: %v", err)
	}
}

func TestParseCheckRuns_RequiredFails(t *testing.T) {
	data := marshalJSON([]checkRun{
		{Name: "Build", State: "failure"},
		{Name: "Lint", State: "success"},
	})
	err := parseCheckRuns(data)
	if err == nil {
		t.Fatal("expected error for failing required check, got nil")
	}
	if !strings.Contains(err.Error(), "Build") {
		t.Errorf("error should mention failing check name, got: %v", err)
	}
}

func TestParseCheckRuns_RequiredPending(t *testing.T) {
	data := marshalJSON([]checkRun{
		{Name: "Build", State: "pending"},
	})
	err := parseCheckRuns(data)
	if err == nil {
		t.Fatal("expected error for pending required check, got nil")
	}
	if !strings.Contains(err.Error(), "pending") {
		t.Errorf("error should mention 'pending', got: %v", err)
	}
}

func TestParseCheckRuns_SkippingIsOK(t *testing.T) {
	data := marshalJSON([]checkRun{
		{Name: "Optional", State: "skipping"},
	})
	if err := parseCheckRuns(data); err != nil {
		t.Fatalf("skipping check should be acceptable, got: %v", err)
	}
}

func TestParseCheckRuns_SkippedIsOK(t *testing.T) {
	// gh pr checks --json returns "SKIPPED" (uppercase) for skipped checks.
	data := marshalJSON([]checkRun{
		{Name: "Generate SBOM", State: "SKIPPED"},
	})
	if err := parseCheckRuns(data); err != nil {
		t.Fatalf("SKIPPED check should be acceptable, got: %v", err)
	}
}

func TestParseCheckRuns_NeutralIsOK(t *testing.T) {
	data := marshalJSON([]checkRun{
		{Name: "Benchmark", State: "neutral"},
	})
	if err := parseCheckRuns(data); err != nil {
		t.Fatalf("neutral check should be acceptable, got: %v", err)
	}
}

func TestParseCheckRuns_Empty(t *testing.T) {
	data := marshalJSON([]checkRun{})
	if err := parseCheckRuns(data); err != nil {
		t.Fatalf("empty check list should pass, got: %v", err)
	}
}

// --- parseReviewThreads ---

type reviewThreadsDoc struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				ReviewThreads struct {
					Nodes []struct {
						IsResolved bool `json:"isResolved"`
						IsOutdated bool `json:"isOutdated"`
						Comments   struct {
							Nodes []struct {
								Body string `json:"body"`
							} `json:"nodes"`
						} `json:"comments"`
					} `json:"nodes"`
				} `json:"reviewThreads"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

func makeReviewJSON(threads []struct {
	resolved bool
	outdated bool
	body     string
}) []byte {
	var doc reviewThreadsDoc
	for _, t := range threads {
		node := struct {
			IsResolved bool `json:"isResolved"`
			IsOutdated bool `json:"isOutdated"`
			Comments   struct {
				Nodes []struct {
					Body string `json:"body"`
				} `json:"nodes"`
			} `json:"comments"`
		}{
			IsResolved: t.resolved,
			IsOutdated: t.outdated,
		}
		if t.body != "" {
			node.Comments.Nodes = append(node.Comments.Nodes, struct {
				Body string `json:"body"`
			}{Body: t.body})
		}
		doc.Data.Repository.PullRequest.ReviewThreads.Nodes = append(
			doc.Data.Repository.PullRequest.ReviewThreads.Nodes, node)
	}
	b, _ := json.Marshal(doc)
	return b
}

func TestParseReviewThreads_AllResolved(t *testing.T) {
	data := makeReviewJSON([]struct {
		resolved bool
		outdated bool
		body     string
	}{
		{resolved: true},
		{resolved: false, outdated: true},
	})
	if err := parseReviewThreads(data); err != nil {
		t.Fatalf("all threads resolved/outdated should pass, got: %v", err)
	}
}

func TestParseReviewThreads_UnresolvedThread(t *testing.T) {
	data := makeReviewJSON([]struct {
		resolved bool
		outdated bool
		body     string
	}{
		{resolved: false, outdated: false, body: "Please fix the nil check"},
	})
	err := parseReviewThreads(data)
	if err == nil {
		t.Fatal("expected error for unresolved thread, got nil")
	}
	if !strings.Contains(err.Error(), "unresolved") {
		t.Errorf("error should mention 'unresolved', got: %v", err)
	}
}

func TestParseReviewThreads_Empty(t *testing.T) {
	data := makeReviewJSON(nil)
	if err := parseReviewThreads(data); err != nil {
		t.Fatalf("no threads should pass, got: %v", err)
	}
}

// --- parseSoak ---

func makeSoakJSON(committedAt time.Time, botLogin string) []byte {
	type commitEntry struct {
		CommittedDate time.Time `json:"committedDate"`
	}
	type reviewEntry struct {
		Author struct {
			Login string `json:"login"`
		} `json:"author"`
		State string `json:"state"`
	}
	type doc struct {
		HeadRefOid string        `json:"headRefOid"`
		Commits    []commitEntry `json:"commits"`
		Reviews    []reviewEntry `json:"reviews"`
	}
	d := doc{HeadRefOid: "abc123"}
	if !committedAt.IsZero() {
		d.Commits = []commitEntry{{CommittedDate: committedAt}}
	}
	if botLogin != "" {
		r := reviewEntry{State: "APPROVED"}
		r.Author.Login = botLogin
		d.Reviews = []reviewEntry{r}
	}
	b, _ := json.Marshal(d)
	return b
}

func TestParseSoak_OldCommitWithBotReview(t *testing.T) {
	now := time.Now()
	past := now.Add(-10 * time.Minute)
	data := makeSoakJSON(past, ReviewBot)
	if err := parseSoak(data, now, false); err != nil {
		t.Fatalf("old commit with bot review should pass, got: %v", err)
	}
}

func TestParseSoak_TooRecent(t *testing.T) {
	now := time.Now()
	recent := now.Add(-1 * time.Minute)
	data := makeSoakJSON(recent, ReviewBot)
	err := parseSoak(data, now, false)
	if err == nil {
		t.Fatal("expected error for too-recent commit, got nil")
	}
	if !strings.Contains(err.Error(), "retry in") {
		t.Errorf("error should mention retry time, got: %v", err)
	}
}

func TestParseSoak_NoBotReview(t *testing.T) {
	now := time.Now()
	past := now.Add(-10 * time.Minute)
	data := makeSoakJSON(past, "")
	err := parseSoak(data, now, false)
	if err == nil {
		t.Fatal("expected error for missing bot review, got nil")
	}
	if !strings.Contains(err.Error(), ReviewBot) {
		t.Errorf("error should name the review bot, got: %v", err)
	}
}

func TestParseSoak_OtherBotReview(t *testing.T) {
	now := time.Now()
	past := now.Add(-10 * time.Minute)
	data := makeSoakJSON(past, "some-other-bot")
	err := parseSoak(data, now, false)
	if err == nil {
		t.Fatal("expected error when wrong bot reviewed, got nil")
	}
}

func TestParseSoak_SkipBotReview(t *testing.T) {
	now := time.Now()
	past := now.Add(-10 * time.Minute)
	// No bot review, but skip=true should still pass.
	data := makeSoakJSON(past, "")
	if err := parseSoak(data, now, true); err != nil {
		t.Fatalf("skip bot review should pass even with no review, got: %v", err)
	}
}

func TestParseSoak_SkipBotReview_StillChecksSoak(t *testing.T) {
	now := time.Now()
	recent := now.Add(-1 * time.Minute)
	// skipBotReview=true does NOT skip the soak-time check.
	data := makeSoakJSON(recent, "")
	err := parseSoak(data, now, true)
	if err == nil {
		t.Fatal("expected soak-time error even with skip-bot-review, got nil")
	}
	if !strings.Contains(err.Error(), "retry in") {
		t.Errorf("error should mention retry time, got: %v", err)
	}
}

// --- SafeMerge config validation ---

func TestSafeMerge_InvalidConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  MergeConfig
		want string
	}{
		{"zero pr", MergeConfig{PRNumber: 0, Repo: "a/b"}, "--pr must be"},
		{"missing repo", MergeConfig{PRNumber: 1, Repo: ""}, "--repo is required"},
		{"bad repo format", MergeConfig{PRNumber: 1, Repo: "nodash"}, "owner/repo format"},
		{"skip-bot-review without reason", MergeConfig{PRNumber: 1, Repo: "a/b", SkipBotReview: true}, "--skip-bot-review requires --skip-bot-review-reason"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := SafeMerge(tc.cfg)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

// --- appendAuditEntry / auditLogDir ---

func TestAuditEntry_WrittenToFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SAFE_MERGE_AUDIT_DIR", dir)

	appendAuditEntry("owner/repo", 42, "merged", "squash merge complete")

	data, err := os.ReadFile(filepath.Join(dir, "safe-merge-audit.jsonl"))
	if err != nil {
		t.Fatalf("audit log not created: %v", err)
	}
	if !strings.Contains(string(data), `"merged"`) {
		t.Errorf("audit log should contain event type, got: %s", data)
	}
	if !strings.Contains(string(data), `"pr":42`) {
		t.Errorf("audit log should contain PR number, got: %s", data)
	}
}

func TestAuditEntry_MultipleEntries(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SAFE_MERGE_AUDIT_DIR", dir)

	appendAuditEntry("owner/repo", 1, "gate_check", "CI: checks failed")
	appendAuditEntry("owner/repo", 1, "merged", "ok")

	data, err := os.ReadFile(filepath.Join(dir, "safe-merge-audit.jsonl"))
	if err != nil {
		t.Fatalf("audit log not created: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 audit entries, got %d: %s", len(lines), data)
	}
}

// --- BuildMergeArgs (TOCTOU anchor) ---

func TestBuildMergeArgs_ContainsMatchHeadCommit(t *testing.T) {
	args := BuildMergeArgs(42, "owner/repo", "abc123def456")

	foundFlag := false
	foundSHA := false
	for i, a := range args {
		if a == "--match-head-commit" {
			foundFlag = true
			if i+1 < len(args) && args[i+1] == "abc123def456" {
				foundSHA = true
			}
		}
	}
	if !foundFlag {
		t.Fatal("BuildMergeArgs must include --match-head-commit to prevent TOCTOU " +
			"(PR #460 regression); got: " + fmt.Sprint(args))
	}
	if !foundSHA {
		t.Fatal("--match-head-commit must be followed by the head SHA; got: " + fmt.Sprint(args))
	}
}

func TestBuildMergeArgs_RequiredFlags(t *testing.T) {
	args := BuildMergeArgs(99, "vbonnet/dear-agent", "deadbeef")

	required := map[string]bool{
		"gh":                  false,
		"pr":                  false,
		"merge":               false,
		"--squash":            false,
		"--delete-branch":     false,
		"--match-head-commit": false,
	}
	for _, a := range args {
		if _, ok := required[a]; ok {
			required[a] = true
		}
	}
	for flag, found := range required {
		if !found {
			t.Errorf("BuildMergeArgs missing required element %q", flag)
		}
	}
}

// --- helpers ---

func marshalJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("marshalJSON: %v", err))
	}
	return b
}
