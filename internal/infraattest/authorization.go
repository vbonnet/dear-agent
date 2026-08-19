package infraattest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"
)

var repositoryPattern = regexp.MustCompile(`^[a-z0-9_.-]+/[a-z0-9_.-]+$`)

type privateEvidence struct {
	inventory        []byte
	backend          []byte
	stateSnapshot    []byte
	migration        []byte
	providerSnapshot []byte
}

// Authorize consumes private plan and state evidence and returns canonical,
// public-safe authorization claims. It never returns a partial authorization.
func Authorize(request AuthorizationRequest) ([]byte, error) {
	toolchain, err := evaluateToolchain(request)
	if err != nil {
		return nil, err
	}
	encryptedPlan, err := readEncryptedPlan(request.EncryptedPlan)
	if err != nil {
		return nil, err
	}
	subjectDigest := digest(encryptedPlan)
	planRaw, err := readBounded(request.PlanJSON, MaxPlanJSONBytes)
	if err != nil {
		return nil, err
	}
	desiredRaw, err := readBounded(request.DesiredRulesetProjection, MaxEvidenceBytes)
	if err != nil {
		return nil, err
	}
	evidence, err := loadPrivateEvidence(request)
	if err != nil {
		return nil, err
	}
	if err := validateStateSnapshot(evidence.stateSnapshot, request.State); err != nil {
		return nil, err
	}
	return authorizeEvaluated(request, subjectDigest, planRaw, desiredRaw, evidence, toolchain)
}

func authorizeEvaluated(
	request AuthorizationRequest,
	subjectDigest string,
	planRaw, desiredRaw []byte,
	evidence privateEvidence,
	toolchain ToolchainClaims,
) ([]byte, error) {
	if err := validateSource(request.Source); err != nil {
		return nil, err
	}
	if err := validatePlanProfile(request.PlanProfile); err != nil {
		return nil, err
	}
	if !request.InventoryComplete {
		return nil, reject(CodeInventoryIncomplete)
	}
	if err := validatePrivateState(request.State); err != nil {
		return nil, err
	}
	if len(request.HMACKey) < CommitmentKeyMinBytes || len(request.Nonce) != CommitmentNonceBytes ||
		!validSHA256(subjectDigest) || validateToolchainClaims(toolchain) != nil {
		return nil, reject(CodeInvalidInput)
	}
	plan, err := evaluatePlan(planRaw, desiredRaw, request.Ruleset)
	if err != nil {
		return nil, err
	}
	freshness, err := evaluateFreshness(request, plan.PlanGeneratedAt)
	if err != nil {
		return nil, err
	}
	if err := validateBaseline(request, evidence, toolchain); err != nil {
		return nil, err
	}

	commitments, stateClaims, err := authorizationCommitments(
		request.HMACKey,
		request.Nonce,
		request.State,
		evidence,
		plan.Projection,
	)
	if err != nil {
		return nil, err
	}
	claims := AuthorizationClaims{
		Schema:          AuthorizationSchema,
		Decision:        "authorize-routine-apply",
		Operation:       "production-apply",
		SubjectSHA256:   subjectDigest,
		Source:          request.Source,
		Toolchain:       toolchain,
		PrivateEvidence: commitments,
		State:           stateClaims,
		Classification: ClassificationClaims{
			Kind:                       plan.Kind,
			HumanAuthorizationRequired: false,
		},
		PlanProfile: request.PlanProfile,
		Freshness:   freshness,
	}
	return canonicalStruct(claims)
}

func loadPrivateEvidence(request AuthorizationRequest) (privateEvidence, error) {
	inventoryRaw, err := readBounded(request.Inventory, MaxEvidenceBytes)
	if err != nil {
		return privateEvidence{}, err
	}
	inventory, err := canonicalJSON(inventoryRaw)
	if err != nil {
		return privateEvidence{}, err
	}
	if err := validateInventory(inventory); err != nil {
		return privateEvidence{}, err
	}
	backendRaw, err := readBounded(request.Backend, MaxEvidenceBytes)
	if err != nil {
		return privateEvidence{}, err
	}
	backend, err := canonicalOpaque(backendRaw)
	if err != nil {
		return privateEvidence{}, err
	}
	stateRaw, err := readBounded(request.StateSnapshot, MaxEvidenceBytes)
	if err != nil {
		return privateEvidence{}, err
	}
	state, err := canonicalJSON(stateRaw)
	if err != nil {
		return privateEvidence{}, err
	}
	migrationRaw, err := readBounded(request.MigrationManifest, MaxEvidenceBytes)
	if err != nil {
		return privateEvidence{}, err
	}
	migration, err := evaluateMigrationManifest(migrationRaw)
	if err != nil {
		return privateEvidence{}, err
	}
	providerRaw, err := readBounded(request.ProviderSnapshot, MaxEvidenceBytes)
	if err != nil {
		return privateEvidence{}, err
	}
	provider, err := canonicalJSON(providerRaw)
	if err != nil {
		return privateEvidence{}, err
	}
	return privateEvidence{
		inventory:        inventory,
		backend:          backend,
		stateSnapshot:    state,
		migration:        migration,
		providerSnapshot: provider,
	}, nil
}

func validateInventory(raw []byte) error {
	object, err := objectValue(raw)
	if err != nil {
		return reject(CodeInventoryIncomplete)
	}
	complete, ok := object["complete"].(bool)
	if !ok || !complete {
		return reject(CodeInventoryIncomplete)
	}
	return nil
}

func authorizationCommitments(
	key, nonce []byte,
	state PrivateState,
	evidence privateEvidence,
	projection []byte,
) (AuthorizationCommitments, StateClaims, error) {
	nonceEncoded, err := encodeNonce(nonce)
	if err != nil {
		return AuthorizationCommitments{}, StateClaims{}, err
	}
	values := make(map[string]string, 7)
	for _, item := range []struct {
		name    string
		payload []byte
	}{
		{"inventory", evidence.inventory},
		{"backend", evidence.backend},
		{"state-snapshot", evidence.stateSnapshot},
		{"migration-surface", evidence.migration},
		{"provider-snapshot", evidence.providerSnapshot},
		{"change-projection", projection},
		{"lineage", []byte(state.Lineage)},
	} {
		value, err := commitment(key, nonce, "authorization/"+item.name, item.payload)
		if err != nil {
			return AuthorizationCommitments{}, StateClaims{}, err
		}
		values[item.name] = value
	}
	return AuthorizationCommitments{
			Nonce:                      nonceEncoded,
			InventoryHMACSHA256:        values["inventory"],
			BackendHMACSHA256:          values["backend"],
			StateSnapshotHMACSHA256:    values["state-snapshot"],
			MigrationSurfaceHMACSHA256: values["migration-surface"],
			ProviderSnapshotHMACSHA256: values["provider-snapshot"],
			ChangeProjectionHMACSHA256: values["change-projection"],
		}, StateClaims{
			LineageHMACSHA256: values["lineage"],
			Serial:            state.Serial,
		}, nil
}

func validateBaseline(request AuthorizationRequest, evidence privateEvidence, toolchain ToolchainClaims) error {
	raw, err := readBounded(request.BaselineReceipt, MaxClaimsBytes)
	if err != nil {
		return reject(CodeBaselineMissing)
	}
	receipt, err := parseReceipt(raw)
	if err != nil {
		return reject(CodeBaselineMismatch)
	}
	if !reflect.DeepEqual(receipt.Toolchain, toolchain) || receipt.State.Serial != request.State.Serial ||
		!receipt.Verification.ProviderVisible || !receipt.Verification.NoDrift ||
		!receipt.Verification.SourceParity || !receipt.Verification.BehavioralCanary {
		return reject(CodeBaselineMismatch)
	}
	nonce, err := decodeNonce(receipt.PrivateEvidence.Nonce)
	if err != nil {
		return reject(CodeBaselineMismatch)
	}
	want := map[string]string{
		"inventory":         receipt.PrivateEvidence.InventoryHMACSHA256,
		"backend":           receipt.PrivateEvidence.BackendHMACSHA256,
		"state-snapshot":    receipt.PrivateEvidence.StateSnapshotHMACSHA256,
		"migration-surface": receipt.PrivateEvidence.MigrationSurfaceHMACSHA256,
		"provider-snapshot": receipt.PrivateEvidence.ProviderSnapshotHMACSHA256,
		"lineage":           receipt.State.LineageHMACSHA256,
	}
	for _, item := range []struct {
		name    string
		payload []byte
	}{
		{"inventory", evidence.inventory},
		{"backend", evidence.backend},
		{"state-snapshot", evidence.stateSnapshot},
		{"migration-surface", evidence.migration},
		{"provider-snapshot", evidence.providerSnapshot},
		{"lineage", []byte(request.State.Lineage)},
	} {
		actual, err := commitment(request.HMACKey, nonce, "receipt/"+item.name, item.payload)
		if err != nil || actual != want[item.name] {
			return reject(CodeBaselineMismatch)
		}
	}
	return nil
}

func evaluateFreshness(request AuthorizationRequest, planGeneratedAt time.Time) (FreshnessClaims, error) {
	times := []time.Time{request.IssuedAt, request.NotBefore, request.ExpiresAt, planGeneratedAt}
	for _, value := range times {
		if value.IsZero() || value.Location() != time.UTC || value.Nanosecond() != 0 {
			return FreshnessClaims{}, reject(CodeFreshness)
		}
	}
	now := request.Now
	if request.Now.IsZero() || !allCanonicalUTC(request.Now) || planGeneratedAt.After(request.IssuedAt) ||
		request.IssuedAt.Sub(planGeneratedAt) > MaxCommitmentTTL ||
		request.NotBefore.Before(request.IssuedAt) || request.NotBefore.After(now) ||
		request.IssuedAt.After(now) || !request.ExpiresAt.After(now) ||
		!request.ExpiresAt.After(request.NotBefore) || request.ExpiresAt.Sub(request.IssuedAt) > MaxCommitmentTTL {
		return FreshnessClaims{}, reject(CodeFreshness)
	}
	return FreshnessClaims{
		PlanGeneratedAt: planGeneratedAt.Format(time.RFC3339),
		IssuedAt:        request.IssuedAt.Format(time.RFC3339),
		NotBefore:       request.NotBefore.Format(time.RFC3339),
		ExpiresAt:       request.ExpiresAt.Format(time.RFC3339),
	}, nil
}

func validateSource(source SourceClaims) error {
	if !repositoryPattern.MatchString(source.Repository) || strings.ToLower(source.Repository) != source.Repository ||
		source.Ref != "refs/heads/main" || !validGitOID(source.CommitSHA) || !validGitOID(source.TreeSHA) ||
		!validGitOID(source.CanonicalRulesetBlobSHA) {
		return reject(CodeInvalidInput)
	}
	return nil
}

func validatePrivateState(state PrivateState) error {
	if strings.TrimSpace(state.Lineage) == "" || len(state.Lineage) > 256 {
		return reject(CodeInvalidInput)
	}
	return nil
}

func validateStateSnapshot(raw []byte, state PrivateState) error {
	object, err := objectValue(raw)
	if err != nil {
		return reject(CodeInvalidInput)
	}
	lineage, lineageOK := object["lineage"].(string)
	serial, serialOK := object["serial"].(json.Number)
	if !lineageOK || !serialOK || lineage != state.Lineage || serial.String() != fmt.Sprintf("%d", state.Serial) {
		return reject(CodeInvalidInput)
	}
	return nil
}

func validatePlanProfile(profile PlanProfile) error {
	if profile.Mode != "normal" || !profile.Refresh || profile.Targets == nil || profile.Excludes == nil ||
		profile.Replaces == nil || len(profile.Targets) != 0 || len(profile.Excludes) != 0 || len(profile.Replaces) != 0 {
		return reject(CodePlanProfile)
	}
	return nil
}

func parseAuthorization(raw []byte) (AuthorizationClaims, error) {
	if len(raw) == 0 || len(raw) > MaxClaimsBytes {
		return AuthorizationClaims{}, reject(CodeAuthorizationMismatch)
	}
	var claims AuthorizationClaims
	if _, err := decodeStrict(raw, &claims); err != nil {
		return AuthorizationClaims{}, reject(CodeAuthorizationMismatch)
	}
	canonical, err := canonicalStruct(claims)
	if err != nil || !bytes.Equal(raw, canonical) || validateAuthorizationClaims(claims) != nil {
		return AuthorizationClaims{}, reject(CodeAuthorizationMismatch)
	}
	return claims, nil
}

func validateAuthorizationClaims(claims AuthorizationClaims) error {
	if claims.Schema != AuthorizationSchema || claims.Decision != "authorize-routine-apply" ||
		claims.Operation != "production-apply" || !validSHA256(claims.SubjectSHA256) ||
		validateSource(claims.Source) != nil || validateToolchainClaims(claims.Toolchain) != nil ||
		validatePlanProfile(claims.PlanProfile) != nil || claims.Classification.HumanAuthorizationRequired ||
		(claims.Classification.Kind != "no-op" && claims.Classification.Kind != "existing-ruleset-in-place-restoration") ||
		!validSHA256(claims.State.LineageHMACSHA256) {
		return reject(CodeAuthorizationMismatch)
	}
	if _, err := decodeNonce(claims.PrivateEvidence.Nonce); err != nil {
		return reject(CodeAuthorizationMismatch)
	}
	for _, value := range []string{
		claims.PrivateEvidence.InventoryHMACSHA256,
		claims.PrivateEvidence.BackendHMACSHA256,
		claims.PrivateEvidence.StateSnapshotHMACSHA256,
		claims.PrivateEvidence.MigrationSurfaceHMACSHA256,
		claims.PrivateEvidence.ProviderSnapshotHMACSHA256,
		claims.PrivateEvidence.ChangeProjectionHMACSHA256,
	} {
		if !validSHA256(value) {
			return reject(CodeAuthorizationMismatch)
		}
	}
	return validateFreshnessClaims(claims.Freshness, time.Time{})
}

func validateToolchainClaims(claims ToolchainClaims) error {
	if !validToolchainClaimShape(claims) {
		return reject(CodeAuthorizationMismatch)
	}
	lock, supported := officialPlatformLocks[claims.Platform]
	provider := claims.Providers[0]
	if !supported || claims.OpenTofuArchiveSHA256 != lock.OpenTofuArchiveSHA256 ||
		claims.OpenTofuBinarySHA256 != lock.OpenTofuBinarySHA256 ||
		provider.ArchiveSHA256 != lock.ProviderArchiveSHA256 || provider.BinarySHA256 != lock.ProviderBinarySHA256 {
		return reject(CodeAuthorizationMismatch)
	}
	return nil
}

func validToolchainClaimShape(claims ToolchainClaims) bool {
	return claims.OpenTofuVersion == OpenTofuVersion && claims.OpenTofuTagCommit == OpenTofuTagCommit &&
		validSHA256(claims.OpenTofuArchiveSHA256) && validSHA256(claims.OpenTofuBinarySHA256) &&
		claims.ToolchainManifestSHA256 == ToolchainManifestSHA256 &&
		claims.DependencyLockfileSHA256 == DependencyLockSHA256 &&
		len(claims.Providers) == 1 && claims.Providers[0].Address == ProviderAddress &&
		claims.Providers[0].Version == ProviderVersion && claims.Providers[0].TagCommit == ProviderTagCommit &&
		validSHA256(claims.Providers[0].ArchiveSHA256) && validSHA256(claims.Providers[0].BinarySHA256)
}

func validateFreshnessClaims(claims FreshnessClaims, now time.Time) error {
	parsed, err := parseFreshnessClaims(claims)
	if err != nil || !validFreshnessOrder(parsed) {
		return reject(CodeFreshness)
	}
	if !now.IsZero() && (parsed.issued.After(now) || parsed.notBefore.After(now) || !parsed.expires.After(now)) {
		return reject(CodeFreshness)
	}
	return nil
}

type parsedFreshness struct {
	generated time.Time
	issued    time.Time
	notBefore time.Time
	expires   time.Time
}

func parseFreshnessClaims(claims FreshnessClaims) (parsedFreshness, error) {
	generated, errGenerated := time.Parse(time.RFC3339, claims.PlanGeneratedAt)
	issued, errIssued := time.Parse(time.RFC3339, claims.IssuedAt)
	notBefore, errNotBefore := time.Parse(time.RFC3339, claims.NotBefore)
	expires, errExpires := time.Parse(time.RFC3339, claims.ExpiresAt)
	if errGenerated != nil || errIssued != nil || errNotBefore != nil || errExpires != nil ||
		!allCanonicalUTC(generated, issued, notBefore, expires) {
		return parsedFreshness{}, reject(CodeFreshness)
	}
	return parsedFreshness{generated: generated, issued: issued, notBefore: notBefore, expires: expires}, nil
}

func validFreshnessOrder(value parsedFreshness) bool {
	return !value.generated.After(value.issued) && value.issued.Sub(value.generated) <= MaxCommitmentTTL &&
		!value.notBefore.Before(value.issued) && value.expires.After(value.notBefore) &&
		value.expires.Sub(value.issued) <= MaxCommitmentTTL
}

func allCanonicalUTC(values ...time.Time) bool {
	for _, value := range values {
		if value.Location() != time.UTC || value.Nanosecond() != 0 {
			return false
		}
	}
	return true
}

// VerifyAuthorization validates canonical claims against the caller's current
// public bindings. Signature/OIDC verification remains the transport's job.
func VerifyAuthorization(raw []byte, expected ExpectedAuthorization) (AuthorizationClaims, error) {
	claims, err := parseAuthorization(raw)
	if err != nil {
		return AuthorizationClaims{}, err
	}
	if expected.Now.IsZero() || !allCanonicalUTC(expected.Now) ||
		validateFreshnessClaims(claims.Freshness, expected.Now) != nil ||
		claims.SubjectSHA256 != expected.SubjectSHA256 || !reflect.DeepEqual(claims.Source, expected.Source) ||
		!reflect.DeepEqual(claims.Toolchain, expected.Toolchain) || !reflect.DeepEqual(claims.State, expected.State) ||
		claims.PrivateEvidence.Nonce != expected.Nonce {
		return AuthorizationClaims{}, reject(CodeAuthorizationMismatch)
	}
	return claims, nil
}

func authorizationDigest(raw []byte) (string, AuthorizationClaims, error) {
	claims, err := parseAuthorization(raw)
	if err != nil {
		return "", AuthorizationClaims{}, err
	}
	return digest(raw), claims, nil
}
