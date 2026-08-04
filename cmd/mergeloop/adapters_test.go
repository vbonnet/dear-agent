package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/vbonnet/dear-agent/internal/mergeloop"
	"github.com/vbonnet/dear-agent/internal/safegit"
)

func TestMergeLoopUsesSharedRequiredProjection(t *testing.T) {
	installMergeLoopFakeGH(t, `
case "$*" in
  "pr list"*) printf '%s\n' '[{"number":42,"title":"test","headRefName":"feature","headRefOid":"abc","isDraft":false,"mergeable":"MERGEABLE","mergeStateStatus":"UNSTABLE","reviewDecision":"","labels":[],"files":[]}]' ;;
  "pr checks 42 --repo owner/repo --json name,state") printf '%s\n' '[{"name":"Required","state":"SUCCESS"},{"name":"Advisory","state":"FAILURE"}]'; exit 1 ;;
  "pr view 42 --repo owner/repo --json baseRefName") printf '%s\n' '{"baseRefName":"main"}' ;;
  "api --paginate --slurp repos/owner/repo/rules/branches/main?per_page=100") printf '%s\n' '[[{"type":"required_status_checks","parameters":{"required_status_checks":[{"context":"Required"}]}}]]' ;;
  "api repos/owner/repo/branches/main/protection/required_status_checks") printf '%s\n' 'gh: Branch not protected (HTTP 404)' >&2; exit 1 ;;
  "pr checks 42 --repo owner/repo --required --json name,state") printf '%s\n' '[{"name":"Required","state":"SUCCESS"}]' ;;
  *) printf '%s\n' "unexpected gh invocation: $*" >&2; exit 2 ;;
esac
`)
	prs, err := (&ghLister{}).ListOpen(context.Background(), "owner/repo", 50)
	if err != nil {
		t.Fatalf("ListOpen() error = %v", err)
	}
	if len(prs) != 1 || len(prs[0].Checks) != 1 {
		t.Fatalf("provider-required checks = %#v", prs)
	}
	check := prs[0].Checks[0]
	if check.Name != "Required" || !check.Required || check.Verdict != mergeloop.CheckPass {
		t.Fatalf("normalized provider-required check = %#v", check)
	}
	if got := mergeloop.NewPolicy().Classify(prs[0], 0, false); got.State != mergeloop.StateGreen {
		t.Fatalf("advisory UNSTABLE history affected classification: %#v", got)
	}
}

func TestMergeLoopPreservesAuthoritativeEmptyFallback(t *testing.T) {
	installMergeLoopFakeGH(t, `
case "$*" in
  "pr list"*) printf '%s\n' '[{"number":42,"title":"test","headRefName":"feature","headRefOid":"abc","isDraft":false,"mergeable":"MERGEABLE","mergeStateStatus":"UNSTABLE","reviewDecision":"","labels":[],"files":[]}]' ;;
  "pr view 42 --repo owner/repo --json baseRefName") printf '%s\n' '{"baseRefName":"main"}' ;;
  "api --paginate --slurp repos/owner/repo/rules/branches/main?per_page=100") printf '%s\n' '[[]]' ;;
  "api repos/owner/repo/branches/main/protection/required_status_checks") printf '%s\n' 'gh: Branch not protected (HTTP 404)' >&2; exit 1 ;;
  "pr checks 42 --repo owner/repo --required --json name,state") printf '%s\n' "no required checks reported on the 'feature' branch" >&2; exit 1 ;;
  "pr checks 42 --repo owner/repo --json name,state") printf '%s\n' '[{"name":"Advisory","state":"FAILURE"}]'; exit 1 ;;
  *) printf '%s\n' "unexpected gh invocation: $*" >&2; exit 2 ;;
esac
`)
	prs, err := (&ghLister{}).ListOpen(context.Background(), "owner/repo", 50)
	if err != nil {
		t.Fatalf("ListOpen() error = %v", err)
	}
	if len(prs) != 1 || len(prs[0].Checks) != 1 {
		t.Fatalf("authoritative-empty fallback checks = %#v", prs)
	}
	check := prs[0].Checks[0]
	if check.Name != "Advisory" || !check.Required || check.Verdict != mergeloop.CheckFail {
		t.Fatalf("normalized fallback check = %#v", check)
	}
	if got := mergeloop.NewPolicy().Classify(prs[0], 0, false); got.State != mergeloop.StateCIFailing {
		t.Fatalf("failed fallback check classification = %#v", got)
	}
}

func TestMergeLoopMapsProjectedRequiredStatuses(t *testing.T) {
	checks, err := mergeLoopChecks([]safegit.RequiredCheck{
		{Name: "pass", Status: safegit.RequiredCheckPassing},
		{Name: "pending", Status: safegit.RequiredCheckPending},
		{Name: "fail", Status: safegit.RequiredCheckFailing},
	})
	if err != nil {
		t.Fatalf("mergeLoopChecks() error = %v", err)
	}
	want := []mergeloop.CheckVerdict{mergeloop.CheckPass, mergeloop.CheckPending, mergeloop.CheckFail}
	for i, verdict := range want {
		if checks[i].Verdict != verdict || !checks[i].Required {
			t.Fatalf("mapped check %d = %#v, want verdict %v and required", i, checks[i], verdict)
		}
	}
	if _, err := mergeLoopChecks([]safegit.RequiredCheck{{Name: "unknown", Status: 255}}); err == nil {
		t.Fatal("unknown safegit status must fail closed")
	}
}

func TestMergeLoopDefersOnlyUnavailableProjection(t *testing.T) {
	sentinel := errors.New("required projection unavailable")
	installMergeLoopFakeGH(t, `
case "$*" in
  "pr list"*) printf '%s\n' '[{"number":41,"title":"blocked"},{"number":42,"title":"ready"}]' ;;
  *) printf '%s\n' "unexpected gh invocation: $*" >&2; exit 2 ;;
esac
`)
	lister := &ghLister{project: func(_ context.Context, pr int, _ string) ([]safegit.RequiredCheck, error) {
		if pr == 41 {
			return nil, sentinel
		}
		return []safegit.RequiredCheck{{Name: "Build", Status: safegit.RequiredCheckPassing}}, nil
	}}
	prs, err := lister.ListOpen(context.Background(), "owner/repo", 50)
	if err != nil {
		t.Fatalf("ListOpen() error = %v", err)
	}
	if len(prs) != 2 {
		t.Fatalf("PR count = %d, want 2: %#v", len(prs), prs)
	}
	if prs[0].CheckProjectionError == "" || len(prs[0].Checks) != 0 {
		t.Fatalf("unavailable projection PR = %#v", prs[0])
	}
	if prs[1].CheckProjectionError != "" || len(prs[1].Checks) != 1 || prs[1].Checks[0].Verdict != mergeloop.CheckPass {
		t.Fatalf("later independent PR = %#v", prs[1])
	}
	policy := mergeloop.NewPolicy()
	if got := policy.Classify(prs[0], 0, false).State; got != mergeloop.StateCIPending {
		t.Fatalf("unavailable projection state = %s, want %s", got, mergeloop.StateCIPending)
	}
	if got := policy.Classify(prs[1], 0, false).State; got != mergeloop.StateGreen {
		t.Fatalf("later independent PR state = %s, want %s", got, mergeloop.StateGreen)
	}
}

func TestMergeLoopDefersOnlyUnknownProjectedStatus(t *testing.T) {
	installMergeLoopFakeGH(t, `
case "$*" in
  "pr list"*) printf '%s\n' '[{"number":41,"title":"unknown"},{"number":42,"title":"ready"}]' ;;
  *) printf '%s\n' "unexpected gh invocation: $*" >&2; exit 2 ;;
esac
`)
	lister := &ghLister{project: func(_ context.Context, pr int, _ string) ([]safegit.RequiredCheck, error) {
		if pr == 41 {
			return []safegit.RequiredCheck{{Name: "Build", Status: 255}}, nil
		}
		return []safegit.RequiredCheck{{Name: "Build", Status: safegit.RequiredCheckPassing}}, nil
	}}
	prs, err := lister.ListOpen(context.Background(), "owner/repo", 50)
	if err != nil {
		t.Fatalf("ListOpen() error = %v", err)
	}
	if len(prs) != 2 || prs[0].CheckProjectionError == "" || prs[1].CheckProjectionError != "" {
		t.Fatalf("normalization isolation = %#v", prs)
	}
}

func TestMergeLoopAbortsWhenParentContextCanceled(t *testing.T) {
	installMergeLoopFakeGH(t, `
case "$*" in
  "pr list"*) printf '%s\n' '[{"number":41,"title":"canceled"}]' ;;
  *) printf '%s\n' "unexpected gh invocation: $*" >&2; exit 2 ;;
esac
`)
	ctx, cancel := context.WithCancel(context.Background())
	lister := &ghLister{project: func(context.Context, int, string) ([]safegit.RequiredCheck, error) {
		cancel()
		return nil, context.Canceled
	}}
	_, err := lister.ListOpen(ctx, "owner/repo", 50)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("parent cancellation error = %v, want context.Canceled", err)
	}
}

func TestMergeLoopSkipsProjectionWhenOpenPRsExceedCap(t *testing.T) {
	installMergeLoopFakeGH(t, `
case "$*" in
  "pr list"*) printf '%s\n' '[{"number":41,"title":"one"},{"number":42,"title":"two"}]' ;;
  *) printf '%s\n' "unexpected gh invocation: $*" >&2; exit 2 ;;
esac
`)
	projectCalls := 0
	lister := &ghLister{project: func(context.Context, int, string) ([]safegit.RequiredCheck, error) {
		projectCalls++
		return nil, errors.New("projector should not be called")
	}}
	prs, err := lister.ListOpen(context.Background(), "owner/repo", 1)
	if err != nil {
		t.Fatalf("ListOpen() error = %v", err)
	}
	if projectCalls != 0 {
		t.Fatalf("required projection calls = %d, want 0", projectCalls)
	}
	if len(prs) != 2 || prs[0].Number != 41 || prs[1].Number != 42 {
		t.Fatalf("metadata-only PRs = %#v", prs)
	}
}

func installMergeLoopFakeGH(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "gh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nset -eu\n"+body), 0o700); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}
