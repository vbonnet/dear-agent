package steps

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cucumber/godog"
)

type agmRuntimePackageGuardrailStateKey struct{}

type compactionVerificationState struct {
	output string
	err    error
}

type compactionVerificationStateKey struct{}

type compactionRegressionGroup struct {
	packagePath string
	testNames   []string
}

// RegisterAGMRuntimePackageGuardrailSteps verifies that AGM runtime support
// packages keep executable SPEC traceability.
func RegisterAGMRuntimePackageGuardrailSteps(ctx *godog.ScenarioContext) {
	registerPackageSpecGuardrailSteps(ctx, packageSpecGuardrailConfig{
		stateKey:          agmRuntimePackageGuardrailStateKey{},
		label:             "AGM runtime package",
		featurePath:       "agm/test/bdd/features/agm_runtime_package_guardrails.feature",
		configuredPattern: `^AGM runtime package "([^"]*)" is configured$`,
		validatePattern:   `^AGM validates AGM runtime package coverage$`,
		colocatedPattern:  `^AGM runtime package "([^"]*)" should have a co-located SPEC$`,
	})
	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		return context.WithValue(ctx, compactionVerificationStateKey{}, &compactionVerificationState{}), nil
	})
	ctx.Step(`^AGM runs the compaction observation provenance regressions$`, agmRunsCompactionObservationProvenanceRegressions)
	ctx.Step(`^non-live session observations should remain distinguishable$`, compactionVerificationRegressionsShouldPass)
	ctx.Step(`^AGM runs the compaction delivery authorization regressions$`, agmRunsCompactionDeliveryAuthorizationRegressions)
	ctx.Step(`^compaction delivery should require positive live-ready evidence$`, compactionVerificationRegressionsShouldPass)
	ctx.Step(`^AGM runs the compaction state ownership regressions$`, agmRunsCompactionStateOwnershipRegressions)
	ctx.Step(`^reused display-name state should require exact stable ownership$`, compactionVerificationRegressionsShouldPass)
	ctx.Step(`^AGM runs the durable compaction accounting regressions$`, agmRunsDurableCompactionAccountingRegressions)
	ctx.Step(`^compaction attempts should be keyed by stable identity and counted conservatively$`, compactionVerificationRegressionsShouldPass)
	ctx.Step(`^AGM runs the positive compaction verification regressions$`, agmRunsPositiveCompactionVerificationRegressions)
	ctx.Step(`^compaction completion should require active then stable ready evidence$`, compactionVerificationRegressionsShouldPass)
	ctx.Step(`^AGM runs the unverified compaction regressions$`, agmRunsUnverifiedCompactionRegressions)
	ctx.Step(`^ambiguous compaction outcomes should return non-success$`, compactionVerificationRegressionsShouldPass)
	ctx.Step(`^AGM runs the compaction command reporting regressions$`, agmRunsCompactionCommandReportingRegressions)
	ctx.Step(`^command output should claim completion only after positive proof$`, compactionVerificationRegressionsShouldPass)
	ctx.Step(`^AGM runs the compaction delivery outcome error regressions$`, agmRunsCompactionDeliveryOutcomeErrorRegressions)
	ctx.Step(`^compaction delivery errors should prohibit automatic retry and completion wording$`, compactionVerificationRegressionsShouldPass)
}

func getCompactionVerificationState(ctx context.Context) (*compactionVerificationState, error) {
	state, ok := ctx.Value(compactionVerificationStateKey{}).(*compactionVerificationState)
	if !ok || state == nil {
		return nil, fmt.Errorf("compaction verification scenario state is not initialized")
	}
	return state, nil
}

func agmRunsCompactionObservationProvenanceRegressions(ctx context.Context) error {
	return runCompactionRegressionGroups(ctx,
		compactionRegressionGroup{
			packagePath: "./agm/internal/session",
			testNames: []string{
				"TestDetectStateWithProbesPreservesObservationEvidence",
				"TestDetectStateWithProbesPreservesLegacyFallbackOnRefinementError",
				"TestDetectStateWithProbesSurfacesLivenessFailure",
				"TestProductionStateDetectorDoesNotMintGenericLiveEvidence",
			},
		},
		compactionRegressionGroup{
			packagePath: "./agm/internal/tmux",
			testNames: []string{
				"TestClassifyHarnessInputDistinguishesHumanDraftFromNativeProcessingTail",
				"TestCheckExpectedHarnessInputForPaneDoesNotFollowActivePaneFocusChange",
				"TestCheckExpectedHarnessInputForPaneRejectsReplacedForegroundHarnessPID",
				"TestCheckExpectedHarnessInputForPaneRejectsDeletedPane",
				"TestReadProcessTableWithArgsRejectsPIDReuseBetweenSnapshots",
			},
		},
	)
}

func agmRunsCompactionDeliveryAuthorizationRegressions(ctx context.Context) error {
	return runCompactionRegressionGroups(ctx,
		compactionRegressionGroup{
			packagePath: "./agm/internal/compaction",
			testNames: []string{
				"TestRunPreflight_RequiresPositiveLiveReadyEvidenceEvenWithForce",
				"TestValidateReadyRejectsCompatibilityDone",
			},
		},
		compactionRegressionGroup{
			packagePath: "./agm/internal/tmux",
			testNames: []string{
				"TestCheckExpectedHarnessInputAndSendHonorsCancellationDuringTmuxLockContention",
				"TestNewSessionBoundStoresStableSessionID",
				"TestBindStableSessionIDRejectsOverwriteAndClearsExactValue",
				"TestStableSessionAdoptionRefusesReusedTmuxIDsAfterServerRestart",
				"TestAtomicExpectedHarnessDeliveryReprovesExactTargetBeforeSend",
				"TestAtomicExpectedHarnessDeliveryPreservesSubmissionUncertainty",
				"TestAtomicExpectedHarnessDeliveryStrictlyReprovesExactTargetAfterSubmit",
				"TestAtomicExpectedHarnessDeliveryTreatsPostSubmitReproofFailureAsUncertain",
				"TestAtomicExpectedHarnessDeliveryLegacySkipsPostSubmitIdentityReproof",
				"TestAtomicExpectedHarnessDeliveryRequiresStableSessionBinding",
				"TestAtomicExpectedHarnessDeliveryPreservesSendOnLockReleaseFailure",
				"TestVerifyingEnter_StrictCaptureFailureIsSubmissionUncertain",
				"TestVerifyingEnterCancellationBeforeFirstEnterIsDefiniteNotSent",
				"TestVerifyingEnterCancellationAfterAcceptedEnterIsUncertain",
				"TestPasteBufferArgsForDeliveryUsesRawBracketedMultilineMode",
				"TestBracketedPasteModeObservationFailureIsDefiniteNotSent",
				"TestRawMultilineDeliveryRequiresLiveBracketedPasteMode",
				"TestAtomicPasteRefusesReusedTmuxIDsAfterServerRestart",
				"TestAtomicEnterRefusesReplacementWithoutSubmittingItsDraft",
				"TestStrictCompactionMutationRefusesAttachedHumanDraft",
				"TestClassifyExactCommandSubmissionStopsRetryAcrossGenericBusyHarnesses",
				"TestClassifyExactCommandSubmissionBindsPromptGlyphInsidePayload",
				"TestClassifyExactCommandSubmissionRetainsMoreThanFiveHundredParkedLines",
				"TestClassifyExactCommandSubmissionPartialOverlapIsAmbiguous",
				"TestClassifyExactCommandSubmissionAbsenceOrConcurrentClearIsAmbiguous",
				"TestVerifyingEnterStrictAmbiguousComposerStopsAfterOneEnter",
			},
		},
		compactionRegressionGroup{
			packagePath: "./agm/internal/ops",
			testNames: []string{
				"TestDeliverSessionCompactionReturnsExactRuntimeIdentity",
				"TestDeliverSessionCompactionUsesManifestFallbacks",
				"TestDeliverSessionCompactionNotReadyRecordsDefiniteNonDelivery",
				"TestDeliverSessionCompactionRejectsNonCompactionCommandBeforeResolution",
				"TestDeliverSessionCompactionRejectsTerminalControlsBeforeResolution",
				"TestDeliverSessionCompactionHonorsCanceledRequest",
				"TestDeliverSessionCompactionReloadsLifecycleUnderStableIDLock",
				"TestDeliverSessionCompactionRejectsReloadedNonActiveLifecycle",
				"TestDeliverSessionCompactionRejectsPureAPISession",
				"TestDeliverSessionCompactionRejectsActiveRenameBeforeAccounting",
				"TestDeliverSessionCompactionRejectsPreservationDriftBeforeAccounting",
				"TestDeliverSessionCompactionExactBindingRejectsSameNamedReplacement",
				"TestDeliverSessionCompactionExactBindingReportsMissingStableID",
				"TestDeliverSessionCompactionRequiresTrustedAccountingRoot",
				"TestCreateSession_HappyPath",
				"TestCreateSession_FailedReusePreservesExistingArtifacts",
				"TestResumeSessionColdStartCommitsPublicOutcome",
				"TestGetSession_ActiveOnlyNamePrefersLiveReplacementOverArchivedIdentity",
				"TestGetSession_ActiveOnlyExactArchivedIDStillResolves",
			},
		},
	)
}

func agmRunsCompactionStateOwnershipRegressions(ctx context.Context) error {
	return runCompactionRegressionGroups(ctx, compactionRegressionGroup{
		packagePath: "./agm/internal/compaction",
		testNames: []string{
			"TestLoadSessionState_RequiresExactStableSessionOwnership",
			"TestLoadSessionState_RejectsMissingResolvedStableID",
		},
	})
}

func agmRunsDurableCompactionAccountingRegressions(ctx context.Context) error {
	return runCompactionRegressionGroups(ctx,
		compactionRegressionGroup{
			packagePath: "./agm/internal/compaction",
			testNames: []string{
				"TestAllocatePromptExclusiveRetriesWithoutTruncatingExistingPrompt",
				"TestAllocatePromptExclusiveConcurrentWritersPreserveEveryPrompt",
				"TestAllocatePromptExclusiveUsesStableSessionIDAsAuditKey",
				"TestAllocatePromptExclusiveRejectsPathTraversal",
				"TestAllocatePromptExclusiveTightensExistingPromptDirectoryPermissions",
				"TestWriteExclusivePromptSyncsParentAfterClosingFile",
				"TestLoadStateForSessionMigratesLegacyNameKeyToStableID",
				"TestLoadStateForSessionRecoversClaimedLegacyMigration",
				"TestLoadStateForSessionRejectsAmbiguousIDLessLegacyState",
				"TestLoadStateForSessionCarriesHistoryAcrossDisplayRename",
				"TestLoadStateForSessionRejectsEmbeddedStableIdentityDrift",
				"TestLoadStateForSessionRejectsLegacyClaimedByAnotherID",
				"TestLoadStateForSessionDoesNotProbeUnsafeLegacyPath",
				"TestSaveStateForSessionRejectsIdentityDrift",
				"TestSaveStateForSessionAtomicReplacementSurvivesConcurrentReads",
				"TestBeginAttemptPersistsPendingBeforeDelivery",
				"TestBeginAttemptDistinguishesPolicyRejectionFromLedgerFailure",
				"TestAttemptOutcomesCountConservativelyForAntiLoop",
				"TestMarkAttemptDefiniteNotSentReleasesAntiLoop",
				"TestMarkAttemptConfirmedIsIdempotent",
				"TestAttemptMarkUncertainUpdatesCountedAttemptSummary",
				"TestMarkAttemptRejectsContradictoryTerminalOutcome",
				"TestBeginAttemptFailsClosedOnCorruptLedger",
				"TestMarkAttemptWriteFailureLeavesPendingFailClosed",
			},
		},
		compactionRegressionGroup{
			packagePath: "./agm/internal/fileutil",
			testNames: []string{
				"TestAtomicWritePublishesFinalModeBeforeDirectorySync",
			},
		},
		compactionRegressionGroup{
			packagePath: "./agm/internal/ops",
			testNames: []string{
				"TestDeliverSessionCompactionPersistsUncertainOutcomeBeforeReturning",
				"TestDeliverSessionCompactionConfirmedWithoutExactReceiptIsUncertain",
				"TestDeliverSessionCompactionPolicyRejectsBeforeSecondDeliveryUnlessForced",
				"TestDeliverSessionCompactionDefiniteNonDeliveryReleasesPolicyBudget",
			},
		},
		compactionRegressionGroup{
			packagePath: "./agm/cmd/agm",
			testNames: []string{
				"TestAllocateDryRunCompactionPromptUsesStableSessionIDAndExclusiveFiles",
				"TestAllocateDryRunCompactionPromptFailsClosedWhenAuditCannotBeSaved",
			},
		},
	)
}

func agmRunsPositiveCompactionVerificationRegressions(ctx context.Context) error {
	return runCompactionRegressionGroups(ctx, compactionRegressionGroup{
		packagePath: "./agm/internal/compaction",
		testNames: []string{
			"TestVerifierRequiresActiveThenStableReady",
			"TestVerifierAcceptsExactPostSubmitProcessingSeed",
			"TestDetectionFromHarnessReadinessPreservesProofClass",
			"TestValidateVerificationReadinessIdentityRejectsTmuxIncarnationDrift",
		},
	}, compactionRegressionGroup{
		packagePath: "./agm/cmd/agm",
		testNames: []string{
			"TestVerificationTargetPreservesAtomicDeliveryIdentity",
		},
	})
}

func agmRunsUnverifiedCompactionRegressions(ctx context.Context) error {
	return runCompactionRegressionGroups(ctx, compactionRegressionGroup{
		packagePath: "./agm/internal/compaction",
		testNames: []string{
			"TestVerifierFailsClosedWithoutPositiveCompletionEvidence",
			"TestOccupiedComposerCannotArmCompactionVerification",
			"TestVerifierRejectsAmbiguousPostDeliveryCycles",
			"TestVerifierReturnsCallerCancellationUnchanged",
			"TestVerifierRejectsProofObservedAfterOwnedDeadline",
			"TestVerifierUsesOnePostObservationTimestampForDeadlineAndProof",
			"TestVerifierReturnsCancellationRaisedDuringFinalObservation",
			"TestVerifierOwnedDeadlineBoundsObservation",
			"TestVerifierClassifiesObserverErrorAfterOwnedDeadlineAsTimeout",
		},
	})
}

func agmRunsCompactionCommandReportingRegressions(ctx context.Context) error {
	return runCompactionRegressionGroups(ctx, compactionRegressionGroup{
		packagePath: "./agm/cmd/agm",
		testNames: []string{
			"TestVerifyCompactionTimeoutIsFailure",
			"TestMonitorCompactionTimeoutIsFailure",
			"TestRunOptionalCompactionVerificationSkipsDisabledVerifier",
			"TestRunOptionalCompactionVerificationPropagatesVerifierFailure",
			"TestVerifyCompactionClaimsCompletionOnlyOnPositiveProof",
			"TestMonitorCompactionClaimsCompletionOnlyOnPositiveProof",
			"TestFinishCompactionDeliverySuppressesSuccessOnUncertainOrIncompleteAccounting",
			"TestFinishCompactionDeliveryUsesDurablePromptReceipt",
			"TestFinishCompactionDeliveryRejectsInconsistentSuccessReceipts",
			"TestSessionCompactMonitorFalseStopsAfterConfirmedDelivery",
			"TestCompactionJSONOutputSilencesCobraErrorRendering",
			"TestSendCompactJSONBoundaryRendersEarlyRawFailureOnce",
			"TestSessionCompactJSONBoundaryRendersEarlyRawFailureOnce",
			"TestCompactionJSONBoundaryRendersBadArityOnce",
			"TestRootCompactionJSONBoundaryRendersPersistentPreRunRawFailureOnce",
			"TestRootCompactionJSONBoundaryRendersUnrenderedPreRunExitErrorOnce",
			"TestRootCompactionJSONNoAgentTTYFailureIsOneProblemWithoutHeader",
			"TestRootCompactionJSONSuppressesWorkspaceDiagnosticsBeforeFailure",
			"TestRootCompactionJSONSuppressesCentralizedMigrationNoticeBeforeFailure",
			"TestRootCompactionJSONSuppressesStaleAndPostRunWarnings",
			"TestDetectWorkspaceSuppressesSuccessfulDebugDiagnosticsAtJSONBoundary",
			"TestValidateRawCompactionInputRejectsControlsBeforeRuntimeWork",
			"TestCompactionCommandFailureCodeIsUniqueAndPublished",
			"TestSendCompactCommandMetadata",
			"TestSessionCompactCommandMetadata",
		},
	})
}

func agmRunsCompactionDeliveryOutcomeErrorRegressions(ctx context.Context) error {
	return runCompactionRegressionGroups(ctx, compactionRegressionGroup{
		packagePath: "./agm/internal/ops",
		testNames: []string{
			"TestDeliverSessionCompactionConfirmedAccountingFailureForbidsRetry",
			"TestStableErrorCodesAreUnique",
			"TestCompactionPolicyErrorPreservesStableIdentity",
			"TestCompactionDeliveryOutcomeErrorsForbidAutomaticRetry",
			"TestErrorCatalogPublishesTheActualProblemType",
		},
	})
}

func runCompactionRegressionGroups(ctx context.Context, groups ...compactionRegressionGroup) error {
	state, err := getCompactionVerificationState(ctx)
	if err != nil {
		return err
	}
	outputs := make([]string, 0, len(groups))
	errs := make([]error, 0, len(groups))
	for _, group := range groups {
		output, runErr := runLocalGuardrailNamedGoTests(ctx, group.packagePath, group.testNames...)
		outputs = append(outputs, output)
		errs = append(errs, runErr)
	}
	state.output = strings.Join(outputs, "\n")
	state.err = errors.Join(errs...)
	return nil
}

func compactionVerificationRegressionsShouldPass(ctx context.Context) error {
	state, err := getCompactionVerificationState(ctx)
	if err != nil {
		return err
	}
	if state.err != nil {
		return fmt.Errorf("compaction verification regressions: %w\n%s", state.err, state.output)
	}
	return nil
}
