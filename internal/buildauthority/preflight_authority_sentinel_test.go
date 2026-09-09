package buildauthority

import (
	"context"
	"strings"
	"testing"
)

func TestAuthorityPhysicalRootSentinelsCleanAdmissionAndRevalidation(t *testing.T) {
	fixture := newAuthorityBracketFixture(t)
	revalidator := fixture.revalidator(t)
	claim, outcome := revalidator.admitPhysicalRootSentinels(
		context.Background(),
		fixture.physicalRoot,
	)
	if !outcome.proved() {
		t.Fatalf("sentinel admission outcome = %+v", outcome)
	}
	if claim.root != fixture.physicalRoot || claim.seal != validPhysicalRootSentinelClaim {
		t.Fatalf("sentinel admission claim = %+v", claim)
	}
	requireAuthoritySentinelEvents(t, fixture.events,
		"sentinel:initial:go.mod#1",
		"sentinel:initial:go.work#1",
		"sentinel:initial:.git#1",
	)

	fixture.resetTrace()
	outcome = revalidator.revalidatePhysicalRootSentinels(context.Background(), claim)
	if !outcome.proved() {
		t.Fatalf("sentinel revalidation outcome = %+v", outcome)
	}
	requireAuthoritySentinelEvents(t, fixture.events,
		"sentinel:expected:go.mod#1",
		"sentinel:expected:go.work#1",
		"sentinel:expected:.git#1",
	)
}

func TestAuthorityPhysicalRootSentinelAdmissionFailsFastWithPrimitiveAttribution(
	t *testing.T,
) {
	fixture := newAuthorityBracketFixture(t)
	fixture.presentSentinels["go.work"] = true
	claim, outcome := fixture.revalidator(t).admitPhysicalRootSentinels(
		context.Background(),
		fixture.physicalRoot,
	)
	requireAuthorityPrimitiveRecord(t, outcome.primary, OperationValidate, CauseUnsupported)
	if outcome.later != nil || outcome.descriptorClose != nil {
		t.Fatalf("sentinel admission secondary outcomes = %+v", outcome)
	}
	if claim != (physicalRootSentinelClaim{}) {
		t.Fatalf("failed sentinel admission minted claim = %+v", claim)
	}
	requireAuthoritySentinelEvents(t, fixture.events,
		"sentinel:initial:go.mod#1",
		"sentinel:initial:go.work#1",
	)
}

func TestAuthorityPhysicalRootSentinelRevalidationAttemptsEveryNameAfterFailures(
	t *testing.T,
) {
	fixture := newAuthorityBracketFixture(t)
	revalidator := fixture.revalidator(t)
	claim, admission := revalidator.admitPhysicalRootSentinels(
		context.Background(),
		fixture.physicalRoot,
	)
	if !admission.proved() {
		t.Fatalf("sentinel admission outcome = %+v", admission)
	}
	fixture.resetTrace()
	fixture.failures["sentinel:expected:go.mod#1"] = newAuthorityPrimitiveFailure(
		OperationProbe,
		CausePermission,
	)
	fixture.failures["sentinel:expected:go.work#1"] = newAuthorityPrimitiveFailure(
		OperationCompare,
		CauseUnstable,
	)
	fixture.failures["sentinel:expected:.git#1"] = newAuthorityPrimitiveFailure(
		OperationProbe,
		CauseCanceled,
	)
	outcome := revalidator.revalidatePhysicalRootSentinels(
		context.Background(),
		claim,
	)
	requireAuthorityPrimitiveRecord(t, outcome.primary, OperationProbe, CausePermission)
	requireAuthorityPrimitiveRecord(t, outcome.later, OperationCompare, CauseUnstable)
	if outcome.descriptorClose != nil {
		t.Fatalf("sentinel revalidation descriptor-close outcome = %+v", outcome.descriptorClose)
	}
	requireAuthoritySentinelEvents(t, fixture.events,
		"sentinel:expected:go.mod#1",
		"sentinel:expected:go.work#1",
		"sentinel:expected:.git#1",
	)
}

func TestAuthorityPhysicalRootSentinelsRejectIncompleteBracketPrimitives(t *testing.T) {
	fixture := newAuthorityBracketFixture(t)
	primitives := fixture.primitives()
	primitives.brackets.probeSentinelAbsence = nil
	if primitives.brackets.valid() {
		t.Fatal("bracket primitives accepted a missing sentinel absence primitive")
	}
	revalidator, failure := newPreflightAuthorityRevalidatorWith(primitives)
	if failure != nil {
		t.Fatalf("construct A1-only revalidator: %+v", failure)
	}
	if revalidator.validForBrackets() {
		t.Fatal("incomplete revalidator passed bracket validation")
	}

	claim, outcome := revalidator.admitPhysicalRootSentinels(
		context.Background(),
		fixture.physicalRoot,
	)
	requireAuthorityPrimitiveRecord(
		t,
		outcome.primary,
		OperationValidate,
		CauseInternalInvariant,
	)
	if claim != (physicalRootSentinelClaim{}) {
		t.Fatalf("incomplete revalidator minted sentinel claim = %+v", claim)
	}

	outcome = revalidator.revalidatePhysicalRootSentinels(
		context.Background(),
		physicalRootSentinelClaim{
			root: fixture.physicalRoot,
			seal: validPhysicalRootSentinelClaim,
		},
	)
	requireAuthorityPrimitiveRecord(
		t,
		outcome.primary,
		OperationValidate,
		CauseInternalInvariant,
	)
	if len(fixture.events) != 0 {
		t.Fatalf("incomplete sentinel seam invoked primitives: %q", fixture.events)
	}
}

func requireAuthoritySentinelEvents(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("sentinel events = %q, want %q", got, want)
	}
}
