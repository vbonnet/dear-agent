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
	perBaseTimeout time.Duration,
	fetch func(context.Context, string, string) (map[string]bool, error),
) (RequiredChecksByBase, error) {
	bases, err := distinctBases(prs)
	if err != nil {
		return RequiredChecksByBase{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, requiredChecksScanBudget(perBaseTimeout, len(bases)))
	defer cancel()

	return resolveRequiredChecksForBases(ctx, bases, func(ctx context.Context, base string) (map[string]bool, error) {
		return fetch(ctx, repo, base)
	})
}

// maxRequiredChecksScanBases bounds how far the scan-wide budget can grow. The
// budget scales with work, but a pathological listing must not be able to make
// a recovery scan run indefinitely.
const maxRequiredChecksScanBases = 32

// requiredChecksScanBudget sizes the shared deadline for the bases actually
// admitted.
//
// Each base performs two sequential provider reads, so reusing the single-call
// allowance as one budget for the whole loop let a stacked queue exhaust it
// even when every individual request completed inside its normal allowance.
// Any timeout aborts before check-run classification or retriggering, so the
// recovery scanner became unavailable rather than slower.
func requiredChecksScanBudget(perBase time.Duration, bases int) time.Duration {
	if bases < 1 {
		bases = 1
	}
	if bases > maxRequiredChecksScanBases {
		bases = maxRequiredChecksScanBases
	}
	return perBase * time.Duration(bases)
}

// distinctBases returns the sorted distinct non-draft bases in prs.
func distinctBases(prs []PR) ([]string, error) {
	bases := make(map[string]bool)
	for _, pr := range prs {
		if pr.IsDraft {
			continue
		}
		if strings.TrimSpace(pr.BaseRefName) == "" {
			return nil, fmt.Errorf("PR #%d has no provider-observed base branch", pr.Number)
		}
		bases[pr.BaseRefName] = true
	}
	ordered := make([]string, 0, len(bases))
	for base := range bases {
		ordered = append(ordered, base)
	}
	sort.Strings(ordered)
	return ordered, nil
}

func resolveRequiredChecksByBase(
	ctx context.Context,
	prs []PR,
	fetch requiredChecksFetchFunc,
) (RequiredChecksByBase, error) {
	ordered, err := distinctBases(prs)
	if err != nil {
		return RequiredChecksByBase{}, err
	}
	return resolveRequiredChecksForBases(ctx, ordered, fetch)
}

func resolveRequiredChecksForBases(
	ctx context.Context,
	ordered []string,
	fetch requiredChecksFetchFunc,
) (RequiredChecksByBase, error) {
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
