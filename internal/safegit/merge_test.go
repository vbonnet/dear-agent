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

type reviewThreadNode struct {
	IsResolved bool `json:"isResolved"`
	IsOutdated bool `json:"isOutdated"`
	Comments   struct {
		Nodes []struct {
			Author struct {
				Login string `json:"login"`
			} `json:"author"`
			Body string `json:"body"`
		} `json:"nodes"`
	} `json:"comments"`
}

type reviewThreadsDoc struct {
	Data struct {
		Repository struct {
			PullRequest struct {
				ReviewThreads struct {
					Nodes []reviewThreadNode `json:"nodes"`
				} `json:"reviewThreads"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
}

func makeReviewJSON(threads []struct {
	resolved bool
	outdated bool
	author   string
	body     string
}) []byte {
	var doc reviewThreadsDoc
	for _, t := range threads {
		node := reviewThreadNode{
			IsResolved: t.resolved,
			IsOutdated: t.outdated,
		}
		if t.body != "" || t.author != "" {
			var c struct {
				Author struct {
					Login string `json:"login"`
				} `json:"author"`
				Body string `json:"body"`
			}
			c.Author.Login = t.author
			c.Body = t.body
			node.Comments.Nodes = append(node.Comments.Nodes, c)
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
		author   string
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
		author   string
		body     string
	}{
		{resolved: false, outdated: false, author: "gemini-code-assist", body: "Please fix the nil check"},
	})
	err := parseReviewThreads(data)
	if err == nil {
		t.Fatal("expected error for unresolved thread, got nil")
	}
	if !strings.Contains(err.Error(), "unresolved") {
		t.Errorf("error should mention 'unresolved', got: %v", err)
	}
}

func TestParseReviewThreads_ShowsAuthorAndBody(t *testing.T) {
	data := makeReviewJSON([]struct {
		resolved bool
		outdated bool
		author   string
		body     string
	}{
		{resolved: false, author: "gemini-code-assist", body: "Consider extracting to a helper"},
		{resolved: false, author: "vbonnet", body: "Will address in follow-up"},
	})
	err := parseReviewThreads(data)
	if err == nil {
		t.Fatal("expected error for unresolved threads, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "@gemini-code-assist") {
		t.Errorf("error should show author @gemini-code-assist, got: %v", msg)
	}
	if !strings.Contains(msg, "@vbonnet") {
		t.Errorf("error should show author @vbonnet, got: %v", msg)
	}
	if !strings.Contains(msg, "Consider extracting") {
		t.Errorf("error should show comment body, got: %v", msg)
	}
}

func TestParseReviewThreads_UnknownAuthorFallback(t *testing.T) {
	data := makeReviewJSON([]struct {
		resolved bool
		outdated bool
		author   string
		body     string
	}{
		{resolved: false, author: "", body: "No author login"},
	})
	err := parseReviewThreads(data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "@unknown") {
		t.Errorf("error should show @unknown when author is absent, got: %v", err.Error())
	}
}

func TestParseReviewThreads_BodyTruncatedAt80Runes(t *testing.T) {
	longBody := strings.Repeat("x", 100)
	data := makeReviewJSON([]struct {
		resolved bool
		outdated bool
		author   string
		body     string
	}{
		{resolved: false, author: "bot", body: longBody},
	})
	err := parseReviewThreads(data)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "…") {
		t.Errorf("long body should be truncated with ellipsis, got: %v", err.Error())
	}
}

func TestParseReviewThreads_Empty(t *testing.T) {
	data := makeReviewJSON(nil)
	if err := parseReviewThreads(data); err != nil {
		t.Fatalf("no threads should pass, got: %v", err)
	}
}

// --- parseSoak ---

func makeSoakJSON(committedAt time.Time) []byte {
	type commitEntry struct {
		CommittedDate time.Time `json:"committedDate"`
	}
	type doc struct {
		HeadRefOid string        `json:"headRefOid"`
		Commits    []commitEntry `json:"commits"`
	}
	d := doc{HeadRefOid: "abc123"}
	if !committedAt.IsZero() {
		d.Commits = []commitEntry{{CommittedDate: committedAt}}
	}
	b, _ := json.Marshal(d)
	return b
}

func TestParseSoak_OldCommit(t *testing.T) {
	now := time.Now()
	past := now.Add(-10 * time.Minute)
	data := makeSoakJSON(past)
	if err := parseSoak(data, now); err != nil {
		t.Fatalf("old commit should pass soak gate, got: %v", err)
	}
}

func TestParseSoak_TooRecent(t *testing.T) {
	now := time.Now()
	recent := now.Add(-1 * time.Minute)
	data := makeSoakJSON(recent)
	err := parseSoak(data, now)
	if err == nil {
		t.Fatal("expected error for too-recent commit, got nil")
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

func TestBuildMergeArgs_PanicsOnEmptySHA(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("BuildMergeArgs must panic on empty headSHA — an empty anchor " +
				"would silently defeat the TOCTOU protection")
		}
	}()
	BuildMergeArgs(1, "o/r", "")
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
