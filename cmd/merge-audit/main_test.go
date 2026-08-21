package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const merged = "2026-06-10T12:00:00Z"

func TestRunRetainsPartialFindingsWithoutJSONStdout(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		name := "persists"
		if dryRun {
			name = "dry-run"
		}
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			var persisted []Violation
			args := []string{"--repos", "owner/good,owner/bad", "--json"}
			if dryRun {
				args = append(args, "--dry-run")
			}
			exitCode := runWithDependencies(args, runDependencies{
				auditRepo: func(_ context.Context, repo string, _ time.Time, _ string) ([]Violation, error) {
					if repo == "owner/bad" {
						return nil, fmt.Errorf("query failed")
					}
					return []Violation{{
						Repo:   repo,
						Type:   typeDirectPush,
						Ref:    "abc123",
						Detail: "successful repository finding",
					}}, nil
				},
				readFile: func(string) ([]byte, error) { return nil, fmt.Errorf("not found") },
				stdout:   &stdout,
				stderr:   &stderr,
				persistViolations: func(violations []Violation, _ options, _ io.Writer) {
					persisted = append(persisted, violations...)
				},
			})
			if exitCode != 1 {
				t.Fatalf("runWithDependencies exit = %d, want 1", exitCode)
			}
			if stdout.Len() != 0 {
				t.Fatalf("incomplete JSON audit wrote stdout: %q", stdout.String())
			}
			for _, want := range []string{"owner/bad: query failed", "partial findings retained", "successful repository finding", "audit incomplete"} {
				if !strings.Contains(stderr.String(), want) {
					t.Fatalf("stderr %q does not contain %q", stderr.String(), want)
				}
			}
			wantPersisted := 1
			if dryRun {
				wantPersisted = 0
			}
			if len(persisted) != wantPersisted {
				t.Fatalf("persisted %d partial findings, want %d", len(persisted), wantPersisted)
			}
			if len(persisted) == 1 && persisted[0].DetectedAt == "" {
				t.Fatal("persisted partial finding was not timestamped")
			}
		})
	}
}

func TestAuditRepoRetainsFindingsWhenLaterRulesetEvidenceFails(t *testing.T) {
	wantPR := Violation{Repo: "owner/repo", Type: typeUnresolvedThreads, Ref: "#1", Detail: "open thread"}
	wantPush := Violation{Repo: "owner/repo", Type: typeDirectPush, Ref: "abc123", Detail: "direct push"}

	got, err := auditRepoWithDependencies(context.Background(), "owner/repo", time.Now(), "ruleset.json", repoAuditDependencies{
		defaultBranch: func(context.Context, string) (string, error) { return "main", nil },
		auditMergedPRs: func(context.Context, string, string, time.Time) ([]Violation, error) {
			return []Violation{wantPR}, nil
		},
		auditDirectPushes: func(context.Context, string, string, time.Time) ([]Violation, error) {
			return []Violation{wantPush}, nil
		},
		auditRulesetDrift: func(context.Context, string, string) ([]Violation, error) {
			return nil, fmt.Errorf("ruleset API unavailable")
		},
	})
	if err == nil || !strings.Contains(err.Error(), "ruleset API unavailable") {
		t.Fatalf("auditRepoWithDependencies error = %v, want ruleset failure", err)
	}
	if len(got) != 2 || got[0] != wantPR || got[1] != wantPush {
		t.Fatalf("retained findings = %#v, want %#v then %#v", got, wantPR, wantPush)
	}

	violations, auditErrors := auditRepositories(context.Background(), []string{"owner/repo"}, options{days: 7, now: time.Now()}, func(context.Context, string, time.Time, string) ([]Violation, error) {
		return got, err
	})
	if len(violations) != 2 || len(auditErrors) != 1 {
		t.Fatalf("aggregate retained %d findings and %d errors, want 2 and 1", len(violations), len(auditErrors))
	}
}

func TestAuditMergedPRsRetainsFindingsBeforeQueryErrors(t *testing.T) {
	prOne := mergedPR{Number: 1, Title: "first", MergedAt: merged, HeadSHA: "head-one"}
	prTwo := mergedPR{Number: 2, Title: "second", MergedAt: merged, HeadSHA: "head-two"}
	tests := []struct {
		name                  string
		prs                   []mergedPR
		unresolvedThreadCount func(context.Context, string, int) (int, error)
		checkRuns             func(context.Context, string, string) ([]checkRun, error)
		wantType              string
		wantRef               string
		wantError             string
	}{
		{
			name: "earlier PR check finding survives a later PR thread error",
			prs:  []mergedPR{prOne, prTwo},
			unresolvedThreadCount: func(_ context.Context, _ string, pr int) (int, error) {
				if pr == prTwo.Number {
					return 0, fmt.Errorf("thread query failed")
				}
				return 0, nil
			},
			checkRuns: func(_ context.Context, _ string, _ string) ([]checkRun, error) {
				return []checkRun{{
					Name: "build", Status: "completed", Conclusion: "failure",
					StartedAt: "2026-06-10T10:00:00Z", CompletedAt: "2026-06-10T11:00:00Z",
				}}, nil
			},
			wantType:  typeChecksIncomplete,
			wantRef:   "#1",
			wantError: "PR #2 threads: thread query failed",
		},
		{
			name: "same PR thread finding survives its check error",
			prs:  []mergedPR{prOne},
			unresolvedThreadCount: func(context.Context, string, int) (int, error) {
				return 1, nil
			},
			checkRuns: func(context.Context, string, string) ([]checkRun, error) {
				return nil, fmt.Errorf("check query failed")
			},
			wantType:  typeUnresolvedThreads,
			wantRef:   "#1",
			wantError: "PR #1 checks: check query failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := mergedPRAuditDependencies{
				mergedPRs: func(context.Context, string, string, time.Time) ([]mergedPR, error) {
					return tt.prs, nil
				},
				unresolvedThreadCount: tt.unresolvedThreadCount,
				checkRuns:             tt.checkRuns,
			}
			got, err := auditMergedPRsWithDependencies(context.Background(), "owner/repo", "main", time.Time{}, deps)
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("auditMergedPRsWithDependencies error = %v, want %q", err, tt.wantError)
			}
			if len(got) != 1 || got[0].Type != tt.wantType || got[0].Ref != tt.wantRef {
				t.Fatalf("retained findings = %#v, want one %s finding on %s", got, tt.wantType, tt.wantRef)
			}

			var stdout, stderr bytes.Buffer
			var persisted []Violation
			exitCode := runWithDependencies([]string{"--repos", "owner/repo", "--json"}, runDependencies{
				auditRepo: func(context.Context, string, time.Time, string) ([]Violation, error) {
					return auditMergedPRsWithDependencies(context.Background(), "owner/repo", "main", time.Time{}, deps)
				},
				readFile: func(string) ([]byte, error) { return nil, fmt.Errorf("not found") },
				stdout:   &stdout,
				stderr:   &stderr,
				persistViolations: func(violations []Violation, _ options, _ io.Writer) {
					persisted = append(persisted, violations...)
				},
			})
			if exitCode != 1 {
				t.Fatalf("incomplete outer audit exit = %d, want 1", exitCode)
			}
			if stdout.Len() != 0 {
				t.Fatalf("incomplete outer audit wrote JSON stdout: %q", stdout.String())
			}
			if len(persisted) != 1 || persisted[0].Type != tt.wantType || persisted[0].DetectedAt == "" {
				t.Fatalf("persisted findings = %#v, want timestamped %s finding", persisted, tt.wantType)
			}
			if !strings.Contains(stderr.String(), "audit incomplete") || !strings.Contains(stderr.String(), tt.wantError) {
				t.Fatalf("incomplete outer audit stderr = %q", stderr.String())
			}
		})
	}
}

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
	local := completeRulesetJSON(`"~DEFAULT_BRANCH"`, `false`, `false`, `true`, `true`, `true`, `false`, `[{"context":"build","integration_id":15368},{"context":"vuln"}]`)

	t.Run("identical projection is in sync", func(t *testing.T) {
		live := completeRulesetJSON(`"~DEFAULT_BRANCH"`, `false`, `false`, `true`, `true`, `true`, `false`, `[{"context":"vuln"},{"context":"build","integration_id":15368}]`)
		live = []byte(strings.Replace(string(live), `{`, `{"id":18061003,"current_user_can_bypass":"never","_links":{},`, 1))
		if d := rulesetDrift(local, live); len(d) != 0 {
			t.Errorf("expected no drift, got %v", d)
		}
	})

	t.Run("conditions and every pull request and status check parameter drift", func(t *testing.T) {
		live := completeRulesetJSON(`"release/*"`, `true`, `true`, `false`, `false`, `false`, `true`, `[{"context":"build"},{"context":"vuln"}]`)
		d := rulesetDrift(local, live)
		if len(d) != 3 {
			t.Fatalf("expected conditions, pull-request, and status-check drifts, got %d: %v", len(d), d)
		}
	})

	t.Run("same context with absent integration identity drifts", func(t *testing.T) {
		live := []byte(strings.Replace(string(local), `{"context":"build","integration_id":15368}`, `{"context":"build"}`, 1))
		d := rulesetDrift(local, live)
		if len(d) != 1 || !strings.Contains(d[0], "rules.required_status_checks") {
			t.Fatalf("expected only required-status-check drift, got %d: %v", len(d), d)
		}
	})

	t.Run("same context with different integration identity drifts", func(t *testing.T) {
		live := []byte(strings.Replace(string(local), `"integration_id":15368`, `"integration_id":999`, 1))
		d := rulesetDrift(local, live)
		if len(d) != 1 || !strings.Contains(d[0], "rules.required_status_checks") {
			t.Fatalf("expected only required-status-check drift, got %d: %v", len(d), d)
		}
	})

	t.Run("merge methods approval count and reviewers drift", func(t *testing.T) {
		live := []byte(strings.NewReplacer(
			`"allowed_merge_methods":["squash"]`, `"allowed_merge_methods":["merge"]`,
			`"required_approving_review_count":0`, `"required_approving_review_count":1`,
			`"required_reviewers":[]`, `"required_reviewers":[{"file_patterns":["docs/**"],"minimum_approvals":1,"reviewer":{"id":7,"type":"Team"}}]`,
		).Replace(string(local)))
		if d := rulesetDrift(local, live); len(d) != 1 {
			t.Fatalf("expected pull-request drift, got %d: %v", len(d), d)
		}
	})

	t.Run("required reviewer and file pattern order is normalized", func(t *testing.T) {
		withReviewers := strings.Replace(string(local), `"required_reviewers":[]`, `"required_reviewers":[{"file_patterns":["docs/**","*.md"],"minimum_approvals":1,"reviewer":{"id":7,"type":"Team"}},{"file_patterns":["cmd/**"],"minimum_approvals":0,"reviewer":{"id":9,"type":"Team"}}]`, 1)
		live := strings.Replace(withReviewers, `"required_reviewers":[{"file_patterns":["docs/**","*.md"],"minimum_approvals":1,"reviewer":{"id":7,"type":"Team"}},{"file_patterns":["cmd/**"],"minimum_approvals":0,"reviewer":{"id":9,"type":"Team"}}]`, `"required_reviewers":[{"minimum_approvals":0,"file_patterns":["cmd/**"],"reviewer":{"type":"Team","id":9}},{"reviewer":{"id":7,"type":"Team"},"minimum_approvals":1,"file_patterns":["*.md","docs/**"]}]`, 1)
		if d := rulesetDrift([]byte(withReviewers), []byte(live)); len(d) != 0 {
			t.Fatalf("expected normalized required reviewers to match, got %v", d)
		}
	})

	t.Run("added bypass actor drifts", func(t *testing.T) {
		live := completeRulesetJSONWithBypass()
		d := rulesetDrift(local, live)
		if len(d) != 1 {
			t.Fatalf("expected 1 drift (bypass_actors), got %d: %v", len(d), d)
		}
	})

	t.Run("unparseable or unsupported policy yields drift sentinel", func(t *testing.T) {
		if d := rulesetDrift([]byte("{bad"), local); len(d) != 1 {
			t.Errorf("expected parse drift sentinel, got %v", d)
		}
		unsupported := []byte(`{"name":"main","target":"branch","enforcement":"active","bypass_actors":[],"conditions":{"ref_name":{"include":["main"],"exclude":[]}},"rules":[{"type":"creation"}]}`)
		if d := rulesetDrift(unsupported, local); len(d) != 1 {
			t.Errorf("expected unsupported-rule drift sentinel, got %v", d)
		}
	})
}

func completeRulesetJSON(include, codeOwners, lastPush, stale, threadResolution, strict, noEnforce, statusChecks string) []byte {
	return fmt.Appendf(nil, `{"name":"main-zero-bypass","target":"branch","enforcement":"active","bypass_actors":[],"conditions":{"ref_name":{"include":[%s],"exclude":[]}},"rules":[{"type":"deletion"},{"type":"non_fast_forward"},{"type":"required_linear_history"},{"type":"pull_request","parameters":{"allowed_merge_methods":["squash"],"required_approving_review_count":0,"dismiss_stale_reviews_on_push":%s,"require_code_owner_review":%s,"require_last_push_approval":%s,"required_review_thread_resolution":%s,"required_reviewers":[]}},{"type":"required_status_checks","parameters":{"strict_required_status_checks_policy":%s,"do_not_enforce_on_create":%s,"required_status_checks":%s}}]}`,
		include, stale, codeOwners, lastPush, threadResolution, strict, noEnforce, statusChecks)
}

func completeRulesetJSONWithBypass() []byte {
	return []byte(`{"name":"main-zero-bypass","target":"branch","enforcement":"active","bypass_actors":[{"actor_id":1,"actor_type":"RepositoryRole","bypass_mode":"always"}],"conditions":{"ref_name":{"include":["~DEFAULT_BRANCH"],"exclude":[]}},"rules":[{"type":"deletion"},{"type":"non_fast_forward"},{"type":"required_linear_history"},{"type":"pull_request","parameters":{"allowed_merge_methods":["squash"],"required_approving_review_count":0,"dismiss_stale_reviews_on_push":true,"require_code_owner_review":false,"require_last_push_approval":false,"required_review_thread_resolution":true,"required_reviewers":[]}},{"type":"required_status_checks","parameters":{"strict_required_status_checks_policy":true,"do_not_enforce_on_create":false,"required_status_checks":[{"context":"build","integration_id":15368},{"context":"vuln"}]}}]}`)
}

func TestParseRulesetFailsClosed(t *testing.T) {
	for _, raw := range [][]byte{
		[]byte(`{"enforcement":"active","bypass_actors":[],"rules":[]}`),
		[]byte(`{"name":"main-zero-bypass","target":"branch","enforcement":"active","bypass_actors":null,"conditions":{"ref_name":{"include":["~DEFAULT_BRANCH"],"exclude":[]}},"rules":[]}`),
		[]byte(`{"name":"main","enforcement":"active","bypass_actors":[],"rules":[{"type":"required_status_checks","parameters":{"required_status_checks":[]}}]}`),
		[]byte(`{"name":"main","enforcement":"active","bypass_actors":[],"rules":[{"type":"required_status_checks","parameters":{"strict_required_status_checks_policy":true,"required_status_checks":[{"context":""}]}}]}`),
		[]byte(`{"name":"main-zero-bypass","target":"branch","enforcement":"active","bypass_actors":[],"conditions":{"ref_name":{"include":["~DEFAULT_BRANCH"],"exclude":[]}},"rules":[{"type":"deletion"},{"type":"non_fast_forward"},{"type":"required_linear_history"},{"type":"pull_request","parameters":{"allowed_merge_methods":["squash"],"required_approving_review_count":0,"dismiss_stale_reviews_on_push":true,"require_code_owner_review":false,"require_last_push_approval":false,"required_review_thread_resolution":true,"required_reviewers":[{"file_patterns":["docs/**"],"reviewer":{"id":7,"type":"Team"}}]}},{"type":"required_status_checks","parameters":{"strict_required_status_checks_policy":true,"do_not_enforce_on_create":false,"required_status_checks":[{"context":"build","integration_id":15368}]}}]}`),
	} {
		if _, err := parseRuleset(raw); err == nil {
			t.Errorf("parseRuleset(%s) unexpectedly succeeded", raw)
		}
	}
}

func TestParseRulesetRejectsUnknownPolicyFields(t *testing.T) {
	base := string(completeRulesetJSON(`"~DEFAULT_BRANCH"`, `false`, `false`, `true`, `true`, `true`, `false`, `[{"context":"build","integration_id":15368}]`))
	withReviewer := strings.Replace(base, `"required_reviewers":[]`, `"required_reviewers":[{"file_patterns":["docs/**"],"minimum_approvals":1,"reviewer":{"id":7,"type":"Team"}}]`, 1)
	tests := map[string]string{
		"top-level policy field":  strings.Replace(base, `"target":"branch"`, `"target":"branch","repository_name":"dear-agent"`, 1),
		"rule field":              strings.Replace(base, `{"type":"deletion"}`, `{"type":"deletion","unexpected":true}`, 1),
		"null simple parameters":  strings.Replace(base, `{"type":"deletion"}`, `{"type":"deletion","parameters":null}`, 1),
		"empty simple parameters": strings.Replace(base, `{"type":"deletion"}`, `{"type":"deletion","parameters":{}}`, 1),
		"ref_name field":          strings.Replace(base, `"exclude":[]`, `"exclude":[],"repository_name":[]`, 1),
		"dismissal restriction":   strings.Replace(base, `"allowed_merge_methods"`, `"dismissal_restriction":{},"allowed_merge_methods"`, 1),
		"reviewer entry field":    strings.Replace(withReviewer, `"minimum_approvals":1`, `"minimum_approvals":1,"teams":[]`, 1),
		"reviewer identity field": strings.Replace(withReviewer,
			`"reviewer":{"id":7,"type":"Team"}`, `"reviewer":{"id":7,"type":"Team","slug":"docs"}`, 1),
		"status parameter field": strings.Replace(base, `"do_not_enforce_on_create":false`, `"do_not_enforce_on_create":false,"unexpected":true`, 1),
		"required check field":   strings.Replace(base, `"integration_id":15368`, `"integration_id":15368,"app_slug":"github-actions"`, 1),
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := parseRuleset([]byte(raw)); err == nil {
				t.Fatal("parseRuleset unexpectedly accepted an unsupported policy field")
			}
			if name == "dismissal restriction" {
				drift := rulesetDrift([]byte(base), []byte(raw))
				if len(drift) != 1 || !strings.Contains(drift[0], "dismissal_restriction") {
					t.Fatalf("live unknown-field drift = %v, want dismissal_restriction evidence", drift)
				}
			}
		})
	}
}

func TestValidateCanonicalRulesetAcceptsRequiredReviewers(t *testing.T) {
	raw := completeRulesetJSON(`"~DEFAULT_BRANCH"`, `false`, `false`, `true`, `true`, `true`, `false`, `[{"context":"build","integration_id":15368}]`)
	raw = []byte(strings.Replace(string(raw), `"required_reviewers":[]`, `"required_reviewers":[{"file_patterns":["docs/**"],"minimum_approvals":1,"reviewer":{"id":7,"type":"Team"}}]`, 1))
	ruleset, err := parseRuleset(raw)
	if err != nil {
		t.Fatalf("parse ruleset with provider-supported required reviewer: %v", err)
	}
	if err := validateCanonicalRuleset(ruleset); err != nil {
		t.Fatalf("validate ruleset with provider-supported required reviewer: %v", err)
	}
}

func TestValidateCanonicalRulesetRejectsWeakenedInvariants(t *testing.T) {
	base := string(completeRulesetJSON(`"~DEFAULT_BRANCH"`, `false`, `false`, `true`, `true`, `true`, `false`, `[{"context":"build","integration_id":15368}]`))
	tests := map[string]string{
		"merge method":           strings.Replace(base, `"allowed_merge_methods":["squash"]`, `"allowed_merge_methods":["merge"]`, 1),
		"rebase method":          strings.Replace(base, `"allowed_merge_methods":["squash"]`, `"allowed_merge_methods":["squash","rebase"]`, 1),
		"non-strict checks":      strings.Replace(base, `"strict_required_status_checks_policy":true`, `"strict_required_status_checks_policy":false`, 1),
		"missing app identity":   strings.Replace(base, `,"integration_id":15368`, ``, 1),
		"wrong app identity":     strings.Replace(base, `"integration_id":15368`, `"integration_id":42`, 1),
		"missing required check": strings.Replace(base, `[{"context":"build","integration_id":15368}]`, `[]`, 1),
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			ruleset, err := parseRuleset([]byte(raw))
			if err != nil {
				t.Fatalf("parse weakened ruleset: %v", err)
			}
			if err := validateCanonicalRuleset(ruleset); err == nil {
				t.Fatal("validateCanonicalRuleset unexpectedly accepted weakened policy")
			}
		})
	}
}

func TestParseRulesetSummaryPages(t *testing.T) {
	rulesets, err := parseRulesetSummaryPages([]byte(`[[{"id":7,"name":"first"}],[{"id":8,"name":"second"}]]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(rulesets) != 2 || rulesets[0].ID != 7 || rulesets[1].ID != 8 {
		t.Fatalf("paginated rulesets = %#v, want both pages flattened", rulesets)
	}
	if _, err := parseRulesetSummaryPages([]byte(`[{"id":7,"name":"not-slurped"}]`)); err == nil {
		t.Fatal("flat non-slurped response unexpectedly accepted")
	}
}

func TestSelectExpectedRuleset(t *testing.T) {
	tests := []struct {
		name      string
		repo      string
		expected  string
		rulesets  []rulesetSummary
		wantID    int64
		wantError bool
	}{
		{"dear-agent legacy name during transition", "vbonnet/dear-agent", "main-zero-bypass", []rulesetSummary{{ID: dearAgentRulesetID, Name: legacyRulesetName}}, dearAgentRulesetID, false},
		{"dear-agent current name after transition", "vbonnet/dear-agent", "main-zero-bypass", []rulesetSummary{{ID: dearAgentRulesetID, Name: "main-zero-bypass"}}, dearAgentRulesetID, false},
		{"dear-agent mixed-case slug retains exact ID gate", "VBonnet/Dear-Agent", "main-zero-bypass", []rulesetSummary{{ID: 99, Name: "main-zero-bypass"}}, 0, true},
		{"dear-agent replacement id", "vbonnet/dear-agent", "main-zero-bypass", []rulesetSummary{{ID: 99, Name: "main-zero-bypass"}}, 0, true},
		{"dear-agent unexpected name", "vbonnet/dear-agent", "main-zero-bypass", []rulesetSummary{{ID: dearAgentRulesetID, Name: "other"}}, 0, true},
		{"dear-agent duplicate transition candidates", "vbonnet/dear-agent", "main-zero-bypass", []rulesetSummary{{ID: dearAgentRulesetID, Name: legacyRulesetName}, {ID: 99, Name: "main-zero-bypass"}}, 0, true},
		{"dear-agent absent", "vbonnet/dear-agent", "main-zero-bypass", nil, 0, true},
		{"generic exact name", "vbonnet/other", "branch-protection", []rulesetSummary{{ID: 7, Name: "branch-protection"}}, 7, false},
		{"generic duplicate name", "vbonnet/other", "branch-protection", []rulesetSummary{{ID: 7, Name: "branch-protection"}, {ID: 8, Name: "branch-protection"}}, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := selectExpectedRuleset(tt.repo, tt.expected, tt.rulesets)
			if tt.wantError {
				if err == nil {
					t.Fatalf("selectExpectedRuleset unexpectedly selected %#v", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.ID != tt.wantID {
				t.Fatalf("selected ID = %d, want %d", got.ID, tt.wantID)
			}
		})
	}
}

func TestCanonicalRulesetPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if path, declared := canonicalRulesetPath("vbonnet/other", ""); declared || path != "" {
		t.Fatalf("undeclared repo source = (%q, %t), want empty/false", path, declared)
	}
	if path, declared := canonicalRulesetPath("vbonnet/other", "/tmp/ruleset.json"); !declared || path != "/tmp/ruleset.json" {
		t.Fatalf("explicit source = (%q, %t), want override/true", path, declared)
	}
	want := filepath.Join(home, "src", "dear-agent", ".github", "rulesets", "main.json")
	if path, declared := canonicalRulesetPath("vbonnet/dear-agent", ""); !declared || path != want {
		t.Fatalf("dear-agent source = (%q, %t), want %q/true", path, declared, want)
	}
	if path, declared := canonicalRulesetPath("VBonnet/Dear-Agent", ""); !declared || path != want {
		t.Fatalf("mixed-case dear-agent source = (%q, %t), want %q/true", path, declared, want)
	}
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
