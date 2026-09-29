package safegit

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

type providerMergeStage uint8

const (
	providerMergeCommandStage providerMergeStage = iota + 1
	providerMergeConfirmationStage
)

type providerMergeFailure struct {
	stage providerMergeStage
	err   error
}

// runProviderMergeTransaction owns the provider-mutation lifetime: it runs the
// provider, requires exact-head confirmation, and records confirmation. It
// never changes local recovery state. Worktree and branch removal belongs to
// sanctioned session cleanup, where ownership and liveness can be established.
func runProviderMergeTransaction(
	ctx context.Context,
	mergeArgs []string,
	confirm func() error,
	onConfirmed func(),
) *providerMergeFailure {
	if err := ctx.Err(); err != nil {
		return &providerMergeFailure{stage: providerMergeCommandStage, err: err}
	}
	mergeCmd := exec.CommandContext(ctx, mergeArgs[0], mergeArgs[1:]...)
	mergeCmd.Stdout = os.Stdout
	mergeCmd.Stderr = os.Stderr
	if err := mergeCmd.Run(); err != nil {
		if ctx.Err() == nil {
			return &providerMergeFailure{stage: providerMergeCommandStage, err: err}
		}
		// Cancellation can race with provider acceptance. Treat the outcome as
		// indeterminate until exact-head confirmation establishes provider truth.
	}
	if err := confirm(); err != nil {
		return &providerMergeFailure{stage: providerMergeConfirmationStage, err: err}
	}
	if onConfirmed != nil {
		onConfirmed()
	}
	fmt.Fprintln(os.Stderr,
		"safe-merge: no local worktree or ref was removed; sanctioned session cleanup owns any removal")
	return nil
}
