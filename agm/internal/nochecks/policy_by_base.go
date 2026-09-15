package nochecks

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"strings"
	"time"
)

// RequiredChecksByBase is a complete required-check policy snapshot for every
// non-draft pull request base admitted to one scan. The zero value is invalid;
// construct it with FetchRequiredChecksByBase so a caller cannot pass a
// partially populated policy map into Scan.
type RequiredChecksByBase struct {
	byBase map[string]map[string]bool
}

type requiredChecksFetchFunc func(context.Context, string) (map[string]bool, error)

// FetchRequiredChecksByBase resolves every distinct non-draft PR base under
// one total deadline. Any missing base, incomplete fetch, unsupported policy,
// or ambiguous nil result returns the zero value and no usable partial map.
func FetchRequiredChecksByBase(
	ctx context.Context,
	repo string,
	prs []PR,
) (RequiredChecksByBase, error) {
	return fetchRequiredChecksByBaseWithin(ctx, repo, prs, ghAPITimeout, fetchRequiredChecks)
}

// fetchRequiredChecksByBaseWithin is the timeout-owning constructor seam. The
// public constructor supplies the production deadline and provider adapter;
// tests supply a short deadline without sleeping for ghAPITimeout.
func fetchRequiredChecksByBaseWithin(
	ctx context.Context,
	repo string,
	prs []PR,
	timeout time.Duration,
	fetch func(context.Context, string, string) (map[string]bool, error),
) (RequiredChecksByBase, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return resolveRequiredChecksByBase(ctx, prs, func(ctx context.Context, base string) (map[string]bool, error) {
		return fetch(ctx, repo, base)
	})
}

func resolveRequiredChecksByBase(
	ctx context.Context,
	prs []PR,
	fetch requiredChecksFetchFunc,
) (RequiredChecksByBase, error) {
	bases := make(map[string]bool)
	for _, pr := range prs {
		if pr.IsDraft {
			continue
		}
		if strings.TrimSpace(pr.BaseRefName) == "" {
			return RequiredChecksByBase{}, fmt.Errorf("PR #%d has no provider-observed base branch", pr.Number)
		}
		bases[pr.BaseRefName] = true
	}

	ordered := make([]string, 0, len(bases))
	for base := range bases {
		ordered = append(ordered, base)
	}
	sort.Strings(ordered)

	resolved := RequiredChecksByBase{byBase: make(map[string]map[string]bool, len(ordered))}
	for _, base := range ordered {
		if err := ctx.Err(); err != nil {
			return RequiredChecksByBase{}, fmt.Errorf(
				"reading required checks for base %q: caller context ended: %w",
				base,
				err,
			)
		}
		policy, err := fetch(ctx, base)
		if err != nil {
			return RequiredChecksByBase{}, fmt.Errorf("reading required checks for base %q: %w", base, err)
		}
		if policy == nil {
			return RequiredChecksByBase{}, fmt.Errorf(
				"reading required checks for base %q returned ambiguous nil policy without an error",
				base,
			)
		}
		resolved.byBase[base] = cloneRequiredChecks(policy)
	}
	return resolved, nil
}

func cloneRequiredChecks(policy map[string]bool) map[string]bool {
	return maps.Clone(policy)
}
