package retrolint

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultFileTimeout = 10 * time.Second

// ValidateGuard checks that a single guard satisfies all machine-verifiable contracts.
func ValidateGuard(ctx context.Context, repoRoot string, g *Guard) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	switch g.Type {
	case GuardTypeDeferred:
		if strings.TrimSpace(g.Bead) == "" || strings.TrimSpace(g.Reason) == "" {
			return fmt.Errorf("deferred guard requires both non-empty bead identifier and rationale")
		}
		g.Valid = true
		return nil

	case GuardTypeLaunchd:
		if strings.TrimSpace(g.Label) == "" {
			return fmt.Errorf("launchd guard requires non-empty label")
		}
		g.Valid = true
		return nil

	case GuardTypeTest, GuardTypeFile, GuardTypeHook, GuardTypeWorkflow, GuardTypeLint:
		return validatePathGuard(repoRoot, g)

	default:
		if g.Path != "" {
			return validatePathGuard(repoRoot, g)
		}
		if g.Label != "" {
			g.Type = GuardTypeLaunchd
			g.Valid = true
			return nil
		}
		if g.Bead != "" {
			g.Type = GuardTypeDeferred
			if strings.TrimSpace(g.Reason) == "" {
				return fmt.Errorf("deferred guard requires both non-empty bead identifier and rationale")
			}
			g.Valid = true
			return nil
		}
		return fmt.Errorf("guard requires a valid type (test, file, launchd, hook, workflow, lint, deferred)")
	}
}

func validatePathGuard(repoRoot string, g *Guard) error {
	p := strings.TrimSpace(g.Path)
	if p == "" {
		return fmt.Errorf("%s guard requires non-empty repository path", g.Type)
	}
	clean := filepath.Clean(strings.TrimPrefix(p, "/"))
	if strings.HasPrefix(clean, "..") {
		return fmt.Errorf("guard path %q escapes repository root", g.Path)
	}
	full := filepath.Join(repoRoot, clean)
	if _, err := os.Stat(full); err != nil {
		return fmt.Errorf("declared artifact %q does not exist in target repository", g.Path)
	}
	g.Valid = true
	return nil
}

// EvaluateRetrospective validates all declared guards for a retrospective.
func EvaluateRetrospective(ctx context.Context, repoRoot string, retro *Retrospective, baseline *Baseline) (*RetroResult, error) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultFileTimeout)
		defer cancel()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	waived := baseline != nil && baseline.Contains(retro.Path)
	res := &RetroResult{
		Path:   retro.Path,
		Date:   retro.Date,
		Guards: make([]Guard, len(retro.Guards)),
		Waived: waived,
	}
	copy(res.Guards, retro.Guards)

	hasValidGuard := false
	var validationErrors []string

	for i := range res.Guards {
		g := &res.Guards[i]
		if err := ValidateGuard(ctx, repoRoot, g); err != nil {
			g.Valid = false
			g.Error = err.Error()
			validationErrors = append(validationErrors, err.Error())
		} else {
			g.Valid = true
			hasValidGuard = true
		}
	}

	if !hasValidGuard {
		if waived {
			res.Status = StatusWaived
			res.Errors = nil
			return res, nil
		}
		res.Status = StatusFail
		res.Errors = append(res.Errors, validationErrors...)
		res.Errors = append(res.Errors, "retrospective must declare at least one machine-verifiable guard or tracked deferral (RLINT-01)")
		return res, nil
	}

	if len(validationErrors) > 0 {
		res.Status = StatusFail
		res.Errors = validationErrors
		return res, nil
	}

	res.Status = StatusPass
	return res, nil
}

// EvaluateRetrospectiveFile reads, parses, and evaluates a retrospective from disk.
func EvaluateRetrospectiveFile(ctx context.Context, repoRoot string, filePath string, baseline *Baseline) (*RetroResult, error) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultFileTimeout)
		defer cancel()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening retrospective: %w", err)
	}
	defer f.Close()

	retro, err := ParseRetrospective(ctx, f, filePath)
	if err != nil {
		return nil, err
	}

	return EvaluateRetrospective(ctx, repoRoot, retro, baseline)
}
