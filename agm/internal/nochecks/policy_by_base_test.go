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
	perBase := time.Minute
	prs := []PR{
		{Number: 2, BaseRefName: "zeta"},
		{Number: 1, BaseRefName: "alpha"},
	}
	// One shared deadline, still owned by the constructor, but sized for the
	// bases actually admitted rather than for a single call. Two distinct
	// bases, each of which performs two sequential provider reads.
	timeout := requiredChecksScanBudget(perBase, 2)
	started := time.Now()
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
		perBase,
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

func TestResolveRequiredChecksByBaseStopsBeforeNextFetchAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	prs := []PR{
		{Number: 2, BaseRefName: "zeta"},
		{Number: 1, BaseRefName: "alpha"},
	}
	var calls []string
	fetch := func(_ context.Context, base string) (map[string]bool, error) {
		calls = append(calls, base)
		cancel()
		return map[string]bool{"Build": true}, nil
	}

	got, err := resolveRequiredChecksByBase(ctx, prs, fetch)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("resolveRequiredChecksByBase() = %#v, %v; want caller cancellation", got, err)
	}
	if want := []string{"alpha"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("fetch calls = %v, want no fetch after cancellation %v", calls, want)
	}
	if got.byBase != nil {
		t.Fatalf("caller cancellation leaked partial policies %#v", got.byBase)
	}
}

// The scan-wide deadline must be sized for the bases actually admitted.
//
// Each base performs two sequential provider reads, so reusing the
// single-call allowance as one shared budget let a stacked queue exhaust it
// while every individual request stayed inside its normal allowance. A
// timeout aborts before any check-run classification, so the recovery scanner
// goes offline rather than degrading.
func TestRequiredChecksScanBudgetScalesWithBases(t *testing.T) {
	const perCall = 20 * time.Second
	for _, tc := range []struct {
		bases int
		want  time.Duration
	}{
		{bases: 0, want: perCall},
		{bases: 1, want: perCall},
		{bases: 4, want: 4 * perCall},
		{bases: maxRequiredChecksScanBases, want: maxRequiredChecksScanBases * perCall},
		// Bounded, so a pathological listing cannot make the scan hang.
		{bases: maxRequiredChecksScanBases + 50, want: maxRequiredChecksScanBases * perCall},
	} {
		if got := requiredChecksScanBudget(perCall, tc.bases); got != tc.want {
			t.Errorf("requiredChecksScanBudget(%s, %d) = %s, want %s", perCall, tc.bases, got, tc.want)
		}
	}
}

// Several bases, each consuming most of a per-call allowance, must all resolve
// rather than the later ones being cut off by a budget sized for one.
func TestMultipleBasesEachGetTheirOwnAllowance(t *testing.T) {
	prs := []PR{
		{Number: 1, BaseRefName: "main"},
		{Number: 2, BaseRefName: "release"},
		{Number: 3, BaseRefName: "staging"},
		{Number: 4, BaseRefName: "next"},
	}
	perCall := 60 * time.Millisecond
	fetched := 0
	resolved, err := fetchRequiredChecksByBaseWithin(context.Background(), "owner/repo", prs, perCall,
		func(ctx context.Context, _, base string) (map[string]bool, error) {
			// Most of one per-call allowance, as a real pair of reads would be.
			select {
			case <-time.After(perCall * 3 / 4):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			fetched++
			return map[string]bool{"CI": true}, nil
		})
	if err != nil {
		t.Fatalf("fetchRequiredChecksByBaseWithin() error = %v, want every base resolved", err)
	}
	if fetched != 4 {
		t.Errorf("fetched %d bases, want 4", fetched)
	}
	for _, base := range []string{"main", "release", "staging", "next"} {
		if resolved.byBase[base] == nil {
			t.Errorf("base %q missing from the resolved policy", base)
		}
	}
}
