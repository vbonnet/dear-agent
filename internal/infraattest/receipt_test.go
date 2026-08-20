package infraattest

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestIssueAndVerifyReceiptWithholdsPrivateEvidence(t *testing.T) {
	authorizationRaw, authorization, evidence, toolchain, request := testAuthorization(t)
	postEvidence := evidence
	postEvidence.stateSnapshot = canonicalStateFixture(t, 42)
	receiptRequest := testReceiptRequest(authorizationRaw, postEvidence, request.HMACKey)
	receiptRaw, err := IssueReceipt(receiptRequest)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(receiptRaw, []byte(privateSentinel)) || bytes.Contains(receiptRaw, []byte("private-lineage")) {
		t.Fatalf("public receipt leaked private evidence: %s", receiptRaw)
	}
	receipt, err := parseReceipt(receiptRaw)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.State.Serial != 42 || receipt.AppliedPlanSHA256 != authorization.SubjectSHA256 {
		t.Fatalf("receipt binding = %+v", receipt)
	}
	if _, err := VerifyReceipt(receiptRaw, ExpectedReceipt{
		AuthorizationClaimsSHA256: receipt.AuthorizationClaimsSHA256,
		AppliedPlanSHA256:         receipt.AppliedPlanSHA256,
		Source:                    receipt.Source,
		Toolchain:                 receipt.Toolchain,
		State:                     receipt.State,
		Nonce:                     receipt.PrivateEvidence.Nonce,
	}); err != nil {
		t.Fatal(err)
	}

	nextRequest, nextPlan, desired, _, _ := testAuthorizationInputs(t)
	nextRequest.State.Serial = 42
	nextRequest.BaselineReceipt = bytes.NewReader(receiptRaw)
	nextRequest.IssuedAt = time.Date(2026, 8, 11, 1, 5, 0, 0, time.UTC)
	nextRequest.NotBefore = nextRequest.IssuedAt
	nextRequest.ExpiresAt = time.Date(2026, 8, 11, 1, 14, 0, 0, time.UTC)
	nextRequest.Now = time.Date(2026, 8, 11, 1, 6, 0, 0, time.UTC)
	nextClaims, err := authorizeEvaluated(
		nextRequest,
		strings.Repeat("4", 64),
		nextPlan,
		desired,
		postEvidence,
		toolchain,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(nextClaims) == 0 {
		t.Fatal("next authorization was empty")
	}
}

func TestReceiptFailsClosedOnIncompleteOrAmbiguousEvidence(t *testing.T) {
	authorizationRaw, _, evidence, _, request := testAuthorization(t)
	tests := []struct {
		name string
		edit func(*ReceiptRequest)
	}{
		{"serial did not advance", func(value *ReceiptRequest) { value.State.Serial = request.State.Serial }},
		{"lineage changed", func(value *ReceiptRequest) { value.State.Lineage = "private-other" }},
		{"provider not visible", func(value *ReceiptRequest) { value.Verification.ProviderVisible = false }},
		{"drift remains", func(value *ReceiptRequest) { value.Verification.NoDrift = false }},
		{"source mismatch", func(value *ReceiptRequest) { value.Verification.SourceParity = false }},
		{"canary failed", func(value *ReceiptRequest) { value.Verification.BehavioralCanary = false }},
		{"authorization expired", func(value *ReceiptRequest) { value.ObservedAt = request.ExpiresAt }},
		{"nonce replay", func(value *ReceiptRequest) { value.Nonce = append([]byte(nil), request.Nonce...) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := testReceiptRequest(authorizationRaw, evidence, request.HMACKey)
			test.edit(&value)
			_, err := IssueReceipt(value)
			assertCode(t, err, CodeReceiptPreconditions)
			if strings.Contains(err.Error(), privateSentinel) {
				t.Fatalf("receipt error leaked private evidence: %v", err)
			}
		})
	}
}

func TestVerifyReceiptRejectsTamperAndReplay(t *testing.T) {
	authorizationRaw, _, evidence, _, request := testAuthorization(t)
	receiptRaw, err := IssueReceipt(testReceiptRequest(authorizationRaw, evidence, request.HMACKey))
	if err != nil {
		t.Fatal(err)
	}
	receipt, _ := parseReceipt(receiptRaw)
	expected := ExpectedReceipt{
		AuthorizationClaimsSHA256: receipt.AuthorizationClaimsSHA256,
		AppliedPlanSHA256:         receipt.AppliedPlanSHA256,
		Source:                    receipt.Source,
		Toolchain:                 receipt.Toolchain,
		State:                     receipt.State,
		Nonce:                     receipt.PrivateEvidence.Nonce,
	}
	tests := []struct {
		name string
		raw  []byte
		edit func(*ExpectedReceipt)
	}{
		{"wrong authorization", receiptRaw, func(value *ExpectedReceipt) { value.AuthorizationClaimsSHA256 = strings.Repeat("f", 64) }},
		{"wrong plan", receiptRaw, func(value *ExpectedReceipt) { value.AppliedPlanSHA256 = strings.Repeat("f", 64) }},
		{"wrong state", receiptRaw, func(value *ExpectedReceipt) { value.State.Serial++ }},
		{"replayed nonce", receiptRaw, func(value *ExpectedReceipt) { value.Nonce = strings.Repeat("A", len(value.Nonce)) }},
		{"noncanonical", append([]byte(" "), receiptRaw...), func(*ExpectedReceipt) {}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := expected
			test.edit(&changed)
			_, err := VerifyReceipt(test.raw, changed)
			assertCode(t, err, CodeReceiptMismatch)
		})
	}

	var document map[string]any
	if err := json.Unmarshal(receiptRaw, &document); err != nil {
		t.Fatal(err)
	}
	document["decision"] = "tampered"
	tampered, _ := json.Marshal(document)
	_, err = VerifyReceipt(tampered, expected)
	assertCode(t, err, CodeReceiptMismatch)
}

// TestVerifyReceiptRejectsZeroObservedAt guards against allCanonicalUTC
// alone accepting the zero time.Time value ("0001-01-01T00:00:00Z"): it is
// in the UTC location with zero nanoseconds, so it passed the shared claims
// validator even though IssueReceipt itself already rejects a zero
// ObservedAt on the write path (codex review on #1257).
func TestVerifyReceiptRejectsZeroObservedAt(t *testing.T) {
	authorizationRaw, _, evidence, _, request := testAuthorization(t)
	receiptRaw, err := IssueReceipt(testReceiptRequest(authorizationRaw, evidence, request.HMACKey))
	if err != nil {
		t.Fatal(err)
	}
	receipt, _ := parseReceipt(receiptRaw)
	expected := ExpectedReceipt{
		AuthorizationClaimsSHA256: receipt.AuthorizationClaimsSHA256,
		AppliedPlanSHA256:         receipt.AppliedPlanSHA256,
		Source:                    receipt.Source,
		Toolchain:                 receipt.Toolchain,
		State:                     receipt.State,
		Nonce:                     receipt.PrivateEvidence.Nonce,
	}

	var document map[string]any
	if err := json.Unmarshal(receiptRaw, &document); err != nil {
		t.Fatal(err)
	}
	document["observed_at"] = "0001-01-01T00:00:00Z"
	tampered, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	_, err = VerifyReceipt(tampered, expected)
	assertCode(t, err, CodeReceiptMismatch)
}

func testAuthorization(t *testing.T) ([]byte, AuthorizationClaims, privateEvidence, ToolchainClaims, AuthorizationRequest) {
	t.Helper()
	request, plan, desired, evidence, toolchain := testAuthorizationInputs(t)
	raw, err := authorizeEvaluated(request, strings.Repeat("5", 64), plan, desired, evidence, toolchain)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := parseAuthorization(raw)
	if err != nil {
		t.Fatal(err)
	}
	return raw, claims, evidence, toolchain, request
}

func testReceiptRequest(authorization []byte, evidence privateEvidence, key []byte) ReceiptRequest {
	evidence.stateSnapshot = []byte(`{"lineage":"private-lineage","private":"DA_PRIVATE_SENTINEL_7c96b2ca","serial":42}`)
	return ReceiptRequest{
		Authorization:     bytes.NewReader(authorization),
		Inventory:         bytes.NewReader(evidence.inventory),
		Backend:           bytes.NewReader(evidence.backend),
		StateSnapshot:     bytes.NewReader(evidence.stateSnapshot),
		MigrationManifest: bytes.NewReader(evidence.migration),
		ProviderSnapshot:  bytes.NewReader(evidence.providerSnapshot),
		State:             PrivateState{Lineage: "private-lineage", Serial: 42},
		Verification: VerificationClaims{
			ProviderVisible: true, NoDrift: true, SourceParity: true, BehavioralCanary: true,
		},
		ObservedAt: time.Date(2026, 8, 11, 1, 3, 0, 0, time.UTC),
		HMACKey:    append([]byte(nil), key...),
		Nonce:      bytes.Repeat([]byte{0x52}, CommitmentNonceBytes),
	}
}

func canonicalStateFixture(t *testing.T, serial uint64) []byte {
	t.Helper()
	raw := []byte(`{"lineage":"private-lineage","private":"DA_PRIVATE_SENTINEL_7c96b2ca","serial":42}`)
	if serial != 42 {
		t.Fatalf("unsupported test serial %d", serial)
	}
	canonical, err := canonicalJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	return canonical
}
