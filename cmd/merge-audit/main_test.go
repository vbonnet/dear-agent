package main

import (
	"testing"
	"time"
)

const merged = "2026-06-10T12:00:00Z"

func TestRedOrPendingChecks(t *testing.T) {
	tests := []struct {
		name        string
		runs        []checkRun
		wantRed     int
		wantPending int
	}{
		{
			name: "all green before merge",
			runs: []checkRun{
				{Name: "build", Status: "completed", Conclusion: "success", StartedAt: "2026-06-10T11:00:00Z", CompletedAt: "2026-06-10T11:30:00Z"},
				{Name: "lint", Status: "completed", Conclusion: "skipped", StartedAt: "2026-06-10T11:00:00Z", CompletedAt: "2026-06-10T11:10:00Z"},
			},
		},
		{
			name: "failure before merge counts as red",
			runs: []checkRun{
				{Name: "build", Status: "completed", Conclusion: "failure", StartedAt: "2026-06-10T11:00:00Z", CompletedAt: "2026-06-10T11:30:00Z"},
			},
			wantRed: 1,
		},
		{
			name: "started but not completed at merge is pending",
			runs: []checkRun{
				{Name: "vuln", Status: "in_progress", Conclusion: "", StartedAt: "2026-06-10T11:00:00Z", CompletedAt: ""},
			},
			wantPending: 1,
		},
		{
			name: "completed after merge is pending",
			runs: []checkRun{
				{Name: "slow", Status: "completed", Conclusion: "success", StartedAt: "2026-06-10T11:00:00Z", CompletedAt: "2026-06-10T13:00:00Z"},
			},
			wantPending: 1,
		},
		{
			name: "neutral and cancelled — neutral ok, cancelled red",
			runs: []checkRun{
				{Name: "neutralcheck", Status: "completed", Conclusion: "neutral", StartedAt: "2026-06-10T11:00:00Z", CompletedAt: "2026-06-10T11:10:00Z"},
				{Name: "cancelledcheck", Status: "completed", Conclusion: "cancelled", StartedAt: "2026-06-10T11:00:00Z", CompletedAt: "2026-06-10T11:10:00Z"},
			},
			wantRed: 1,
		},
		{
			name: "check started after merge is ignored",
			runs: []checkRun{
				{Name: "post", Status: "in_progress", Conclusion: "", StartedAt: "2026-06-10T13:00:00Z", CompletedAt: ""},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			red, pending := redOrPendingChecks(tt.runs, merged)
			if len(red) != tt.wantRed {
				t.Errorf("red = %v (%d), want %d", red, len(red), tt.wantRed)
			}
			if len(pending) != tt.wantPending {
				t.Errorf("pending = %v (%d), want %d", pending, len(pending), tt.wantPending)
			}
		})
	}
}

func TestIsDirectPush(t *testing.T) {
	tests := []struct {
		name    string
		message string
		parents int
		want    bool
	}{
		{"squash merge has PR ref", "feat: add thing (#123)", 1, false},
		{"squash merge multiline body", "feat: add thing (#123)\n\nlong body here", 1, false},
		{"true merge commit two parents", "Merge pull request #5 from x/y", 2, false},
		{"merge prefix one parent", "Merge branch 'main'", 1, false},
		{"direct push no ref", "quick fix typo", 1, true},
		{"direct push hash but not pr ref", "fix issue #42 manually", 1, true},
		{"root commit zero parents", "initial commit", 0, false},
		{"empty message single parent", "", 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDirectPush(tt.message, tt.parents); got != tt.want {
				t.Errorf("isDirectPush(%q, %d) = %v, want %v", tt.message, tt.parents, got, tt.want)
			}
		})
	}
}

func TestContainsPRRef(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"feat: x (#1)", true},
		{"feat: x (#12345)", true},
		{"chore (#7) and more", true},
		{"no ref here", false},
		{"loose #42 not parenthesized", false},
		{"(#) empty", false},
		{"(#12 unterminated", false},
		{"trailing (#9)", true},
	}
	for _, tt := range tests {
		if got := containsPRRef(tt.in); got != tt.want {
			t.Errorf("containsPRRef(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestRulesetDrift(t *testing.T) {
	local := []byte(`{
      "name":"main-zero-bypass","enforcement":"active","bypass_actors":[],
      "rules":[
        {"type":"deletion"},
        {"type":"pull_request"},
        {"type":"required_status_checks","parameters":{"required_status_checks":[
          {"context":"build"},{"context":"vuln"}]}}
      ]}`)

	t.Run("identical projection is in sync", func(t *testing.T) {
		live := []byte(`{
          "id":99,"node_id":"abc","created_at":"2026-01-01",
          "name":"main-zero-bypass","enforcement":"active","bypass_actors":[],
          "rules":[
            {"type":"pull_request"},
            {"type":"deletion"},
            {"type":"required_status_checks","parameters":{"required_status_checks":[
              {"context":"vuln"},{"context":"build"}]}}
          ]}`)
		if d := rulesetDrift(local, live); len(d) != 0 {
			t.Errorf("expected no drift, got %v", d)
		}
	})

	t.Run("enforcement disabled drifts", func(t *testing.T) {
		live := []byte(`{"enforcement":"disabled","bypass_actors":[],"rules":[
          {"type":"deletion"},{"type":"pull_request"},
          {"type":"required_status_checks","parameters":{"required_status_checks":[
            {"context":"build"},{"context":"vuln"}]}}]}`)
		d := rulesetDrift(local, live)
		if len(d) != 1 {
			t.Fatalf("expected 1 drift, got %d: %v", len(d), d)
		}
	})

	t.Run("added bypass actor drifts", func(t *testing.T) {
		live := []byte(`{"enforcement":"active","bypass_actors":[{"actor_id":1}],"rules":[
          {"type":"deletion"},{"type":"pull_request"},
          {"type":"required_status_checks","parameters":{"required_status_checks":[
            {"context":"build"},{"context":"vuln"}]}}]}`)
		d := rulesetDrift(local, live)
		if len(d) != 1 {
			t.Fatalf("expected 1 drift (bypass_actors), got %d: %v", len(d), d)
		}
	})

	t.Run("missing rule type and dropped check drift", func(t *testing.T) {
		live := []byte(`{"enforcement":"active","bypass_actors":[],"rules":[
          {"type":"deletion"},
          {"type":"required_status_checks","parameters":{"required_status_checks":[
            {"context":"build"}]}}]}`)
		d := rulesetDrift(local, live)
		if len(d) != 2 {
			t.Fatalf("expected 2 drifts (rule types + checks), got %d: %v", len(d), d)
		}
	})

	t.Run("unparseable input yields drift sentinel", func(t *testing.T) {
		if d := rulesetDrift([]byte("{bad"), local); len(d) != 1 {
			t.Errorf("expected parse drift sentinel, got %v", d)
		}
	})
}

func TestBreakGlassViolations(t *testing.T) {
	since := time.Date(2026, 6, 13, 0, 0, 0, 0, time.UTC)
	data := []byte(`{"timestamp":"2026-06-15T10:00:00Z","tool":"safe-merge","gate":"ci","reason":"hotfix","allowed":true}
{"timestamp":"2026-06-15T11:00:00Z","tool":"safe-push","gate":"soak","reason":"denied attempt","allowed":false}
{"timestamp":"2026-06-01T09:00:00Z","tool":"safe-merge","gate":"ci","reason":"old","allowed":true}
garbage line
{"timestamp":"2026-06-16T08:00:00Z","tool":"safe-rebase","gate":"threads","reason":"x","allowed":true}
`)
	got := breakGlassViolations(data, since)
	if len(got) != 2 {
		t.Fatalf("expected 2 break-glass violations, got %d: %v", len(got), got)
	}
	for _, v := range got {
		if v.Type != typeBreakGlass {
			t.Errorf("type = %q, want %q", v.Type, typeBreakGlass)
		}
	}
}

func TestBeadTitle(t *testing.T) {
	tests := []struct {
		v    Violation
		want string
	}{
		{Violation{Repo: "vbonnet/dear-agent", Type: typeUnresolvedThreads, Ref: "#42"}, "merge-audit: unresolved-threads on vbonnet/dear-agent #42"},
		{Violation{Repo: "vbonnet/dear-agent", Type: typeRulesetDrift}, "merge-audit: ruleset-drift on vbonnet/dear-agent"},
	}
	for _, tt := range tests {
		if got := beadTitle(tt.v); got != tt.want {
			t.Errorf("beadTitle = %q, want %q", got, tt.want)
		}
	}
}

func TestParseRemoteSlug(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"https://github.com/vbonnet/dear-agent.git", "vbonnet/dear-agent"},
		{"https://github.com/vbonnet/dear-agent", "vbonnet/dear-agent"},
		{"git@github.com:vbonnet/dear-agent.git", "vbonnet/dear-agent"},
		{"ssh://git@github.com/owner/repo", "owner/repo"},
		{"https://gitlab.com/owner/repo.git", ""},
		{"https://github.com/owner/repo/extra", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := parseRemoteSlug(tt.in); got != tt.want {
			t.Errorf("parseRemoteSlug(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestSplitRepo(t *testing.T) {
	tests := []struct {
		in   string
		o, n string
		ok   bool
	}{
		{"vbonnet/dear-agent", "vbonnet", "dear-agent", true},
		{"noslash", "", "", false},
		{"/leading", "", "", false},
		{"trailing/", "", "", false},
	}
	for _, tt := range tests {
		o, n, ok := splitRepo(tt.in)
		if o != tt.o || n != tt.n || ok != tt.ok {
			t.Errorf("splitRepo(%q) = (%q,%q,%v), want (%q,%q,%v)", tt.in, o, n, ok, tt.o, tt.n, tt.ok)
		}
	}
}

func TestSmallHelpers(t *testing.T) {
	if got := firstLine("a\nb\nc"); got != "a" {
		t.Errorf("firstLine = %q, want a", got)
	}
	if got := firstLine("single"); got != "single" {
		t.Errorf("firstLine = %q, want single", got)
	}
	if got := shortSHA("0123456789abcdef0123"); got != "0123456789ab" {
		t.Errorf("shortSHA = %q", got)
	}
	if got := shortSHA("short"); got != "short" {
		t.Errorf("shortSHA short = %q", got)
	}
	if got := repoName("vbonnet/dear-agent"); got != "dear-agent" {
		t.Errorf("repoName = %q", got)
	}
	if got := repoName("bare"); got != "bare" {
		t.Errorf("repoName bare = %q", got)
	}
	if got := splitCSV(" a , b ,, c "); len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Errorf("splitCSV = %v", got)
	}
}

func TestChecksDetail(t *testing.T) {
	got := checksDetail(7, "fix", []string{"build [failure]"}, []string{"vuln"})
	want := "PR #7 (fix) merged on red: failed: build [failure]; in-progress at merge: vuln"
	if got != want {
		t.Errorf("checksDetail = %q, want %q", got, want)
	}
	if got := checksDetail(7, "fix", []string{"build [failure]"}, nil); got != "PR #7 (fix) merged on red: failed: build [failure]" {
		t.Errorf("checksDetail red-only = %q", got)
	}
}
