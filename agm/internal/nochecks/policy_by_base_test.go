package nochecks

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestResolveRequiredChecksByBaseFetchesSortedDistinctBasesOnce(t *testing.T) {
	prs := []PR{
		{Number: 3, BaseRefName: "stack/zeta"},
		{Number: 1, BaseRefName: "main"},
		{Number: 2, BaseRefName: "stack/zeta"},
	}
	var calls []string
	fetch := func(_ context.Context, base string) (map[string]bool, error) {
		calls = append(calls, base)
		return map[string]bool{base + " check": true}, nil
	}

	got, err := resolveRequiredChecksByBase(context.Background(), prs, fetch)
	if err != nil {
		t.Fatalf("resolveRequiredChecksByBase() error = %v", err)
	}
	if want := []string{"main", "stack/zeta"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("fetch calls = %v, want sorted distinct bases %v", calls, want)
	}
	if got.byBase == nil || len(got.byBase) != 2 {
		t.Fatalf("resolved policies = %#v, want two initialized base entries", got.byBase)
	}
	if !got.byBase["main"]["main check"] || !got.byBase["stack/zeta"]["stack/zeta check"] {
		t.Fatalf("resolved policies = %#v, want each fetched policy under its own base", got.byBase)
	}
}

func TestResolveRequiredChecksByBaseSharesOneDeadlineAcrossBases(t *testing.T) {
	deadline := time.Now().Add(time.Minute)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	prs := []PR{
		{Number: 2, BaseRefName: "zeta"},
		{Number: 1, BaseRefName: "alpha"},
	}
	var first context.Context
	fetch := func(got context.Context, _ string) (map[string]bool, error) {
		if first == nil {
			first = got
		} else if got != first {
			t.Fatal("per-base fetch received a different context; want one shared policy deadline")
		}
		gotDeadline, ok := got.Deadline()
		if !ok || !gotDeadline.Equal(deadline) {
			t.Fatalf("fetch deadline = %v, %t; want shared deadline %v", gotDeadline, ok, deadline)
		}
		return map[string]bool{"Build": true}, nil
	}

	if _, err := resolveRequiredChecksByBase(ctx, prs, fetch); err != nil {
		t.Fatalf("resolveRequiredChecksByBase() error = %v", err)
	}
}

func TestFetchRequiredChecksByBaseWithinOwnsOneTotalDeadline(t *testing.T) {
	timeout := time.Minute
	started := time.Now()
	prs := []PR{
		{Number: 2, BaseRefName: "zeta"},
		{Number: 1, BaseRefName: "alpha"},
	}
	var first context.Context
	fetch := func(got context.Context, repo, _ string) (map[string]bool, error) {
		if repo != "owner/repo" {
			t.Fatalf("fetch repo = %q, want owner/repo", repo)
		}
		if first == nil {
			first = got
		} else if got != first {
			t.Fatal("per-base fetch received a different context; want one constructor-owned deadline")
		}
		deadline, ok := got.Deadline()
		if !ok {
			t.Fatal("fetch context has no deadline; constructor must own a total deadline")
		}
		if earliest := started.Add(timeout - time.Second); deadline.Before(earliest) {
			t.Fatalf("fetch deadline %v is earlier than expected constructor deadline %v", deadline, earliest)
		}
		if latest := started.Add(timeout + time.Second); deadline.After(latest) {
			t.Fatalf("fetch deadline %v is later than expected constructor deadline %v", deadline, latest)
		}
		return map[string]bool{"Build": true}, nil
	}

	if _, err := fetchRequiredChecksByBaseWithin(
		context.Background(),
		"owner/repo",
		prs,
		timeout,
		fetch,
	); err != nil {
		t.Fatalf("fetchRequiredChecksByBaseWithin() error = %v", err)
	}
}

func TestResolveRequiredChecksByBaseClonesFetchedPolicy(t *testing.T) {
	policy := map[string]bool{"Build": true}
	got, err := resolveRequiredChecksByBase(
		context.Background(),
		[]PR{{Number: 1, BaseRefName: "main"}},
		func(context.Context, string) (map[string]bool, error) { return policy, nil },
	)
	if err != nil {
		t.Fatalf("resolveRequiredChecksByBase() error = %v", err)
	}

	delete(policy, "Build")
	policy["Mutated"] = true
	resolved := got.byBase["main"]
	if !resolved["Build"] || resolved["Mutated"] || len(resolved) != 1 {
		t.Fatalf("resolved policy changed through source alias: %#v", resolved)
	}
}

func TestResolveRequiredChecksByBaseValidatesEveryCandidateBeforeFetching(t *testing.T) {
	prs := []PR{
		{Number: 1, BaseRefName: "main"},
		{Number: 2},
	}
	callCount := 0
	fetch := func(_ context.Context, _ string) (map[string]bool, error) {
		callCount++
		return map[string]bool{"Build": true}, nil
	}

	got, err := resolveRequiredChecksByBase(context.Background(), prs, fetch)
	if err == nil || !strings.Contains(err.Error(), "#2") {
		t.Fatalf("resolveRequiredChecksByBase() = %#v, %v; want missing-base error naming PR #2", got, err)
	}
	if callCount != 0 {
		t.Fatalf("fetch called %d time(s) before all candidate bases were validated", callCount)
	}
	if got.byBase != nil {
		t.Fatalf("failed validation returned usable policies %#v", got.byBase)
	}
}

func TestResolveRequiredChecksByBaseIgnoresDraftWithMissingBase(t *testing.T) {
	prs := []PR{{Number: 7, IsDraft: true}}
	fetch := func(_ context.Context, base string) (map[string]bool, error) {
		t.Fatalf("fetch called for ineligible draft base %q", base)
		return nil, nil
	}

	got, err := resolveRequiredChecksByBase(context.Background(), prs, fetch)
	if err != nil {
		t.Fatalf("resolveRequiredChecksByBase() error = %v", err)
	}
	if got.byBase == nil {
		t.Fatal("draft-only resolution returned an uninitialized owner")
	}
	if len(got.byBase) != 0 {
		t.Fatalf("draft-only resolution returned policies %#v, want none", got.byBase)
	}
}

func TestResolveRequiredChecksByBaseIgnoresDraftBaseAlongsideCandidate(t *testing.T) {
	prs := []PR{
		{Number: 7, BaseRefName: "draft-base", IsDraft: true},
		{Number: 8, BaseRefName: "main"},
	}
	var calls []string
	fetch := func(_ context.Context, base string) (map[string]bool, error) {
		calls = append(calls, base)
		return map[string]bool{"Build": true}, nil
	}

	got, err := resolveRequiredChecksByBase(context.Background(), prs, fetch)
	if err != nil {
		t.Fatalf("resolveRequiredChecksByBase() error = %v", err)
	}
	if want := []string{"main"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("policy fetches = %v, want only non-draft base %v", calls, want)
	}
	if len(got.byBase) != 1 || got.byBase["main"] == nil {
		t.Fatalf("resolved policies = %#v, want only initialized main policy", got.byBase)
	}
}

func TestResolveRequiredChecksByBaseRejectsMissingNonDraftBaseWithoutFetching(t *testing.T) {
	prs := []PR{{Number: 11}}
	callCount := 0
	fetch := func(_ context.Context, _ string) (map[string]bool, error) {
		callCount++
		return map[string]bool{}, nil
	}

	got, err := resolveRequiredChecksByBase(context.Background(), prs, fetch)
	if err == nil || !strings.Contains(err.Error(), "#11") {
		t.Fatalf("resolveRequiredChecksByBase() = %#v, %v; want missing-base error naming PR #11", got, err)
	}
	if callCount != 0 {
		t.Fatalf("fetch called %d time(s) for a PR with no base identity", callCount)
	}
	if got.byBase != nil {
		t.Fatalf("missing-base failure returned usable policies %#v", got.byBase)
	}
}

func TestResolveRequiredChecksByBaseRejectsNilPolicy(t *testing.T) {
	prs := []PR{{Number: 1, BaseRefName: "main"}}
	fetch := func(_ context.Context, _ string) (map[string]bool, error) {
		return nil, nil
	}

	got, err := resolveRequiredChecksByBase(context.Background(), prs, fetch)
	if err == nil || !strings.Contains(err.Error(), "main") {
		t.Fatalf("resolveRequiredChecksByBase() = %#v, %v; want ambiguous nil-policy error for main", got, err)
	}
	if got.byBase != nil {
		t.Fatalf("nil-policy failure returned usable policies %#v", got.byBase)
	}
}

func TestResolveRequiredChecksByBaseAcceptsNonNilEmptyPolicy(t *testing.T) {
	prs := []PR{{Number: 1, BaseRefName: "main"}}
	fetch := func(_ context.Context, _ string) (map[string]bool, error) {
		return map[string]bool{}, nil
	}

	got, err := resolveRequiredChecksByBase(context.Background(), prs, fetch)
	if err != nil {
		t.Fatalf("resolveRequiredChecksByBase() error = %v", err)
	}
	policy, ok := got.byBase["main"]
	if !ok {
		t.Fatalf("resolved policies = %#v, want main entry", got.byBase)
	}
	if policy == nil || len(policy) != 0 {
		t.Fatalf("main policy = %#v, want authoritative non-nil empty set", policy)
	}
}

func TestResolveRequiredChecksByBaseLaterFailureReturnsUnusableOwner(t *testing.T) {
	prs := []PR{
		{Number: 2, BaseRefName: "stack/zeta"},
		{Number: 1, BaseRefName: "main"},
	}
	var calls []string
	fetch := func(_ context.Context, base string) (map[string]bool, error) {
		calls = append(calls, base)
		if base == "stack/zeta" {
			return nil, errors.New("provider unavailable")
		}
		return map[string]bool{"Build": true}, nil
	}

	got, err := resolveRequiredChecksByBase(context.Background(), prs, fetch)
	if err == nil || !strings.Contains(err.Error(), "stack/zeta") {
		t.Fatalf("resolveRequiredChecksByBase() = %#v, %v; want later-base failure", got, err)
	}
	if want := []string{"main", "stack/zeta"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("fetch calls = %v, want %v", calls, want)
	}
	if got.byBase != nil {
		t.Fatalf("later fetch failure leaked partial policies %#v", got.byBase)
	}
}
