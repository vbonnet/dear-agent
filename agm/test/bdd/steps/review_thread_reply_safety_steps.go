package steps

import (
	"context"
	"fmt"

	"github.com/cucumber/godog"
)

type reviewThreadReplySafetyState struct {
	output string
	err    error
}

type reviewThreadReplySafetyStateKey struct{}

// RegisterReviewThreadReplySafetySteps registers the data-only review reply behavior.
func RegisterReviewThreadReplySafetySteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, reviewThreadReplySafetyStateKey{}, &reviewThreadReplySafetyState{}), nil
	})

	ctx.Step(`^AGM runs the review reply data-only regressions$`, agmRunsReviewReplyDataOnlyRegressions)
	ctx.Step(`^reply-resolve should require a body-file source$`, replyResolveShouldRequireBodyFileSource)
	ctx.Step(
		`^invalid body sources should fail before GitHub mutation$`,
		invalidBodySourcesShouldFailBeforeGitHubMutation,
	)
	ctx.Step(
		`^oversized and endless body sources should be bounded before GitHub mutation$`,
		oversizedAndEndlessBodySourcesShouldBeBoundedBeforeGitHubMutation,
	)
	ctx.Step(
		`^a replaced named source should fail without blocking$`,
		replacedNamedSourceShouldFailWithoutBlocking,
	)
	ctx.Step(
		`^GitHub should receive the exact reply bytes without shell evaluation$`,
		githubShouldReceiveExactReplyBytesWithoutShellEvaluation,
	)
	ctx.Step(
		`^failed provider diagnostics should not echo the reply body$`,
		failedProviderDiagnosticsShouldNotEchoReplyBody,
	)
	ctx.Step(
		`^body-bearing access denials should remain redacted and require credential repair$`,
		bodyBearingAccessDenialsShouldRemainRedactedAndRequireCredentialRepair,
	)
	ctx.Step(
		`^body-free provider diagnostics should remain available$`,
		bodyFreeProviderDiagnosticsShouldRemainAvailable,
	)
	ctx.Step(
		`^retry guidance should reuse the body-file source without rendering its content$`,
		retryGuidanceShouldReuseBodyFileWithoutRenderingContent,
	)
	ctx.Step(
		`^posted-reply continuation receipts should authenticate host-bound body, identity, time, and edit evidence without reposting$`,
		postedReplyContinuationReceiptsShouldBindEvidenceWithoutReposting,
	)
	ctx.Step(
		`^continuation issuance should accept a safe shared state namespace while keeping command-owned issuer state private and preflighted$`,
		continuationIssuerStateShouldBePrivateDurableAndPreflighted,
	)
	ctx.Step(
		`^unsupported continuation platforms should fail before reply mutation and before continuation body-source or provider access$`,
		unsupportedContinuationPlatformsShouldFailClosed,
	)
	ctx.Step(
		`^continuation environment failures should retain the exact receipt and source for issuer-state restoration$`,
		continuationEnvironmentFailuresShouldRetainReceiptAndSource,
	)
	ctx.Step(
		`^continuation replay should require the unchanged current provider-visible extant boundary$`,
		continuationReplayShouldRequireCurrentExtantBoundary,
	)
	ctx.Step(
		`^unreceipted existing replies should not be rebound into fresh temporal evidence$`,
		unreceiptedExistingRepliesShouldNotBeRebound,
	)
	ctx.Step(
		`^changed retry bodies should not bypass provider-history deduplication while reviewer hand-back permits revision$`,
		changedRetryBodiesShouldNotBypassProviderHistory,
	)
	ctx.Step(
		`^resolved reviewer hand-backs should never be skipped as terminal$`,
		resolvedReviewerHandbacksShouldNeverBeSkipped,
	)
	ctx.Step(
		`^generated reply-file guidance should prescribe per-thread external creation, retry retention, and confirmed-terminal cleanup$`,
		generatedReplyFileGuidanceShouldStayOutsideWorktreeThroughTerminalCleanup,
	)
	ctx.Step(
		`^superseded reply guidance should revise the same source without losing its cleanup identity$`,
		supersededReplyGuidanceShouldReviseSameSourceWithoutLosingCleanupIdentity,
	)
	ctx.Step(
		`^ambiguous reply outcomes should preserve the full predecessor identity, time, edit, and author boundary before choosing unchanged retry or same-source revision$`,
		ambiguousReplyOutcomesShouldPreservePredecessorEvidence,
	)
	ctx.Step(
		`^stale resolved continuation mismatches should reopen before recovery guidance$`,
		staleResolvedContinuationMismatchesShouldReopen,
	)
	ctx.Step(
		`^resolution-anchor recovery should distinguish unverifiable absence from confirmed change$`,
		resolutionAnchorRecoveryShouldDistinguishAbsenceFromChange,
	)
	ctx.Step(
		`^resolved buried or jumped replies should reopen before recovery guidance$`,
		resolvedPlacementMismatchesShouldReopenBeforeGuidance,
	)
	ctx.Step(
		`^uncertain thread evidence should retain the actual source pending inspection$`,
		uncertainThreadEvidenceShouldRetainSourcePendingInspection,
	)
	ctx.Step(
		`^unchanged and revised guidance should preserve the caller's named path or standard-input form$`,
		recoveryGuidanceShouldPreserveCallerSourceForm,
	)
	ctx.Step(
		`^every mutation response should prove the requested thread identity, state, and answer-author boundary$`,
		mutationResponsesShouldProveRequestedIdentityAndState,
	)
	ctx.Step(
		`^moved resolved tails should reopen before already-resolved guidance$`,
		movedResolvedTailsShouldReopenBeforeAlreadyResolvedGuidance,
	)
	ctx.Step(
		`^incomplete comment identities should require inspection$`,
		incompleteCommentIdentitiesShouldRequireInspection,
	)
	ctx.Step(
		`^resolve transport errors should be classified from a fresh state read$`,
		resolveTransportErrorsShouldBeClassifiedFromFreshState,
	)
	ctx.Step(
		`^unresolve transport errors should reconcile fresh state before claims$`,
		unresolveTransportErrorsShouldReconcileFreshStateBeforeClaims,
	)
	ctx.Step(
		`^aggregate refusal summaries should report only confirmed outcomes$`,
		aggregateRefusalSummariesShouldReportOnlyConfirmedOutcomes,
	)
}

func agmRunsReviewReplyDataOnlyRegressions(ctx context.Context) error {
	state, err := getReviewThreadReplySafetyState(ctx)
	if err != nil {
		return err
	}
	state.output, state.err = runLocalGuardrailNamedGoTests(ctx,
		"./cmd/resolve-review-threads",
		"TestReplyResolveRequiresBodyFile",
		"TestReplyResolveRejectsInvalidBodySourcesBeforeProviderMutation",
		"TestLoadReplyBodyPreservesExactBytes",
		"TestLoadReplyBodyRejectsOversizedOrEndlessSources",
		"TestLoadReplyBodyRejectsReplacedFIFOWithoutBlocking",
		"TestLoadReplyBodyUsesNonblockingOpenerWithoutPathStat",
		"TestGHGraphQLSendsQueryAndVariablesAsJSONStdin",
		"TestGHGraphQLSuppressesReplyBodyEchoedByChildStderr",
		"TestBodyBearingAccessDenialIsRedactedAndRequiresCredentialRepair",
		"TestResolveAccessDenialCannotBeRecoveredAsAppliedMutation",
		"TestFailedCorrectiveReopenPreservesAccessDenialCause",
		"TestDeniedResolveWithChangedUnresolvedEvidencePreservesAccessCause",
		"TestResolveErrorRecoveryReopensChangedAuthorEvidence",
		"TestResolveErrorRecoveryPrioritizesChangedAnchorOverAuthorEvidence",
		"TestResolveTransportRecoveryNeverAttributesConcurrentMatchingState",
		"TestDirectUnresolveAccessDenialUsesNeutralConfirmedStateGuidance",
		"TestDeniedReplyCannotAdoptIndependentlyObservedExactBody",
		"TestDeniedObservedReplyPrioritizesChangedPredecessorIDOverMissingBody",
		"TestDeniedObservedReplyReopensResolvedUnansweredAuthorEvidence",
		"TestDeniedAmbiguousReplyRetainsCredentialRepairAcrossChangedEvidence",
		"TestResolveAllAbortsOnAccessDeniedSupersessionCause",
		"TestReplyBodyAccessMarkersDoNotForgeAccessDenial",
		"TestRedactedProviderDiagnosticClassifier",
		"TestBodyBearingProviderDropsDebugEnvironment",
		"TestBodySelectingProviderFailureSuppressesPayloadAndKeepsAccessCategory",
		"TestBodyFreeAmbiguousDiagnosticsStayTransportErrors",
		"TestIsAccessDenied",
		"TestGHGraphQLPreservesBodyFreeChildStderr",
		"TestRetryAdviceDoesNotRenderReplyBody",
		"TestAmbiguousPostFailureRetainsUnchangedBodySource",
		"TestContinueResolveRejectsInvalidReceiptBeforeProviderAccess",
		"TestContinuationReceiptHMACRoundTripAndTamperRejection",
		"TestContinueResolveEnvironmentFailuresRetainReceiptBeforeBodyOrProvider",
		"TestEffectiveContinuationProviderHostDefaultsAndCanonicalizes",
		"TestReplyResolvePreflightsSigningKeyBeforeReplyMutation",
		"TestContinuationReceiptKeyIsPrivateStableAndReused",
		"TestContinuationReceiptKeyAcceptsSharedStateNamespace",
		"TestContinuationReceiptKeyRejectsUnsafeSharedStateNamespace",
		"TestLoadContinuationReceiptKeyRejectsUnsafeSharedStateNamespace",
		"TestConcurrentContinuationReceiptKeyCreationConverges",
		"TestReadContinuationReceiptKeyRejectsUnsafeLeaf",
		"TestReadContinuationReceiptKeyRejectsSymlink",
		"TestReadContinuationReceiptKeyRejectsFIFOWithoutBlocking",
		"TestEnsurePrivateContinuationDirectoryRejectsManagedSymlink",
		"TestEnsurePrivateContinuationDirectoryRetriesEveryParentSync",
		"TestExplicitXDGDirectoryPlanSyncsOnlyManagedDescendants",
		"TestHomeFallbackDirectoryPlanPreservesAnchorAndSyncOrder",
		"TestExplicitXDGDirectoryPlanRequiresExistingBoundary",
		"TestExplicitXDGStateRootBelowExecuteOnlyAncestor",
		"TestEnsurePrivateContinuationDirectoryRejectsPublicFinalDirectory",
		"TestReplyResolveRequiresExistingExplicitXDGStateRootBeforeMutation",
		"TestReplyResolveAcceptsHomeFallbackSharedStateBeforeMutation",
		"TestReplyResolveRejectsWritableSharedStateBeforeMutation",
		"TestContinuationPlatformBoundaryRejectsNonUnix",
		"TestReplyResolveRejectsUnsupportedContinuationPlatformBeforeMutation",
		"TestContinueResolveRejectsUnsupportedPlatformBeforeBodyOrProvider",
		"TestHelpDocumentsUnixOnlyContinuationBoundary",
		"TestHelpDocumentsStateRootOwnership",
		"TestHelpUsesCanonicalFirstAnswerGuidance",
		"TestObservedEditRevisionRequiresCountAndLastNode",
		"TestProviderQueriesRequestLastEditNodeID",
		"TestProviderReadsCarryLastEditNodeIDs",
		"TestReplyMutationEvidenceCarriesLastEditNodeID",
		"TestValidateContinuationHistoryRejectsChangedLastEditIDAtFixedCount",
		"TestRecoveredReplyRejectsChangedLastEditIDAtFixedCount",
		"TestContinuationBoundaryRejectsChangedLastEditIDAtFixedCount",
		"TestReplyHistoryBoundaryRejectsChangedLastEditIDAtFixedCount",
		"TestValidateContinuationHistory",
		"TestContinueResolveRejectsMismatchedProviderThreadIdentity",
		"TestReplyResolveRefusesHistoryThatAdvancedPastInitialTail",
		"TestReplyResolveRefusesResolvedStateDriftBeforeMutation",
		"TestReplyResolveRefusesSameIDPredecessorBodyEditBeforePosting",
		"TestContinueResolveRefusesEditedBodyAtSamePreResolveAnchor",
		"TestContinueResolveReopensWhenResolveResponseBodyChangedAtSameAnchor",
		"TestContinueResolveRefusesPredecessorBodyEditAfterReceipt",
		"TestReplyResolveReopensResolvedThreadWhenPredecessorEditedBeforePlacementVerification",
		"TestContinueResolveRefusesSameIDPredecessorEditAtImmediatePreResolveRead",
		"TestContinueResolveReopensWhenResolveResponsePredecessorEditedAtSameID",
		"TestReplyResolveInitiallyResolvedExactReplyStopsAfterStableClassification",
		"TestReplyResolveNeverSkipsResolvedReviewerHandbackAsTerminal",
		"TestStableReviewerHandbackWithUnknownAuthorNeverLeavesStaleResolution",
		"TestInitiallyResolvedSupersededReplyRequiresExplicitReopen",
		"TestReplyResolveRefusesChangedOrMissingStableAuthorEvidenceBeforePosting",
		"TestAmbiguousReplyRejectsSameIDPredecessorEditBeforeUnchangedRetry",
		"TestAmbiguousAbsentReplyBindsFullPrePostPredecessorSnapshot",
		"TestAmbiguousAbsentReplyRequiresBothRecoveryReadsToMatchSnapshot",
		"TestAmbiguousReplyReconcilesDeletedPredecessorBeforeRecovery",
		"TestAmbiguousReplyRecoversDelayedVisibleExactReply",
		"TestInitialHistoryMismatchReopensConcurrentResolution",
		"TestContinueResolveReopensAlreadyResolvedBuriedReceiptEvidence",
		"TestContinueResolveReopensAlreadyResolvedSameIDPredecessorEdit",
		"TestContinueResolveTreatsMissingResolvedIdentityAsUnverified",
		"TestContinueResolveCarriesConclusiveHistoryContradictionAcrossOmittedCurrentEvidence",
		"TestContinueResolveLeavesResolvedThreadUnchangedWhenFullHistoryIsIncomplete",
		"TestAmbiguousReplyAccessDeniedStateReadRequiresCredentialRepair",
		"TestContinuationMismatchAccessDeniedStateReadRequiresCredentialRepair",
		"TestReplyResolveRefusesToRebindExistingExactReplyWithoutReceipt",
		"TestReplyResolveRefusesTrimEquivalentByteDifferentTail",
		"TestReplyResolveResolvedShortcutRefusesSameIDTrimEquivalentBodyEdit",
		"TestReplyResolveEmitsReplayableContinuationReceipt",
		"TestContinueResolveUsesReceiptWithoutReplyMutation",
		"TestContinueResolveAuthorEvidenceRequiresIdentityRepair",
		"TestContinueResolveRejectsChangedNamedBodyBeforeProviderAccess",
		"TestContinueResolveRejectsChangedStdinBeforeProviderAccess",
		"TestReplyResolveRefusesChangedBodyAfterProviderVisibleReply",
		"TestReplyResolveAllowsRevisedBodyAfterReviewerHandback",
		"TestReplyResolveRefusesMissingAuthorEvidenceBeforeMutation",
		"TestReplyResolveDoesNotAdoptMatchingReviewerText",
		"TestReplyResolveResolvedDifferentAnswerDoesNotReopenOrClaimSuccess",
		"TestNewReplyBodyGuidanceUsesExternalTemporaryFileLifecycle",
		"TestRevisedReplyBodyGuidanceReusesExistingSource",
		"TestUnchangedReplyBodyGuidancePreservesExistingSource",
		"TestGeneratedReplyGuidanceRoutesThroughExternalLifecycle",
		"TestReplyResolutionEvidenceFailureEmitsOneLifecycle",
		"TestPostReplyRecoveryMatrixPreservesOriginalTail",
		"TestResolvedPlacementMismatchReopensBeforeRevisedGuidance",
		"TestResolutionMutationRecoveryDistinguishesUnverifiableFromSuperseded",
		"TestUnavailableOrEqualAuthorEvidenceInspectsAndRetains",
		"TestMutationResponsesRequireRequestedIdentityAndState",
		"TestResolveMutationRequestsAndParsesAtomicAuthorBoundary",
		"TestBareNonForceResolveReopensChangedAuthorBoundary",
		"TestExactNonForceResolveReopensChangedOpeningAuthor",
		"TestForcedResolveDoesNotRequireIndependentAuthorBoundary",
		"TestMovedResolvedTailReopensBeforeGuidance",
		"TestResolvedSupersededReplyWithUnknownAuthorIsReopened",
		"TestContinuationMismatchPrioritizesMovedLastIDOverMissingBody",
		"TestContinueResolvePreReadPrioritizesChangedPredecessorIDOverMissingBody",
		"TestStableHistoryPrioritizesChangedPredecessorIDOverMissingLastBody",
		"TestReplyPlacementPrioritizesChangedLastIDOverMissingPredecessorID",
		"TestResolveMutationEvidencePrioritizesChangedPredecessorIDOverMissingBody",
		"TestInitialHistoryMismatchPrioritizesChangedLastIDOverMissingBody",
		"TestIncompleteCommentIdentityRequiresInspection",
		"TestResolveErrorRecoveryClassifiesFreshState",
		"TestUnresolveErrorRecoveryReconcilesFreshState",
		"TestResolveAllRefusalSummaryIsTruthful",
	)
	if state.err != nil {
		return fmt.Errorf("resolve-review-threads data-only regressions: %w\n%s", state.err, state.output)
	}
	output, err := runLocalGuardrailNamedGoTests(ctx,
		"./cmd/pr-blockers",
		"TestUsageRoutesReviewThreadLifecycleToOwner",
		"TestSkillRoutesReviewThreadLifecycleToOwner",
		"TestReplyFileLifecycleHasOneProductionOwner",
	)
	state.output += "\n" + output
	state.err = err
	if state.err != nil {
		return fmt.Errorf("pr-blockers reply-file guidance regressions: %w\n%s", state.err, state.output)
	}
	output, err = runLocalGuardrailNamedGoTests(ctx,
		"./internal/safegit",
		"TestClassifyBlockers_OutdatedUnresolvedThreadBlocks",
		"TestThreadRemediationGuidance_IsRunnable",
	)
	state.output += "\n" + output
	state.err = err
	return nil
}

func replyResolveShouldRequireBodyFileSource(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func invalidBodySourcesShouldFailBeforeGitHubMutation(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func oversizedAndEndlessBodySourcesShouldBeBoundedBeforeGitHubMutation(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func replacedNamedSourceShouldFailWithoutBlocking(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func githubShouldReceiveExactReplyBytesWithoutShellEvaluation(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func failedProviderDiagnosticsShouldNotEchoReplyBody(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func bodyBearingAccessDenialsShouldRemainRedactedAndRequireCredentialRepair(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func bodyFreeProviderDiagnosticsShouldRemainAvailable(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func retryGuidanceShouldReuseBodyFileWithoutRenderingContent(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func postedReplyContinuationReceiptsShouldBindEvidenceWithoutReposting(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func continuationIssuerStateShouldBePrivateDurableAndPreflighted(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func unsupportedContinuationPlatformsShouldFailClosed(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func continuationEnvironmentFailuresShouldRetainReceiptAndSource(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func continuationReplayShouldRequireCurrentExtantBoundary(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func unreceiptedExistingRepliesShouldNotBeRebound(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func changedRetryBodiesShouldNotBypassProviderHistory(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func resolvedReviewerHandbacksShouldNeverBeSkipped(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func generatedReplyFileGuidanceShouldStayOutsideWorktreeThroughTerminalCleanup(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func supersededReplyGuidanceShouldReviseSameSourceWithoutLosingCleanupIdentity(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func ambiguousReplyOutcomesShouldPreservePredecessorEvidence(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func staleResolvedContinuationMismatchesShouldReopen(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func resolutionAnchorRecoveryShouldDistinguishAbsenceFromChange(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func resolvedPlacementMismatchesShouldReopenBeforeGuidance(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func uncertainThreadEvidenceShouldRetainSourcePendingInspection(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func recoveryGuidanceShouldPreserveCallerSourceForm(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func mutationResponsesShouldProveRequestedIdentityAndState(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func movedResolvedTailsShouldReopenBeforeAlreadyResolvedGuidance(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func incompleteCommentIdentitiesShouldRequireInspection(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func resolveTransportErrorsShouldBeClassifiedFromFreshState(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func unresolveTransportErrorsShouldReconcileFreshStateBeforeClaims(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func aggregateRefusalSummariesShouldReportOnlyConfirmedOutcomes(ctx context.Context) error {
	return requireReviewThreadReplySafetyRegressions(ctx)
}

func requireReviewThreadReplySafetyRegressions(ctx context.Context) error {
	state, err := getReviewThreadReplySafetyState(ctx)
	if err != nil {
		return err
	}
	if state.err != nil {
		return fmt.Errorf("review reply data-only regressions: %w\n%s", state.err, state.output)
	}
	return nil
}

func getReviewThreadReplySafetyState(ctx context.Context) (*reviewThreadReplySafetyState, error) {
	state, ok := ctx.Value(reviewThreadReplySafetyStateKey{}).(*reviewThreadReplySafetyState)
	if !ok || state == nil {
		return nil, fmt.Errorf("review thread reply safety state not initialized")
	}
	return state, nil
}
