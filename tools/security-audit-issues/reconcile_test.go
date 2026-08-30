package main

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

type providerCall struct {
	stdin string
	args  []string
	out   string
	err   error
}

type scriptedProvider struct {
	t     *testing.T
	calls []providerCall
	next  int
}

func (p *scriptedProvider) run(ctx context.Context, stdin string, args ...string) ([]byte, error) {
	p.t.Helper()
	if _, ok := ctx.Deadline(); !ok {
		p.t.Fatal("provider call has no deadline")
	}
	if p.next >= len(p.calls) {
		p.t.Fatalf("unexpected provider call: stdin=%q args=%q", stdin, args)
	}
	want := p.calls[p.next]
	p.next++
	if stdin != want.stdin {
		p.t.Errorf("provider call %d stdin = %q, want %q", p.next, stdin, want.stdin)
	}
	if !reflect.DeepEqual(args, want.args) {
		p.t.Errorf("provider call %d args = %q, want %q", p.next, args, want.args)
	}
	return []byte(want.out), want.err
}

func (p *scriptedProvider) done() {
	p.t.Helper()
	if p.next != len(p.calls) {
		p.t.Fatalf("provider calls consumed = %d, want %d", p.next, len(p.calls))
	}
}

func TestReconcileCreatesOneRollingIssue(t *testing.T) {
	observedAt := time.Date(2026, 8, 30, 12, 34, 56, 0, time.FixedZone("offset", -7*60*60))
	snapshot := findings{compromised: "\n- vendor/action@v1\n", permissions: "\n- workflow.yml: write-all\n"}
	body := renderIssueBody(snapshot, observedAt)
	provider := &scriptedProvider{t: t, calls: []providerCall{
		{args: labelArgs()},
		{args: listArgs(), out: `[[]]`},
		{stdin: body, args: createArgs()},
	}}

	result, err := reconcile(context.Background(), provider, "owner/repository", snapshot, observedAt)
	if err != nil {
		t.Fatal(err)
	}
	provider.done()
	if got := result.summary(); got != "security-audit issue created" {
		t.Fatalf("summary = %q", got)
	}
	for _, want := range []string{
		managedMarker,
		findingsDigestMarker(snapshot),
		"2026-08-30T19:34:56Z",
		"- vendor/action@v1",
		"- workflow.yml: write-all",
		"### Third-party actions not pinned by SHA\n```\n(none)\n```",
		"### pull_request_target usage\n```\n(none)\n```",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("issue body omits %q:\n%s", want, body)
		}
	}
}

func TestReconcileUpdatesCanonicalAndClosesDuplicate(t *testing.T) {
	observedAt := time.Date(2026, 8, 30, 1, 2, 3, 0, time.UTC)
	snapshot := findings{unpinned: "workflow.yml:7: uses: vendor/action@v1"}
	body := renderIssueBody(snapshot, observedAt)
	provider := &scriptedProvider{t: t, calls: []providerCall{
		{args: labelArgs()},
		{args: listArgs(), out: `[[
			{"number":44,"title":"security-audit: workflow findings","body":"old duplicate"},
			{"number":5,"title":"human security investigation"},
			{"number":6,"title":"security-audit: workflow findings","pull_request":{}},
			{"number":11,"title":"security-audit: workflow findings","body":"old canonical"}
		],[
			{"number":44,"title":"security-audit: workflow findings","body":"old duplicate"}
		]]`},
		{stdin: body, args: []string{"issue", "edit", "11", "--repo", "owner/repository", "--body-file", "-"}},
		{args: []string{"issue", "close", "44", "--repo", "owner/repository", "--comment", "Duplicate of the command-owned rolling issue #11; auto-closing."}},
	}}

	result, err := reconcile(context.Background(), provider, "owner/repository", snapshot, observedAt)
	if err != nil {
		t.Fatal(err)
	}
	provider.done()
	if got := result.summary(); got != "security-audit issue #11 updated; duplicate issues closed: [44]; unrelated labelled items ignored: 2" {
		t.Fatalf("summary = %q", got)
	}
}

func TestReconcileIdenticalCanonicalIsTrueNoOp(t *testing.T) {
	firstObservedAt := time.Date(2026, 8, 29, 1, 2, 3, 0, time.UTC)
	laterObservedAt := firstObservedAt.Add(24 * time.Hour)
	snapshot := findings{pullRequestTarget: "workflow.yml:8: pull_request_target:"}
	body := renderIssueBody(snapshot, firstObservedAt)
	provider := &scriptedProvider{t: t, calls: []providerCall{
		{args: labelArgs()},
		{args: listArgs(), out: `[[{"number":17,"title":"security-audit: workflow findings","body":` + strconv.Quote(body) + `}]]`},
	}}

	result, err := reconcile(context.Background(), provider, "owner/repository", snapshot, laterObservedAt)
	if err != nil {
		t.Fatal(err)
	}
	provider.done()
	if got := result.summary(); got != "security-audit issue #17 already current" {
		t.Fatalf("summary = %q", got)
	}
}

func TestReconcileProviderCRLFBodyIsTrueNoOp(t *testing.T) {
	firstObservedAt := time.Date(2026, 8, 29, 1, 2, 3, 0, time.UTC)
	laterObservedAt := firstObservedAt.Add(24 * time.Hour)
	snapshot := findings{pullRequestTarget: "workflow.yml:8: pull_request_target:"}
	body := strings.ReplaceAll(renderIssueBody(snapshot, firstObservedAt), "\n", "\r\n")
	provider := &scriptedProvider{t: t, calls: []providerCall{
		{args: labelArgs()},
		{args: listArgs(), out: `[[{"number":17,"title":"security-audit: workflow findings","body":` + strconv.Quote(body) + `}]]`},
	}}

	result, err := reconcile(context.Background(), provider, "owner/repository", snapshot, laterObservedAt)
	if err != nil {
		t.Fatal(err)
	}
	provider.done()
	if got := result.summary(); got != "security-audit issue #17 already current" {
		t.Fatalf("summary = %q", got)
	}
}

func TestReconcileRepairsTamperedManagedBody(t *testing.T) {
	firstObservedAt := time.Date(2026, 8, 29, 1, 2, 3, 0, time.UTC)
	laterObservedAt := firstObservedAt.Add(24 * time.Hour)
	snapshot := findings{compromised: "- vendor/action@v1"}
	tamperedBody := strings.Replace(
		renderIssueBody(snapshot, firstObservedAt),
		"- vendor/action@v1",
		"- substituted/content@v9",
		1,
	)
	provider := &scriptedProvider{t: t, calls: []providerCall{
		{args: labelArgs()},
		{args: listArgs(), out: `[[{"number":17,"title":"security-audit: workflow findings","body":` + strconv.Quote(tamperedBody) + `}]]`},
		{stdin: renderIssueBody(snapshot, laterObservedAt), args: []string{"issue", "edit", "17", "--repo", "owner/repository", "--body-file", "-"}},
	}}

	result, err := reconcile(context.Background(), provider, "owner/repository", snapshot, laterObservedAt)
	if err != nil {
		t.Fatal(err)
	}
	provider.done()
	if got := result.summary(); got != "security-audit issue #17 updated" {
		t.Fatalf("summary = %q", got)
	}
}

func TestReconcileCleanClosesOnlyCommandOwnedIssues(t *testing.T) {
	provider := &scriptedProvider{t: t, calls: []providerCall{
		{args: labelArgs()},
		{args: listArgs(), out: `[[
			{"number":9,"title":"security-audit: workflow findings"},
			{"number":4,"title":"human security investigation"},
			{"number":3,"title":"security-audit: workflow findings"}
		]]`},
		{args: []string{"issue", "close", "3", "--repo", "owner/repository", "--comment", cleanComment}},
		{args: []string{"issue", "close", "9", "--repo", "owner/repository", "--comment", cleanComment}},
	}}

	result, err := reconcile(context.Background(), provider, "owner/repository", findings{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	provider.done()
	if got := result.summary(); got != "security-audit clean; command-owned issues closed: [3 9]; unrelated labelled items ignored: 1" {
		t.Fatalf("summary = %q", got)
	}
}

func TestReconcileCleanWithNoOwnedIssueIsNoOp(t *testing.T) {
	provider := &scriptedProvider{t: t, calls: []providerCall{
		{args: labelArgs()},
		{args: listArgs(), out: `[[{"number":4,"title":"human security investigation"}]]`},
	}}

	result, err := reconcile(context.Background(), provider, "owner/repository", findings{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	provider.done()
	if got := result.summary(); got != "security-audit clean; no command-owned issue is open; unrelated labelled items ignored: 1" {
		t.Fatalf("summary = %q", got)
	}
}

func TestReconcileFailsClosedOnProviderAndPayloadErrors(t *testing.T) {
	providerFailure := errors.New("provider unavailable")
	tests := []struct {
		name     string
		findings findings
		calls    []providerCall
		want     string
	}{
		{
			name:  "label provisioning on clean run",
			calls: []providerCall{{args: labelArgs(), err: providerFailure}},
			want:  "reconcile security-audit label",
		},
		{
			name:  "malformed issue inventory",
			calls: []providerCall{{args: labelArgs()}, {args: listArgs(), out: `{`}},
			want:  "parse open security-audit issues",
		},
		{
			name:  "null issue inventory",
			calls: []providerCall{{args: labelArgs()}, {args: listArgs(), out: `null`}},
			want:  "expected at least one response page",
		},
		{
			name:  "zero-page issue inventory",
			calls: []providerCall{{args: labelArgs()}, {args: listArgs(), out: `[]`}},
			want:  "expected at least one response page",
		},
		{
			name:  "null issue inventory page",
			calls: []providerCall{{args: labelArgs()}, {args: listArgs(), out: `[null]`}},
			want:  "page 1 is not an array",
		},
		{
			name:  "issue inventory provider failure",
			calls: []providerCall{{args: labelArgs()}, {args: listArgs(), err: providerFailure}},
			want:  "list open security-audit issues",
		},
		{
			name:  "invalid owned issue number",
			calls: []providerCall{{args: labelArgs()}, {args: listArgs(), out: `[[{"number":0,"title":"security-audit: workflow findings"}]]`}},
			want:  "invalid issue number",
		},
		{
			name:     "issue creation",
			findings: findings{compromised: "finding"},
			calls: []providerCall{
				{args: labelArgs()},
				{args: listArgs(), out: `[[]]`},
				{stdin: renderIssueBody(findings{compromised: "finding"}, time.Time{}), args: createArgs(), err: providerFailure},
			},
			want: "create security-audit issue",
		},
		{
			name:     "issue update",
			findings: findings{compromised: "finding"},
			calls: []providerCall{
				{args: labelArgs()},
				{args: listArgs(), out: `[[{"number":8,"title":"security-audit: workflow findings","body":"old"}]]`},
				{stdin: renderIssueBody(findings{compromised: "finding"}, time.Time{}), args: []string{"issue", "edit", "8", "--repo", "owner/repository", "--body-file", "-"}, err: providerFailure},
			},
			want: "update security-audit issue #8",
		},
		{
			name: "clean close",
			calls: []providerCall{
				{args: labelArgs()},
				{args: listArgs(), out: `[[{"number":7,"title":"security-audit: workflow findings"}]]`},
				{args: []string{"issue", "close", "7", "--repo", "owner/repository", "--comment", cleanComment}, err: providerFailure},
			},
			want: "close security-audit issue #7",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := &scriptedProvider{t: t, calls: test.calls}
			_, err := reconcile(context.Background(), provider, "owner/repository", test.findings, time.Time{})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
			provider.done()
		})
	}
}

func TestValidateRepositoryRejectsUntrustedShapesBeforeProviderCall(t *testing.T) {
	for _, repository := range []string{"", "owner", "owner/repo/extra", "https://example.com/o/r", "owner/repo?x=1", "owner name/repo", "../repo", "owner/.."} {
		t.Run(repository, func(t *testing.T) {
			provider := &scriptedProvider{t: t}
			_, err := reconcile(context.Background(), provider, repository, findings{}, time.Time{})
			if err == nil {
				t.Fatal("expected repository validation error")
			}
			provider.done()
		})
	}
}

func TestFindingsFromEnvironmentUsesWorkflowContract(t *testing.T) {
	values := map[string]string{
		"compromised":   "c\r\n1",
		"unpinned":      "u\r\n2",
		"perm_findings": "p\r\n3",
		"prt_hits":      "t\r\n4",
	}
	got := findingsFromEnvironment(func(key string) string { return values[key] })
	want := findings{compromised: "c\n1", unpinned: "u\n2", permissions: "p\n3", pullRequestTarget: "t\n4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("findings = %#v, want %#v", got, want)
	}
}

func labelArgs() []string {
	return []string{"label", "create", issueLabel, "--repo", "owner/repository", "--color", issueLabelColor,
		"--description", issueLabelDetail, "--force"}
}

func listArgs() []string {
	return []string{"api", "--paginate", "--slurp",
		"repos/owner/repository/issues?state=open&labels=security-audit&per_page=100"}
}

func createArgs() []string {
	return []string{"issue", "create", "--repo", "owner/repository", "--title", issueTitle,
		"--body-file", "-", "--label", issueLabel}
}
