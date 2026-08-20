package infraattest

import (
	"errors"
	"fmt"
)

// Code is a stable, public-safe rejection reason. Codes intentionally omit
// resource addresses, repository names, state values, and provider payloads.
type Code string

const (
	// CodeInvalidInput and the following constants form the stable public-safe rejection catalog.
	CodeInvalidInput Code = "invalid-input"
	// CodeInputTooLarge rejects evidence beyond a declared byte bound.
	CodeInputTooLarge Code = "input-too-large"
	// CodeMalformedJSON rejects invalid or non-unique JSON.
	CodeMalformedJSON Code = "malformed-json"
	// CodeUnsupportedToolchain rejects an untrusted toolchain.
	CodeUnsupportedToolchain Code = "unsupported-toolchain"
	// CodeMalformedLockfile rejects a dependency lock outside the exact contract.
	CodeMalformedLockfile Code = "malformed-lockfile"
	// CodeMalformedPlan rejects invalid plan JSON.
	CodeMalformedPlan Code = "malformed-plan"
	// CodeUnsupportedPlanFormat rejects an unsupported plan schema or tool version.
	CodeUnsupportedPlanFormat Code = "unsupported-plan-format"
	// CodePlanErrored rejects an errored plan.
	CodePlanErrored Code = "plan-errored"
	// CodePlanProfile rejects a non-routine plan invocation.
	CodePlanProfile Code = "non-routine-plan-profile"
	// CodeInventoryIncomplete rejects an incomplete inventory projection.
	CodeInventoryIncomplete Code = "inventory-incomplete"
	// CodeMigrationSurface rejects critical state or configuration migrations.
	CodeMigrationSurface Code = "critical-migration-surface"
	// CodeBaselineMissing rejects authorization without a prior receipt.
	CodeBaselineMissing Code = "baseline-missing"
	// CodeBaselineMismatch rejects moved baseline evidence.
	CodeBaselineMismatch Code = "baseline-mismatch"
	// CodePlanUnknown rejects unknown planned values.
	CodePlanUnknown Code = "plan-has-unknown-values"
	// CodePlanSensitive rejects sensitive planned changes.
	CodePlanSensitive Code = "plan-has-sensitive-values"
	// CodePlanChecks rejects a non-passing check result.
	CodePlanChecks Code = "plan-checks-not-passing"
	// CodePlanOutputs rejects output changes.
	CodePlanOutputs Code = "plan-has-output-changes"
	// CodePlanMove rejects resource-address movement.
	CodePlanMove Code = "plan-has-move"
	// CodePlanImport rejects imports or generated configuration.
	CodePlanImport Code = "plan-has-import"
	// CodePlanDeposed rejects deposed resource objects.
	CodePlanDeposed Code = "plan-has-deposed-object"
	// CodePlanCreate rejects resource creation.
	CodePlanCreate Code = "plan-has-create"
	// CodePlanDelete rejects deletion or state forgetting.
	CodePlanDelete Code = "plan-has-delete-or-forget"
	// CodePlanReplace rejects resource replacement.
	CodePlanReplace Code = "plan-has-replacement"
	// CodePlanRead rejects refresh-only reads.
	CodePlanRead Code = "plan-has-read"
	// CodePlanResourceType rejects resources outside the routine allowlist.
	CodePlanResourceType Code = "plan-has-non-routine-resource"
	// CodePlanAmbiguous rejects a plan that is not uniquely routine.
	CodePlanAmbiguous Code = "plan-is-ambiguous"
	// CodeRulesetBinding rejects ruleset identity movement.
	CodeRulesetBinding Code = "ruleset-binding-mismatch"
	// CodeRulesetProjection rejects a non-exact desired after-state.
	CodeRulesetProjection Code = "ruleset-projection-mismatch"
	// CodeFreshness rejects an invalid authorization lifetime.
	CodeFreshness Code = "freshness-invalid"
	// CodeAuthorizationMismatch rejects a changed authorization binding.
	CodeAuthorizationMismatch Code = "authorization-mismatch"
	// CodeReceiptPreconditions rejects incomplete post-application evidence.
	CodeReceiptPreconditions Code = "receipt-preconditions-not-met"
	// CodeReceiptMismatch rejects a changed receipt binding.
	CodeReceiptMismatch Code = "receipt-mismatch"
)

// Rejection is the only error emitted by this module. Error text is bounded
// and contains only a stable code, so callers cannot accidentally log private
// plan or state material by printing it.
type Rejection struct {
	Code Code
}

func (e *Rejection) Error() string {
	return fmt.Sprintf("infra plan policy rejected: %s", e.Code)
}

func reject(code Code) error {
	return &Rejection{Code: code}
}

// RejectionCode returns the stable public code for err, or CodeInvalidInput
// for errors outside this module's rejection envelope.
func RejectionCode(err error) Code {
	var typed *Rejection
	if errors.As(err, &typed) {
		return typed.Code
	}
	return CodeInvalidInput
}
