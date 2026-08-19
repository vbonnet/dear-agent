package infraattest

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

const privateSentinel = "DA_PRIVATE_SENTINEL_7c96b2ca"

func TestAuthorizeRoutineRulesetRestorationIsCanonicalAndPrivate(t *testing.T) {
	request, plan, desired, evidence, toolchain := testAuthorizationInputs(t)
	claimsRaw, err := authorizeEvaluated(request, strings.Repeat("9", 64), plan, desired, evidence, toolchain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(claimsRaw, []byte(privateSentinel)) || bytes.Contains(claimsRaw, []byte("private-lineage")) {
		t.Fatalf("public claims leaked private evidence: %s", claimsRaw)
	}
	claims, err := parseAuthorization(claimsRaw)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Classification.Kind != "existing-ruleset-in-place-restoration" ||
		claims.Classification.HumanAuthorizationRequired {
		t.Fatalf("classification = %+v", claims.Classification)
	}
	request.BaselineReceipt = bytes.NewReader(testBaselineReceipt(t, request.HMACKey, evidence, toolchain, request.State))
	second, err := authorizeEvaluated(request, strings.Repeat("9", 64), plan, desired, evidence, toolchain)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(claimsRaw, second) {
		t.Fatal("same inputs did not produce deterministic canonical claims")
	}
	if _, err := VerifyAuthorization(claimsRaw, ExpectedAuthorization{
		SubjectSHA256: claims.SubjectSHA256,
		Source:        claims.Source,
		Toolchain:     claims.Toolchain,
		State:         claims.State,
		Nonce:         claims.PrivateEvidence.Nonce,
		Now:           request.Now,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorizeNoOp(t *testing.T) {
	request, planRaw, desired, evidence, toolchain := testAuthorizationInputs(t)
	plan := decodePlanForTest(t, planRaw)
	change := &plan.ResourceChanges[0]
	change.Change.Actions = []string{"no-op"}
	change.Change.Before = append(json.RawMessage(nil), change.Change.After...)
	plan.ResourceDrift = nil
	claimsRaw, err := authorizeEvaluated(request, strings.Repeat("8", 64), encodePlanForTest(t, plan), desired, evidence, toolchain)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := parseAuthorization(claimsRaw)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Classification.Kind != "no-op" {
		t.Fatalf("kind = %q", claims.Classification.Kind)
	}
}

func TestPlanClassifierFailsClosed(t *testing.T) {
	request, planRaw, desired, evidence, toolchain := testAuthorizationInputs(t)
	tests := []struct {
		name string
		code Code
		edit func(*rawPlan)
	}{
		{"create", CodePlanCreate, func(plan *rawPlan) { plan.ResourceChanges[0].Change.Actions = []string{"create"} }},
		{"delete", CodePlanDelete, func(plan *rawPlan) { plan.ResourceChanges[0].Change.Actions = []string{"delete"} }},
		{"forget", CodePlanDelete, func(plan *rawPlan) { plan.ResourceChanges[0].Change.Actions = []string{"forget"} }},
		{"replace delete first", CodePlanReplace, func(plan *rawPlan) { plan.ResourceChanges[0].Change.Actions = []string{"delete", "create"} }},
		{"replace create first", CodePlanReplace, func(plan *rawPlan) { plan.ResourceChanges[0].Change.Actions = []string{"create", "delete"} }},
		{"read", CodePlanRead, func(plan *rawPlan) { plan.ResourceChanges[0].Change.Actions = []string{"read"} }},
		{"unknown action", CodePlanAmbiguous, func(plan *rawPlan) { plan.ResourceChanges[0].Change.Actions = []string{"future"} }},
		{"move", CodePlanMove, func(plan *rawPlan) { plan.ResourceChanges[0].PreviousAddress = "private.previous" }},
		{"deposed", CodePlanDeposed, func(plan *rawPlan) { plan.ResourceChanges[0].Deposed = "private" }},
		{"import", CodePlanImport, func(plan *rawPlan) { plan.ResourceChanges[0].Change.Importing = json.RawMessage(`{"id":"private"}`) }},
		{"generated import", CodePlanImport, func(plan *rawPlan) { plan.ResourceChanges[0].Change.GeneratedConfig = "private" }},
		{"replace paths", CodePlanReplace, func(plan *rawPlan) {
			plan.ResourceChanges[0].Change.ReplacePaths = []json.RawMessage{json.RawMessage(`["name"]`)}
		}},
		{"unknown value", CodePlanUnknown, func(plan *rawPlan) { plan.ResourceChanges[0].Change.AfterUnknown = json.RawMessage(`{"id":true}`) }},
		{"sensitive before", CodePlanSensitive, func(plan *rawPlan) { plan.ResourceChanges[0].Change.BeforeSensitive = json.RawMessage(`{"name":true}`) }},
		{"sensitive after", CodePlanSensitive, func(plan *rawPlan) { plan.ResourceChanges[0].Change.AfterSensitive = json.RawMessage(`true`) }},
		{"action reason", CodePlanAmbiguous, func(plan *rawPlan) { plan.ResourceChanges[0].ActionReason = "private" }},
		{"other resource", CodePlanResourceType, func(plan *rawPlan) { plan.ResourceChanges[0].Type = "github_repository" }},
		{"other provider", CodePlanResourceType, func(plan *rawPlan) { plan.ResourceChanges[0].ProviderName = "registry.opentofu.org/other/private" }},
		{"output change", CodePlanOutputs, func(plan *rawPlan) { plan.OutputChanges = map[string]json.RawMessage{"private": json.RawMessage(`{}`)} }},
		{"failed check", CodePlanChecks, func(plan *rawPlan) { plan.Checks = []rawCheck{{Status: "fail"}} }},
		{"unknown check", CodePlanChecks, func(plan *rawPlan) { plan.Checks = []rawCheck{{Status: "unknown"}} }},
		{"deferred", CodePlanAmbiguous, func(plan *rawPlan) { plan.DeferredChanges = []json.RawMessage{json.RawMessage(`{}`)} }},
		{"missing complete marker", CodePlanAmbiguous, func(plan *rawPlan) { plan.Complete = nil }},
		{"update not applyable", CodePlanAmbiguous, func(plan *rawPlan) {
			applyable := false
			plan.Applyable = &applyable
		}},
		{"errored", CodePlanErrored, func(plan *rawPlan) { plan.Errored = true }},
		{"major version", CodeUnsupportedPlanFormat, func(plan *rawPlan) { plan.FormatVersion = "2.0" }},
		{"wrong tofu", CodeUnsupportedPlanFormat, func(plan *rawPlan) { plan.TerraformVersion = "1.12.4" }},
		{"wrong id", CodeRulesetBinding, func(plan *rawPlan) { setPlanField(t, plan, "id", "999") }},
		{"wrong ruleset id", CodeRulesetBinding, func(plan *rawPlan) { setPlanField(t, plan, "ruleset_id", json.Number("999")) }},
		{"string ruleset id", CodeRulesetBinding, func(plan *rawPlan) { setPlanField(t, plan, "ruleset_id", "18061003") }},
		{"wrong repository", CodeRulesetBinding, func(plan *rawPlan) { setPlanField(t, plan, "repository", "private-other") }},
		{"unmatched drift", CodePlanAmbiguous, func(plan *rawPlan) {
			drift := plan.ResourceChanges[0]
			drift.Address = "private.other"
			plan.ResourceDrift = []rawResourceChange{drift}
		}},
		{"missing restoration drift", CodePlanAmbiguous, func(plan *rawPlan) { plan.ResourceDrift = nil }},
		{"duplicate restoration drift", CodePlanAmbiguous, func(plan *rawPlan) {
			plan.ResourceDrift = append(plan.ResourceDrift, plan.ResourceDrift[0])
		}},
		{"non-inverse restoration drift", CodePlanAmbiguous, func(plan *rawPlan) {
			plan.ResourceDrift[0].Change.Before = append(json.RawMessage(nil), plan.ResourceDrift[0].Change.After...)
		}},
		{"multiple updates", CodePlanAmbiguous, func(plan *rawPlan) { plan.ResourceChanges = append(plan.ResourceChanges, plan.ResourceChanges[0]) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			plan := decodePlanForTest(t, planRaw)
			test.edit(&plan)
			_, err := authorizeEvaluated(request, strings.Repeat("7", 64), encodePlanForTest(t, plan), desired, evidence, toolchain)
			assertCode(t, err, test.code)
			if err != nil && strings.Contains(err.Error(), privateSentinel) {
				t.Fatalf("error leaked private sentinel: %v", err)
			}
		})
	}
}

func TestPlanAndEvidenceParserRejectMalformedOversizeAndStructuralInputs(t *testing.T) {
	request, plan, desired, evidence, toolchain := testAuthorizationInputs(t)
	_, err := authorizeEvaluated(request, strings.Repeat("7", 64), append(plan, []byte(`{"duplicate":true}`)...), desired, evidence, toolchain)
	assertCode(t, err, CodeMalformedPlan)

	duplicate := bytes.Replace(plan, []byte(`"format_version": "1.2"`), []byte(`"format_version": "1.2", "format_version": "1.2"`), 1)
	_, err = authorizeEvaluated(request, strings.Repeat("7", 64), duplicate, desired, evidence, toolchain)
	assertCode(t, err, CodeMalformedPlan)

	unknown := bytes.Replace(
		plan,
		[]byte(`"errored": false`),
		[]byte(`"future_private_field": "`+privateSentinel+`", "errored": false`),
		1,
	)
	_, err = authorizeEvaluated(request, strings.Repeat("7", 64), unknown, desired, evidence, toolchain)
	assertCode(t, err, CodeMalformedPlan)

	missingPriorState := decodePlanForTest(t, plan)
	missingPriorState.PriorState = nil
	_, err = authorizeEvaluated(
		request,
		strings.Repeat("7", 64),
		encodePlanForTest(t, missingPriorState),
		desired,
		evidence,
		toolchain,
	)
	assertCode(t, err, CodeMalformedPlan)

	migration := readFixture(t, "migration-surface.json")
	for _, edit := range []func(map[string]any){
		func(value map[string]any) { value["moved_blocks"] = []any{"private"} },
		func(value map[string]any) { value["removed_blocks"] = []any{"private"} },
		func(value map[string]any) { value["import_blocks"] = []any{"private"} },
		func(value map[string]any) { value["state_encryption"] = "enforced" },
		func(value map[string]any) { value["plan_encryption"] = "disabled" },
		func(value map[string]any) { value["workspace"] = "private" },
		func(value map[string]any) { value["providers"] = []any{} },
	} {
		var value map[string]any
		if err := json.Unmarshal(migration, &value); err != nil {
			t.Fatal(err)
		}
		edit(value)
		raw, _ := json.Marshal(value)
		_, err := evaluateMigrationManifest(raw)
		assertCode(t, err, CodeMigrationSurface)
	}

	tooLarge := bytes.NewReader(make([]byte, MaxPlanJSONBytes+1))
	_, err = readBounded(tooLarge, MaxPlanJSONBytes)
	assertCode(t, err, CodeInputTooLarge)
}

func TestCanonicalJSONRejectsDepthNodeStringAndUTF8Bounds(t *testing.T) {
	deep := []byte(strings.Repeat("[", MaxJSONDepth+2) + "0" + strings.Repeat("]", MaxJSONDepth+2))
	if _, err := canonicalJSON(deep); err == nil {
		t.Fatal("over-depth JSON was accepted")
	}

	var nodes strings.Builder
	nodes.Grow(MaxJSONNodes * 2)
	nodes.WriteByte('[')
	for index := range MaxJSONNodes {
		if index != 0 {
			nodes.WriteByte(',')
		}
		nodes.WriteByte('0')
	}
	nodes.WriteByte(']')
	if _, err := canonicalJSON([]byte(nodes.String())); err == nil {
		t.Fatal("over-node JSON was accepted")
	}

	longString := []byte(`"` + strings.Repeat("x", MaxJSONStringBytes+1) + `"`)
	if _, err := canonicalJSON(longString); err == nil {
		t.Fatal("over-length JSON string was accepted")
	}
	if _, err := canonicalJSON([]byte{'"', 0xff, '"'}); err == nil {
		t.Fatal("invalid UTF-8 JSON was accepted")
	}
}

// numbersJSON builds an array of count copies of literal, so a test can state
// its input by the expansion it asks for rather than by a byte count.
func numbersJSON(literal string, count int) []byte {
	var array strings.Builder
	array.Grow(count * (len(literal) + 1))
	array.WriteByte('[')
	for index := range count {
		if index != 0 {
			array.WriteByte(',')
		}
		array.WriteString(literal)
	}
	array.WriteByte(']')
	return []byte(array.String())
}

func TestCanonicalJSONRejectsCompactNumberExpansion(t *testing.T) {
	// Each literal below is a handful of input bytes whose exponent-free form
	// runs past MaxJSONNumberBytes, so accepting it would allocate far more
	// than the document was ever allowed to occupy.
	for _, literal := range []string{
		"1e4000000",
		"-1e4000000",
		"1e-4000000",
		"1e5000",
		"1.5e5000",
		"1e-5000",
	} {
		raw := []byte(`{"n":` + literal + `}`)
		if len(raw) > 64 {
			t.Fatalf("fixture %s is not compact", literal)
		}
		_, err := canonicalJSON(raw)
		assertCode(t, err, CodeMalformedJSON)
	}

	// Positive control: the same shape just inside the per-number bound is
	// accepted, so the expansion width is what the rejections turn on.
	got, err := canonicalJSON([]byte(`{"n":1e4000}`))
	if err != nil {
		t.Fatal(err)
	}
	if want := len(`{"n":}`) + 1 + 4000; len(got) != want {
		t.Fatalf("canonical length = %d, want %d", len(got), want)
	}
}

func TestCanonicalJSONRejectsAggregateNumberExpansion(t *testing.T) {
	// Every value here is individually legal: 1e4000 normalises to 4001 bytes,
	// inside MaxJSONNumberBytes. Only their sum breaches the document's
	// aggregate allowance, which is what a per-number bound cannot see.
	const literal = "1e4000"
	const width = 4001
	overBudget := numbersJSON(literal, MaxEvidenceBytes/width+1)
	if len(overBudget) > MaxEvidenceBytes {
		t.Fatalf("over-budget fixture is %d raw bytes, past the raw bound", len(overBudget))
	}
	assertCode(t, canonicalJSONError(t, overBudget), CodeMalformedJSON)

	// Positive control: the same literal repeated just under the allowance is
	// accepted, so aggregation is what the rejection turns on rather than the
	// literal, the node count, or the raw size.
	underBudget := numbersJSON(literal, MaxEvidenceBytes/width-1)
	if _, err := canonicalJSON(underBudget); err != nil {
		t.Fatalf("under-budget document was rejected: %v", err)
	}
}

func canonicalJSONError(t *testing.T, raw []byte) error {
	t.Helper()
	_, err := canonicalJSON(raw)
	return err
}

func TestCanonicalJSONNormalizesOrdinaryNumbersUnchanged(t *testing.T) {
	// The bounds above must not disturb the numbers real plan and evidence JSON
	// actually carries, including the extremes of IEEE-754 double range.
	for _, testCase := range []struct{ raw, want string }{
		{`0`, `0`},
		{`-0`, `0`},
		{`1`, `1`},
		{`-1`, `-1`},
		{`42`, `42`},
		{`1234567890123456789`, `1234567890123456789`},
		{`0.5`, `0.5`},
		{`-2.75`, `-2.75`},
		{`1e3`, `1000`},
		{`-1.5e3`, `-1500`},
		{`1e-3`, `0.001`},
		{`1e308`, `1` + strings.Repeat("0", 308)},
		{`5e-324`, `0.` + strings.Repeat("0", 323) + `5`},
	} {
		got, err := canonicalJSON([]byte(`{"n":` + testCase.raw + `}`))
		if err != nil {
			t.Fatalf("canonicalJSON(%s): %v", testCase.raw, err)
		}
		if want := `{"n":` + testCase.want + `}`; string(got) != want {
			t.Fatalf("canonicalJSON(%s) = %s, want %s", testCase.raw, got, want)
		}
	}
}

func TestCanonicalJSONNormalizesEquivalentDecimalSpellings(t *testing.T) {
	want, err := canonicalJSON([]byte(`{"n":1}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range [][]byte{
		[]byte(`{"n":1.0}`),
		[]byte(`{"n":10e-1}`),
		[]byte(`{"n":0.01e2}`),
	} {
		got, err := canonicalJSON(raw)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("canonical JSON = %s, want %s", got, want)
		}
	}
}

func TestPrivateInventoryMustDeclareCompleteness(t *testing.T) {
	for _, raw := range [][]byte{
		[]byte(`{"complete":false}`),
		[]byte(`{"repositories":[]}`),
		[]byte(`[]`),
	} {
		if err := validateInventory(raw); err == nil {
			t.Fatal("incomplete private inventory was accepted")
		}
	}
}

func TestAuthorizationRejectsBindingReplayFreshnessAndBaselineMovement(t *testing.T) {
	request, plan, desired, evidence, toolchain := testAuthorizationInputs(t)
	tests := []struct {
		name string
		code Code
		edit func(*AuthorizationRequest, *privateEvidence)
	}{
		{"incomplete inventory", CodeInventoryIncomplete, func(request *AuthorizationRequest, _ *privateEvidence) { request.InventoryComplete = false }},
		{"targeted profile", CodePlanProfile, func(request *AuthorizationRequest, _ *privateEvidence) {
			request.PlanProfile.Targets = []string{"private"}
		}},
		{"refresh disabled", CodePlanProfile, func(request *AuthorizationRequest, _ *privateEvidence) { request.PlanProfile.Refresh = false }},
		{"wrong source ref", CodeInvalidInput, func(request *AuthorizationRequest, _ *privateEvidence) { request.Source.Ref = "refs/heads/private" }},
		{"future issuance", CodeFreshness, func(request *AuthorizationRequest, _ *privateEvidence) {
			request.IssuedAt = request.Now.Add(time.Second)
		}},
		{"expired", CodeFreshness, func(request *AuthorizationRequest, _ *privateEvidence) { request.ExpiresAt = request.Now }},
		{"long ttl", CodeFreshness, func(request *AuthorizationRequest, _ *privateEvidence) {
			request.ExpiresAt = request.IssuedAt.Add(MaxCommitmentTTL + time.Second)
		}},
		{"non UTC verification clock", CodeFreshness, func(request *AuthorizationRequest, _ *privateEvidence) {
			request.Now = request.Now.In(time.FixedZone("private-offset", 60*60))
		}},
		{"fractional verification clock", CodeFreshness, func(request *AuthorizationRequest, _ *privateEvidence) {
			request.Now = request.Now.Add(time.Nanosecond)
		}},
		{"state moved", CodeBaselineMismatch, func(request *AuthorizationRequest, _ *privateEvidence) { request.State.Serial++ }},
		{"lineage moved", CodeBaselineMismatch, func(request *AuthorizationRequest, _ *privateEvidence) { request.State.Lineage = "private-other" }},
		{"inventory moved", CodeBaselineMismatch, func(_ *AuthorizationRequest, evidence *privateEvidence) {
			evidence.inventory = []byte(`{"other":true}`)
		}},
		{"backend moved", CodeBaselineMismatch, func(_ *AuthorizationRequest, evidence *privateEvidence) { evidence.backend = []byte("other = true\n") }},
		{"state snapshot moved", CodeBaselineMismatch, func(_ *AuthorizationRequest, evidence *privateEvidence) {
			evidence.stateSnapshot = []byte(`{"other":true}`)
		}},
		{"provider moved", CodeBaselineMismatch, func(_ *AuthorizationRequest, evidence *privateEvidence) {
			evidence.providerSnapshot = []byte(`{"other":true}`)
		}},
		{"migration moved", CodeBaselineMismatch, func(_ *AuthorizationRequest, evidence *privateEvidence) {
			evidence.migration = []byte(`{"other":true}`)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changedRequest := request
			changedRequest.BaselineReceipt = bytes.NewReader(testBaselineReceipt(t, request.HMACKey, evidence, toolchain, request.State))
			changedRequest.PlanProfile = cloneProfile(request.PlanProfile)
			changedEvidence := evidence
			test.edit(&changedRequest, &changedEvidence)
			_, err := authorizeEvaluated(changedRequest, strings.Repeat("6", 64), plan, desired, changedEvidence, toolchain)
			assertCode(t, err, test.code)
		})
	}
}

func TestAuthorizationCommitmentsAreNonceBoundAndDomainSeparated(t *testing.T) {
	request, _, _, evidence, _ := testAuthorizationInputs(t)
	first, state, err := authorizationCommitments(request.HMACKey, request.Nonce, request.State, evidence, []byte(`{"kind":"update"}`))
	if err != nil {
		t.Fatal(err)
	}
	secondNonce := bytes.Repeat([]byte{0x44}, CommitmentNonceBytes)
	second, _, err := authorizationCommitments(request.HMACKey, secondNonce, request.State, evidence, []byte(`{"kind":"update"}`))
	if err != nil {
		t.Fatal(err)
	}
	if first.InventoryHMACSHA256 == second.InventoryHMACSHA256 {
		t.Fatal("changing nonce did not change commitment")
	}
	values := []string{
		first.InventoryHMACSHA256, first.BackendHMACSHA256, first.StateSnapshotHMACSHA256,
		first.MigrationSurfaceHMACSHA256, first.ProviderSnapshotHMACSHA256,
		first.ChangeProjectionHMACSHA256, state.LineageHMACSHA256,
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, duplicate := seen[value]; duplicate {
			t.Fatal("domain-separated commitments collided")
		}
		seen[value] = struct{}{}
	}
}

func TestAuthorizationVerificationRejectsTamperReplayAndNoncanonicalJSON(t *testing.T) {
	request, plan, desired, evidence, toolchain := testAuthorizationInputs(t)
	raw, err := authorizeEvaluated(request, strings.Repeat("5", 64), plan, desired, evidence, toolchain)
	if err != nil {
		t.Fatal(err)
	}
	claims, _ := parseAuthorization(raw)
	expected := ExpectedAuthorization{
		SubjectSHA256: claims.SubjectSHA256,
		Source:        claims.Source,
		Toolchain:     claims.Toolchain,
		State:         claims.State,
		Nonce:         claims.PrivateEvidence.Nonce,
		Now:           request.Now,
	}
	tests := []struct {
		name string
		raw  []byte
		edit func(*ExpectedAuthorization)
	}{
		{"replay nonce", raw, func(value *ExpectedAuthorization) { value.Nonce = strings.Repeat("A", len(value.Nonce)) }},
		{"main moved", raw, func(value *ExpectedAuthorization) { value.Source.CommitSHA = strings.Repeat("f", 40) }},
		{"state moved", raw, func(value *ExpectedAuthorization) { value.State.Serial++ }},
		{"plan substituted", raw, func(value *ExpectedAuthorization) { value.SubjectSHA256 = strings.Repeat("f", 64) }},
		{"expired on verify", raw, func(value *ExpectedAuthorization) { value.Now = request.ExpiresAt }},
		{"non UTC verify clock", raw, func(value *ExpectedAuthorization) {
			value.Now = value.Now.In(time.FixedZone("private-offset", 60*60))
		}},
		{"fractional verify clock", raw, func(value *ExpectedAuthorization) { value.Now = value.Now.Add(time.Nanosecond) }},
		{"noncanonical whitespace", append([]byte(" "), raw...), func(*ExpectedAuthorization) {}},
		{"trailing document", append(append([]byte(nil), raw...), []byte(`{}`)...), func(*ExpectedAuthorization) {}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changed := expected
			test.edit(&changed)
			_, err := VerifyAuthorization(test.raw, changed)
			assertCode(t, err, CodeAuthorizationMismatch)
		})
	}

	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	document["decision"] = "tampered"
	tampered, _ := json.Marshal(document)
	_, err = VerifyAuthorization(tampered, expected)
	assertCode(t, err, CodeAuthorizationMismatch)
}

func testAuthorizationInputs(t *testing.T) (AuthorizationRequest, []byte, []byte, privateEvidence, ToolchainClaims) {
	t.Helper()
	plan := readFixture(t, "private-plan-update.json")
	desired := readFixture(t, "desired-ruleset-after.json")
	migration, err := evaluateMigrationManifest(readFixture(t, "migration-surface.json"))
	if err != nil {
		t.Fatal(err)
	}
	evidence := privateEvidence{
		inventory:        mustCanonicalFixture(t, "inventory.json"),
		backend:          mustOpaqueFixture(t, "backend.hcl"),
		stateSnapshot:    mustCanonicalFixture(t, "state.json"),
		migration:        migration,
		providerSnapshot: mustCanonicalFixture(t, "provider-snapshot.json"),
	}
	toolchain := testToolchainClaims()
	key := bytes.Repeat([]byte{0x41}, CommitmentKeyMinBytes)
	baseline := testBaselineReceipt(t, key, evidence, toolchain, PrivateState{Lineage: "private-lineage", Serial: 41})
	if _, err := parseReceipt(baseline); err != nil {
		t.Fatalf("baseline fixture is invalid: %v; %s", err, baseline)
	}
	request := AuthorizationRequest{
		BaselineReceipt: bytes.NewReader(baseline),
		Platform:        "darwin_arm64",
		Source: SourceClaims{
			Repository: "vbonnet/dear-agent", Ref: "refs/heads/main",
			CommitSHA: strings.Repeat("a", 40), TreeSHA: strings.Repeat("b", 40),
			CanonicalRulesetBlobSHA: strings.Repeat("c", 40),
		},
		State: PrivateState{Lineage: "private-lineage", Serial: 41},
		Ruleset: RulesetBinding{
			Address:     `module.managed_repos["dear-agent"].github_repository_ruleset.branch_protection`,
			ImmutableID: "18061003",
			Repository:  "dear-agent",
		},
		PlanProfile:       PlanProfile{Mode: "normal", Refresh: true, Targets: []string{}, Excludes: []string{}, Replaces: []string{}},
		InventoryComplete: true,
		IssuedAt:          time.Date(2026, 8, 11, 1, 1, 0, 0, time.UTC),
		NotBefore:         time.Date(2026, 8, 11, 1, 1, 0, 0, time.UTC),
		ExpiresAt:         time.Date(2026, 8, 11, 1, 10, 0, 0, time.UTC),
		Now:               time.Date(2026, 8, 11, 1, 2, 0, 0, time.UTC),
		HMACKey:           key,
		Nonce:             bytes.Repeat([]byte{0x42}, CommitmentNonceBytes),
	}
	return request, plan, desired, evidence, toolchain
}

func testToolchainClaims() ToolchainClaims {
	lock := officialPlatformLocks["darwin_arm64"]
	return ToolchainClaims{
		Platform: "darwin_arm64", OpenTofuVersion: OpenTofuVersion, OpenTofuTagCommit: OpenTofuTagCommit,
		OpenTofuArchiveSHA256: lock.OpenTofuArchiveSHA256, OpenTofuBinarySHA256: lock.OpenTofuBinarySHA256,
		ToolchainManifestSHA256: ToolchainManifestSHA256, DependencyLockfileSHA256: DependencyLockSHA256,
		Providers: []ProviderClaims{{
			Address: ProviderAddress, Version: ProviderVersion, TagCommit: ProviderTagCommit,
			ArchiveSHA256: lock.ProviderArchiveSHA256, BinarySHA256: lock.ProviderBinarySHA256,
		}},
	}
}

func testBaselineReceipt(t *testing.T, key []byte, evidence privateEvidence, toolchain ToolchainClaims, state PrivateState) []byte {
	t.Helper()
	nonce := bytes.Repeat([]byte{0x21}, CommitmentNonceBytes)
	privateClaims, stateClaims, err := receiptCommitments(key, nonce, state, evidence)
	if err != nil {
		t.Fatal(err)
	}
	receipt := ReceiptClaims{
		Schema: ReceiptSchema, Decision: "provider-reconciled", Operation: "production-apply",
		AuthorizationClaimsSHA256: strings.Repeat("7", 64), AppliedPlanSHA256: strings.Repeat("8", 64),
		Source: SourceClaims{
			Repository: "vbonnet/dear-agent", Ref: "refs/heads/main", CommitSHA: strings.Repeat("d", 40),
			TreeSHA: strings.Repeat("e", 40), CanonicalRulesetBlobSHA: strings.Repeat("f", 40),
		},
		Toolchain: toolchain, PrivateEvidence: privateClaims, State: stateClaims,
		Verification: VerificationClaims{ProviderVisible: true, NoDrift: true, SourceParity: true, BehavioralCanary: true},
		ObservedAt:   "2026-08-11T00:50:00Z",
	}
	raw, err := canonicalStruct(receipt)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func decodePlanForTest(t *testing.T, raw []byte) rawPlan {
	t.Helper()
	var plan rawPlan
	if _, err := decodeStrict(raw, &plan); err != nil {
		t.Fatal(err)
	}
	return plan
}

func encodePlanForTest(t *testing.T, plan rawPlan) []byte {
	t.Helper()
	raw, err := canonicalStruct(plan)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func setPlanField(t *testing.T, plan *rawPlan, field string, value any) {
	t.Helper()
	for _, target := range []*json.RawMessage{&plan.ResourceChanges[0].Change.Before, &plan.ResourceChanges[0].Change.After} {
		var object map[string]any
		if err := json.Unmarshal(*target, &object); err != nil {
			t.Fatal(err)
		}
		object[field] = value
		raw, _ := json.Marshal(object)
		*target = raw
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func mustCanonicalFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := canonicalJSON(readFixture(t, name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func mustOpaqueFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := canonicalOpaque(readFixture(t, name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func assertCode(t *testing.T, err error, want Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected rejection %s", want)
	}
	var rejection *Rejection
	if !errors.As(err, &rejection) || rejection.Code != want {
		t.Fatalf("error = %v, want code %s", err, want)
	}
}

func cloneProfile(profile PlanProfile) PlanProfile {
	profile.Targets = append([]string{}, profile.Targets...)
	profile.Excludes = append([]string{}, profile.Excludes...)
	profile.Replaces = append([]string{}, profile.Replaces...)
	return profile
}
