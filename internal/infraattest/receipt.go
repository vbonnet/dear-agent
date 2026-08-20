package infraattest

import (
	"bytes"
	"reflect"
	"time"
)

// IssueReceipt withholds a receipt unless every post-apply observation is
// affirmative and the state advances from the exact authorization lineage.
func IssueReceipt(request ReceiptRequest) ([]byte, error) {
	authorizationRaw, err := readBounded(request.Authorization, MaxClaimsBytes)
	if err != nil {
		return nil, err
	}
	authorizationSHA, authorization, err := authorizationDigest(authorizationRaw)
	if err != nil {
		return nil, err
	}
	if !validReceiptRequest(request, authorization) {
		return nil, reject(CodeReceiptPreconditions)
	}
	if err := validateReceiptLineage(request, authorization); err != nil {
		return nil, err
	}

	evidence, err := loadReceiptEvidence(request)
	if err != nil {
		return nil, err
	}
	if err := validateStateSnapshot(evidence.stateSnapshot, request.State); err != nil {
		return nil, reject(CodeReceiptPreconditions)
	}
	privateClaims, stateClaims, err := receiptCommitments(request.HMACKey, request.Nonce, request.State, evidence)
	if err != nil {
		return nil, err
	}
	receipt := ReceiptClaims{
		Schema:                    ReceiptSchema,
		Decision:                  "provider-reconciled",
		Operation:                 "production-apply",
		AuthorizationClaimsSHA256: authorizationSHA,
		AppliedPlanSHA256:         authorization.SubjectSHA256,
		Source:                    authorization.Source,
		Toolchain:                 authorization.Toolchain,
		PrivateEvidence:           privateClaims,
		State:                     stateClaims,
		Verification:              request.Verification,
		ObservedAt:                request.ObservedAt.Format(time.RFC3339),
	}
	return canonicalStruct(receipt)
}

func validReceiptRequest(request ReceiptRequest, authorization AuthorizationClaims) bool {
	return len(request.HMACKey) >= CommitmentKeyMinBytes && len(request.Nonce) == CommitmentNonceBytes &&
		!request.ObservedAt.IsZero() && allCanonicalUTC(request.ObservedAt) &&
		request.State.Serial > authorization.State.Serial && validatePrivateState(request.State) == nil &&
		validVerification(request.Verification) && validateFreshnessClaims(authorization.Freshness, request.ObservedAt) == nil
}

func validVerification(verification VerificationClaims) bool {
	return verification.ProviderVisible && verification.NoDrift && verification.SourceParity && verification.BehavioralCanary
}

func validateReceiptLineage(request ReceiptRequest, authorization AuthorizationClaims) error {
	authorizationNonce, err := decodeNonce(authorization.PrivateEvidence.Nonce)
	if err != nil || bytes.Equal(authorizationNonce, request.Nonce) {
		return reject(CodeReceiptPreconditions)
	}
	lineageCommitment, err := commitment(
		request.HMACKey,
		authorizationNonce,
		"authorization/lineage",
		[]byte(request.State.Lineage),
	)
	if err != nil || lineageCommitment != authorization.State.LineageHMACSHA256 {
		return reject(CodeReceiptPreconditions)
	}
	return nil
}

func loadReceiptEvidence(request ReceiptRequest) (privateEvidence, error) {
	return loadPrivateEvidence(AuthorizationRequest{
		Inventory:         request.Inventory,
		Backend:           request.Backend,
		StateSnapshot:     request.StateSnapshot,
		MigrationManifest: request.MigrationManifest,
		ProviderSnapshot:  request.ProviderSnapshot,
	})
}

func receiptCommitments(
	key, nonce []byte,
	state PrivateState,
	evidence privateEvidence,
) (ReceiptCommitments, StateClaims, error) {
	nonceEncoded, err := encodeNonce(nonce)
	if err != nil {
		return ReceiptCommitments{}, StateClaims{}, err
	}
	values := make(map[string]string, 6)
	for _, item := range []struct {
		name    string
		payload []byte
	}{
		{"inventory", evidence.inventory},
		{"backend", evidence.backend},
		{"state-snapshot", evidence.stateSnapshot},
		{"migration-surface", evidence.migration},
		{"provider-snapshot", evidence.providerSnapshot},
		{"lineage", []byte(state.Lineage)},
	} {
		value, err := commitment(key, nonce, "receipt/"+item.name, item.payload)
		if err != nil {
			return ReceiptCommitments{}, StateClaims{}, err
		}
		values[item.name] = value
	}
	return ReceiptCommitments{
			Nonce:                      nonceEncoded,
			InventoryHMACSHA256:        values["inventory"],
			BackendHMACSHA256:          values["backend"],
			StateSnapshotHMACSHA256:    values["state-snapshot"],
			MigrationSurfaceHMACSHA256: values["migration-surface"],
			ProviderSnapshotHMACSHA256: values["provider-snapshot"],
		}, StateClaims{
			LineageHMACSHA256: values["lineage"],
			Serial:            state.Serial,
		}, nil
}

func parseReceipt(raw []byte) (ReceiptClaims, error) {
	if len(raw) == 0 || len(raw) > MaxClaimsBytes {
		return ReceiptClaims{}, reject(CodeReceiptMismatch)
	}
	var receipt ReceiptClaims
	if _, err := decodeStrict(raw, &receipt); err != nil {
		return ReceiptClaims{}, reject(CodeReceiptMismatch)
	}
	canonical, err := canonicalStruct(receipt)
	if err != nil || !bytes.Equal(raw, canonical) || validateReceiptClaims(receipt) != nil {
		return ReceiptClaims{}, reject(CodeReceiptMismatch)
	}
	return receipt, nil
}

func validateReceiptClaims(receipt ReceiptClaims) error {
	observedAt, err := time.Parse(time.RFC3339, receipt.ObservedAt)
	if err != nil || observedAt.IsZero() || !validReceiptIdentity(receipt, observedAt) {
		// allCanonicalUTC alone accepts the zero time.Time value
		// ("0001-01-01T00:00:00Z"): it is in the UTC location with zero
		// nanoseconds, so it passes that check even though it carries no
		// meaningful observation time. IssueReceipt already rejects a zero
		// ObservedAt on the write path; this closes the same gap on the
		// shared read path an independently supplied receipt goes through
		// (codex review on #1257).
		return reject(CodeReceiptMismatch)
	}
	if _, err := decodeNonce(receipt.PrivateEvidence.Nonce); err != nil {
		return reject(CodeReceiptMismatch)
	}
	for _, value := range []string{
		receipt.PrivateEvidence.InventoryHMACSHA256,
		receipt.PrivateEvidence.BackendHMACSHA256,
		receipt.PrivateEvidence.StateSnapshotHMACSHA256,
		receipt.PrivateEvidence.MigrationSurfaceHMACSHA256,
		receipt.PrivateEvidence.ProviderSnapshotHMACSHA256,
	} {
		if !validSHA256(value) {
			return reject(CodeReceiptMismatch)
		}
	}
	return nil
}

func validReceiptIdentity(receipt ReceiptClaims, observedAt time.Time) bool {
	return receipt.Schema == ReceiptSchema && receipt.Decision == "provider-reconciled" &&
		receipt.Operation == "production-apply" && validSHA256(receipt.AuthorizationClaimsSHA256) &&
		validSHA256(receipt.AppliedPlanSHA256) && validateSource(receipt.Source) == nil &&
		validateToolchainClaims(receipt.Toolchain) == nil && validSHA256(receipt.State.LineageHMACSHA256) &&
		allCanonicalUTC(observedAt) && validVerification(receipt.Verification)
}

// VerifyReceipt validates canonical receipt claims against the current public
// bindings after transport-level signature verification.
func VerifyReceipt(raw []byte, expected ExpectedReceipt) (ReceiptClaims, error) {
	receipt, err := parseReceipt(raw)
	if err != nil {
		return ReceiptClaims{}, err
	}
	if receipt.AuthorizationClaimsSHA256 != expected.AuthorizationClaimsSHA256 ||
		receipt.AppliedPlanSHA256 != expected.AppliedPlanSHA256 ||
		!reflect.DeepEqual(receipt.Source, expected.Source) ||
		!reflect.DeepEqual(receipt.Toolchain, expected.Toolchain) ||
		!reflect.DeepEqual(receipt.State, expected.State) ||
		receipt.PrivateEvidence.Nonce != expected.Nonce {
		return ReceiptClaims{}, reject(CodeReceiptMismatch)
	}
	return receipt, nil
}
