package main

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestPlanBaselineUpdateRejectsExpansion(t *testing.T) {
	prior := baseline{Version: baselineSchemaV1, Findings: keySet(map[string][]string{
		"dead-package": {"pkg/a"},
	})}
	current := findingSet(map[string][]string{
		"dead-package": {"pkg/a", "pkg/b"},
	})
	before := cloneBaseline(prior)

	_, err := planBaselineUpdate(prior, current, strings.Repeat("a", 64), updateRequest{})
	if err == nil || !strings.Contains(err.Error(), "1 new finding") {
		t.Fatalf("planBaselineUpdate error = %v, want new-finding rejection", err)
	}
	if !reflect.DeepEqual(prior, before) {
		t.Fatalf("rejected update mutated prior baseline\n got: %#v\nwant: %#v", prior, before)
	}
}

func TestPlanBaselineUpdateRejectsSameCountReplacement(t *testing.T) {
	prior := baseline{Version: baselineSchemaV1, Findings: keySet(map[string][]string{
		"dead-package": {"pkg/old"},
	})}
	current := findingSet(map[string][]string{
		"dead-package": {"pkg/new"},
	})

	_, err := planBaselineUpdate(prior, current, strings.Repeat("b", 64), updateRequest{})
	if err == nil {
		t.Fatal("planBaselineUpdate accepted same-count key replacement")
	}
}

func TestPlanBaselineUpdateRecordsExplicitAdmission(t *testing.T) {
	prior := baseline{Version: baselineSchemaV1, Findings: keySet(map[string][]string{
		"dead-package": {"pkg/keep", "pkg/old"},
	})}
	current := findingSet(map[string][]string{
		"dead-package": {"pkg/keep", "pkg/new"},
	})
	request := updateRequest{
		AcceptNew: true,
		Reason:    "restore audited green reference",
		Reference: "ce-3knl.13.1 / engram-research#250",
	}

	plan, err := planBaselineUpdate(prior, current, strings.Repeat("c", 64), request)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Write {
		t.Fatal("explicit admission should produce a write")
	}
	if plan.Baseline.Version != baselineSchemaV2 || plan.Baseline.ScannerVersion != scannerKeyVersion {
		t.Fatalf("planned versions = schema %d scanner %d", plan.Baseline.Version, plan.Baseline.ScannerVersion)
	}
	if len(plan.Baseline.Transitions) != 1 {
		t.Fatalf("transitions = %d, want 1", len(plan.Baseline.Transitions))
	}
	transition := plan.Baseline.Transitions[0]
	if transition.PreviousScannerVersion != scannerKeyVersion {
		t.Errorf("previous scanner version = %d, want %d", transition.PreviousScannerVersion, scannerKeyVersion)
	}
	if got := transition.Added["dead-package"]; !reflect.DeepEqual(got, []string{"pkg/new"}) {
		t.Errorf("added = %v, want [pkg/new]", got)
	}
	if got := transition.Removed["dead-package"]; !reflect.DeepEqual(got, []string{"pkg/old"}) {
		t.Errorf("removed = %v, want [pkg/old]", got)
	}
	if transition.Reason != request.Reason || transition.Reference != request.Reference {
		t.Errorf("admission = %q / %q", transition.Reason, transition.Reference)
	}
}

func TestPlanBaselineUpdatePreservesTransitionHistory(t *testing.T) {
	prior := validV2Baseline(keySet(map[string][]string{
		"dead-package": {"pkg/a", "pkg/b"},
	}))
	current := findingSet(map[string][]string{
		"dead-package": {"pkg/a"},
	})

	plan, err := planBaselineUpdate(prior, current, mustBaselineSHA256(t, prior), updateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Baseline.Transitions) != len(prior.Transitions)+1 {
		t.Fatalf("transitions = %d, want %d", len(plan.Baseline.Transitions), len(prior.Transitions)+1)
	}
	if !reflect.DeepEqual(plan.Baseline.Transitions[0], prior.Transitions[0]) {
		t.Fatal("prior transition history changed")
	}
	last := plan.Baseline.Transitions[len(plan.Baseline.Transitions)-1]
	if got := last.Removed["dead-package"]; !reflect.DeepEqual(got, []string{"pkg/b"}) {
		t.Errorf("removed = %v, want [pkg/b]", got)
	}
	if last.Reason != "" || last.Reference != "" {
		t.Errorf("shrink transition unexpectedly has admission metadata: %#v", last)
	}
}

func TestPlanBaselineUpdateValidatesAdmissionFlags(t *testing.T) {
	prior := baseline{Version: baselineSchemaV1, Findings: keySet(nil)}
	currentWithNew := findingSet(map[string][]string{"zero-test": {"pkg/new"}})
	currentEmpty := findingSet(nil)

	tests := []struct {
		name    string
		current map[string][]finding
		request updateRequest
	}{
		{name: "missing reason", current: currentWithNew, request: updateRequest{AcceptNew: true, Reference: "ce-x"}},
		{name: "missing reference", current: currentWithNew, request: updateRequest{AcceptNew: true, Reason: "because"}},
		{name: "reason without accept", current: currentWithNew, request: updateRequest{Reason: "because"}},
		{name: "unnecessary accept", current: currentEmpty, request: updateRequest{AcceptNew: true, Reason: "because", Reference: "ce-x"}},
		{name: "non-durable reference", current: currentWithNew, request: updateRequest{AcceptNew: true, Reason: "because", Reference: "x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := planBaselineUpdate(prior, tt.current, strings.Repeat("e", 64), tt.request); err == nil {
				t.Fatal("planBaselineUpdate accepted invalid admission flags")
			}
		})
	}
}

func TestValidateUpdateAuthorizationRequiresScannerMigration(t *testing.T) {
	change := baselineChange{Added: keySet(nil), Removed: keySet(nil)}
	if _, _, err := validateUpdateAuthorization(change, 1, 2, updateRequest{}); err == nil {
		t.Fatal("scanner version change succeeded without explicit authorization")
	}
	reason, reference, err := validateUpdateAuthorization(change, 1, 2, updateRequest{
		AcceptScannerChange: true,
		Reason:              "migrate stable-key semantics",
		Reference:           "ce-version.2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if reason == "" || reference == "" {
		t.Fatal("scanner migration discarded provenance")
	}
}

func TestIsDurableReference(t *testing.T) {
	tests := []struct {
		name      string
		reference string
		want      bool
	}{
		{name: "tracker", reference: "ce-3knl.13.1", want: true},
		{name: "pull request", reference: "vbonnet/dear-agent#1100", want: true},
		{name: "https url with path", reference: "https://github.com/vbonnet/dear-agent/issues/1", want: true},
		{name: "qualified commit", reference: "dear-agent@c0da2ae6", want: true},
		{name: "bare commit", reference: strings.Repeat("a", 40), want: true},
		{name: "opaque text", reference: "reviewed-by-the-team", want: false},
		{name: "host only url", reference: "https://github.com", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDurableReference(tt.reference); got != tt.want {
				t.Fatalf("isDurableReference(%q) = %v, want %v", tt.reference, got, tt.want)
			}
		})
	}
}

func TestValidateModeFlagsRejectsJSONUpdate(t *testing.T) {
	if err := validateModeFlags(true, true, updateRequest{}); err == nil {
		t.Fatal("--json --update-baseline combination was accepted")
	}
}

func TestCheckedInBaselineV2Transition(t *testing.T) {
	prior, priorBytes, err := readBaselineFile(filepath.Join("testdata", "baseline-v1-c0da2ae6.json"))
	if err != nil {
		t.Fatal(err)
	}
	checkedIn, _, err := readBaselineFile(filepath.Join("..", "..", defaultBaselinePath))
	if err != nil {
		t.Fatal(err)
	}
	if prior.Version != baselineSchemaV1 {
		t.Fatalf("fixture schema = %d, want %d", prior.Version, baselineSchemaV1)
	}
	if checkedIn.Version != baselineSchemaV2 || len(checkedIn.Transitions) == 0 {
		t.Fatalf("checked-in baseline = schema %d with %d transitions", checkedIn.Version, len(checkedIn.Transitions))
	}

	first := checkedIn.Transitions[0]
	wantDigest := fmt.Sprintf("%x", sha256.Sum256(priorBytes))
	if first.PreviousBaselineSHA256 != wantDigest {
		t.Fatalf("first transition prior digest = %s, want %s", first.PreviousBaselineSHA256, wantDigest)
	}
	if got := keyMapCount(first.Added); got != 50 {
		t.Fatalf("first transition added = %d, want 50", got)
	}
	if got := keyMapCount(first.Removed); got != 23 {
		t.Fatalf("first transition removed = %d, want 23", got)
	}

	// Derive the snapshot immediately after the first transition by undoing
	// any later transitions. This keeps the provenance test valid as the
	// ratchet tightens and history grows.
	afterFirst := cloneKeyMap(checkedIn.Findings)
	for i := len(checkedIn.Transitions) - 1; i > 0; i-- {
		transition := checkedIn.Transitions[i]
		afterFirst = applyKeyChange(t, afterFirst, baselineChange{
			Added:   transition.Removed,
			Removed: transition.Added,
		})
	}
	firstChange := compareKeyMaps(prior.Findings, afterFirst)
	if !reflect.DeepEqual(firstChange.Added, first.Added) {
		t.Fatalf("first transition added map does not match exact set difference")
	}
	if !reflect.DeepEqual(firstChange.Removed, first.Removed) {
		t.Fatalf("first transition removed map does not match exact set difference")
	}

	replayed := cloneKeyMap(prior.Findings)
	replayedScannerVersion := effectiveScannerVersion(prior)
	replayedBytes := priorBytes
	for i, transition := range checkedIn.Transitions {
		stepDigest := fmt.Sprintf("%x", sha256.Sum256(replayedBytes))
		if transition.PreviousBaselineSHA256 != stepDigest {
			t.Fatalf(
				"transition %d prior digest = %s, want %s",
				i,
				transition.PreviousBaselineSHA256,
				stepDigest,
			)
		}
		if transition.PreviousScannerVersion != replayedScannerVersion {
			t.Fatalf(
				"transition %d previous scanner version = %d, want %d",
				i,
				transition.PreviousScannerVersion,
				replayedScannerVersion,
			)
		}
		replayed = applyKeyChange(t, replayed, baselineChange{
			Added:   transition.Added,
			Removed: transition.Removed,
		})
		replayedScannerVersion = transition.ScannerVersion
		replayedBytes = mustMarshalBaseline(t, baseline{
			Version:        baselineSchemaV2,
			ScannerVersion: replayedScannerVersion,
			Findings:       replayed,
			Transitions:    cloneTransitions(checkedIn.Transitions[:i+1]),
		})
	}
	if !reflect.DeepEqual(replayed, checkedIn.Findings) {
		t.Fatal("transition history does not reconstruct checked-in findings")
	}
	if replayedScannerVersion != checkedIn.ScannerVersion {
		t.Fatalf("replayed scanner version = %d, want %d", replayedScannerVersion, checkedIn.ScannerVersion)
	}
}

func applyKeyChange(t *testing.T, before map[string][]string, change baselineChange) map[string][]string {
	t.Helper()
	out := emptyKeyMap()
	for _, scan := range scanNames {
		keys := make(map[string]bool, len(before[scan]))
		for _, key := range before[scan] {
			keys[key] = true
		}
		for _, key := range change.Removed[scan] {
			if !keys[key] {
				t.Fatalf("transition removes absent key %q from %s", key, scan)
			}
			delete(keys, key)
		}
		for _, key := range change.Added[scan] {
			if keys[key] {
				t.Fatalf("transition adds existing key %q to %s", key, scan)
			}
			keys[key] = true
		}
		for key := range keys {
			out[scan] = append(out[scan], key)
		}
		sort.Strings(out[scan])
	}
	return out
}

func TestV1NoopMigratesAndV2NoopDoesNotWrite(t *testing.T) {
	current := findingSet(map[string][]string{"dead-package": {"pkg/a"}})
	v1 := baseline{Version: baselineSchemaV1, Findings: keySet(map[string][]string{
		"dead-package": {"pkg/a"},
	})}

	v1Plan, err := planBaselineUpdate(v1, current, strings.Repeat("f", 64), updateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !v1Plan.Write || len(v1Plan.Baseline.Transitions) != 1 {
		t.Fatalf("v1 no-op plan = write %v transitions %d", v1Plan.Write, len(v1Plan.Baseline.Transitions))
	}
	if v1Plan.Change.count() != 0 {
		t.Fatalf("v1 migration change count = %d, want 0", v1Plan.Change.count())
	}

	v2Plan, err := planBaselineUpdate(v1Plan.Baseline, current, strings.Repeat("0", 64), updateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if v2Plan.Write {
		t.Fatal("v2 no-op should not rewrite baseline")
	}
}

func TestReadBaselineValidation(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "unsupported schema", data: `{"version":3,"findings":{}}`},
		{name: "unsupported root scanner version", data: baselineJSONWithUnsupportedScanner(t)},
		{name: "unsupported transition scanner version", data: baselineJSONWithUnsupportedTransitionScanner(t)},
		{name: "missing scans", data: `{"version":1,"findings":{"dead-package":[]}}`},
		{name: "unknown scan", data: baselineJSONWithExtraScan(t)},
		{name: "nil list", data: baselineJSONWithNilScan(t)},
		{name: "duplicate key", data: baselineJSONWithDuplicate(t)},
		{name: "unknown field", data: baselineJSONWithUnknownField(t)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "baseline.json")
			if err := os.WriteFile(path, []byte(tt.data), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := readBaseline(path); err == nil {
				t.Fatal("readBaseline accepted invalid manifest")
			}
		})
	}
}

func TestReadBaselineRejectsDuplicateJSONMembers(t *testing.T) {
	validV1 := string(mustMarshalBaseline(t, baseline{
		Version:  baselineSchemaV1,
		Findings: keySet(nil),
	}))
	validV2 := string(mustMarshalBaseline(t, validV2Baseline(keySet(nil))))
	tests := []struct {
		name       string
		data       string
		memberName string
	}{
		{
			name:       "root escaped equivalent",
			data:       strings.Replace(validV1, "{\n", "{\n  \"\\u0066indings\": {},\n", 1),
			memberName: "findings",
		},
		{
			name: "nested escaped equivalent",
			data: strings.Replace(
				validV1,
				"  \"findings\": {\n",
				"  \"findings\": {\n    \"\\u0064ead-package\": [],\n",
				1,
			),
			memberName: "dead-package",
		},
		{
			name: "transition object inside array",
			data: strings.Replace(
				validV2,
				"    {\n      \"previous_baseline_sha256\":",
				"    {\n      \"\\u0070revious_baseline_sha256\": \""+strings.Repeat("0", 64)+"\",\n      \"previous_baseline_sha256\":",
				1,
			),
			memberName: "previous_baseline_sha256",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "baseline.json")
			if err := os.WriteFile(path, []byte(tt.data), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := readBaseline(path)
			if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("duplicate JSON object member %q", tt.memberName)) {
				t.Fatalf("readBaseline error = %v, want duplicate member %q", err, tt.memberName)
			}
		})
	}
}

func TestReadBaselineValidatesEveryReconstructableTransitionDigest(t *testing.T) {
	initial := baseline{
		Version: baselineSchemaV1,
		Findings: keySet(map[string][]string{
			"dead-package": {"pkg/a"},
		}),
	}
	initialBytes := mustMarshalBaseline(t, initial)
	first, err := planBaselineUpdate(
		initial,
		findingSet(map[string][]string{"dead-package": {"pkg/a", "pkg/b"}}),
		fmt.Sprintf("%x", sha256.Sum256(initialBytes)),
		updateRequest{
			AcceptNew: true,
			Reason:    "admit audited test finding",
			Reference: "ce-test",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := planBaselineUpdate(
		first.Baseline,
		findingSet(map[string][]string{"dead-package": {"pkg/a"}}),
		mustBaselineSHA256(t, first.Baseline),
		updateRequest{},
	)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "baseline.json")
	if err := os.WriteFile(path, mustMarshalBaseline(t, second.Baseline), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readBaseline(path); err != nil {
		t.Fatalf("read valid digest chain: %v", err)
	}

	tamperedCases := []struct {
		name   string
		mutate func(*baseline)
	}{
		{
			name: "later digest",
			mutate: func(bl *baseline) {
				bl.Transitions[1].PreviousBaselineSHA256 = strings.Repeat("f", 64)
			},
		},
		{
			name: "predecessor history metadata",
			mutate: func(bl *baseline) {
				bl.Transitions[0].Reason = "different but otherwise valid provenance"
			},
		},
	}
	for _, tt := range tamperedCases {
		t.Run(tt.name, func(t *testing.T) {
			tampered := cloneBaseline(second.Baseline)
			tt.mutate(&tampered)
			if err := os.WriteFile(path, mustMarshalBaseline(t, tampered), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := readBaseline(path)
			if err == nil || !strings.Contains(err.Error(), "transition 1 previous_baseline_sha256") {
				t.Fatalf("readBaseline error = %v, want transition digest mismatch", err)
			}
		})
	}
}

func TestUpdateBaselineFileRejectionPreservesBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	prior := baseline{Version: baselineSchemaV1, Findings: keySet(map[string][]string{
		"dead-package": {"pkg/fixed", "pkg/keep"},
	})}
	priorBytes := mustMarshalBaseline(t, prior)
	if err := os.WriteFile(path, priorBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	current := findingSet(map[string][]string{
		"dead-package": {"pkg/keep", "pkg/new"},
	})

	if _, err := updateBaselineFile(path, current, updateRequest{}); err == nil {
		t.Fatal("updateBaselineFile accepted expansion")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, priorBytes) {
		t.Fatalf("rejected update changed baseline\n got: %s\nwant: %s", got, priorBytes)
	}
}

func TestUpdateBaselineFileRecordsLiteralPriorDigest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	prior := baseline{Version: baselineSchemaV1, Findings: keySet(nil)}
	priorBytes := mustMarshalBaseline(t, prior)
	if err := os.WriteFile(path, priorBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	current := findingSet(map[string][]string{"zero-test": {"pkg/new"}})
	request := updateRequest{AcceptNew: true, Reason: "audited", Reference: "ce-test"}

	plan, err := updateBaselineFile(path, current, request)
	if err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(priorBytes))
	if got := plan.Baseline.Transitions[0].PreviousBaselineSHA256; got != digest {
		t.Fatalf("previous digest = %s, want %s", got, digest)
	}
	bl, err := readBaseline(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(bl, plan.Baseline) {
		t.Fatalf("persisted baseline differs from plan\n got: %#v\nwant: %#v", bl, plan.Baseline)
	}
}

func TestUpdateBaselineFileMigratesV1NoopAndThenPreservesBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	prior := baseline{Version: baselineSchemaV1, Findings: keySet(map[string][]string{
		"dead-package": {"pkg/a"},
	})}
	priorBytes := mustMarshalBaseline(t, prior)
	if err := os.WriteFile(path, priorBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	current := findingSet(map[string][]string{"dead-package": {"pkg/a"}})

	plan, err := updateBaselineFile(path, current, updateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Write || plan.Change.count() != 0 {
		t.Fatalf("migration plan = write %v change %d", plan.Write, plan.Change.count())
	}
	wantDigest := fmt.Sprintf("%x", sha256.Sum256(priorBytes))
	if got := plan.Baseline.Transitions[0].PreviousBaselineSHA256; got != wantDigest {
		t.Fatalf("migration digest = %s, want %s", got, wantDigest)
	}
	migratedBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(migratedBytes, priorBytes) {
		t.Fatal("v1 no-op did not migrate the on-disk schema")
	}
	if _, err := readBaseline(path); err != nil {
		t.Fatalf("read migrated baseline: %v", err)
	}

	second, err := updateBaselineFile(path, current, updateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if second.Write {
		t.Fatal("v2 no-op rewrote the baseline")
	}
	afterNoop, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterNoop, migratedBytes) {
		t.Fatal("v2 no-op changed literal baseline bytes")
	}
}

func TestUpdateBaselineFileCanonicalizesV2PredecessorDigest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	prior := baseline{Version: baselineSchemaV1, Findings: keySet(map[string][]string{
		"dead-package": {"pkg/a", "pkg/b"},
	})}
	if err := os.WriteFile(path, mustMarshalBaseline(t, prior), 0o644); err != nil {
		t.Fatal(err)
	}

	migrated, err := updateBaselineFile(
		path,
		findingSet(map[string][]string{"dead-package": {"pkg/a", "pkg/b"}}),
		updateRequest{},
	)
	if err != nil {
		t.Fatal(err)
	}
	canonicalBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	reformatted := bytes.Replace(canonicalBytes, []byte("{\n"), []byte("{  \n"), 1)
	if bytes.Equal(reformatted, canonicalBytes) {
		t.Fatal("test did not reformat the v2 baseline")
	}
	if err := os.WriteFile(path, reformatted, 0o644); err != nil {
		t.Fatal(err)
	}

	updated, err := updateBaselineFile(
		path,
		findingSet(map[string][]string{"dead-package": {"pkg/a"}}),
		updateRequest{},
	)
	if err != nil {
		t.Fatal(err)
	}
	last := updated.Baseline.Transitions[len(updated.Baseline.Transitions)-1]
	wantDigest := mustBaselineSHA256(t, migrated.Baseline)
	if last.PreviousBaselineSHA256 != wantDigest {
		t.Fatalf("v2 predecessor digest = %s, want canonical %s", last.PreviousBaselineSHA256, wantDigest)
	}
	literalDigest := fmt.Sprintf("%x", sha256.Sum256(reformatted))
	if last.PreviousBaselineSHA256 == literalDigest {
		t.Fatal("v2 predecessor digest unexpectedly used reformatted literal bytes")
	}
	if _, err := readBaseline(path); err != nil {
		t.Fatalf("read updated baseline: %v", err)
	}
}

func TestUpdateBaselineFileBootstrapRequiresAdmission(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.json")
	current := findingSet(map[string][]string{"zero-test": {"pkg/new"}})

	if _, err := updateBaselineFile(path, current, updateRequest{}); err == nil {
		t.Fatal("bootstrap accepted findings without admission")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("bootstrap rejection created destination: %v", err)
	}
}

func TestWriteBaselineAtomicRenameFailurePreservesOriginal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	original := []byte("original bytes\n")
	if err := os.WriteFile(path, original, 0o640); err != nil {
		t.Fatal(err)
	}
	bl := validV2Baseline(keySet(nil))

	err := writeBaselineAtomicWithRename(path, bl, func(_, _ string) error {
		return errors.New("injected rename failure")
	})
	if err == nil {
		t.Fatal("writeBaselineAtomicWithRename succeeded")
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("atomic failure changed destination: %q", got)
	}
	info, statErr := os.Stat(path)
	if statErr != nil {
		t.Fatal(statErr)
	}
	if gotMode := info.Mode().Perm(); gotMode != 0o640 {
		t.Fatalf("atomic failure changed destination mode to %o, want 640", gotMode)
	}
	matches, globErr := filepath.Glob(filepath.Join(filepath.Dir(path), ".baseline.json.tmp-*"))
	if globErr != nil {
		t.Fatal(globErr)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files remain: %v", matches)
	}
}

func keySet(overrides map[string][]string) map[string][]string {
	out := make(map[string][]string, len(scanNames))
	for _, name := range scanNames {
		keys := append([]string{}, overrides[name]...)
		if keys == nil {
			keys = []string{}
		}
		out[name] = keys
	}
	return out
}

func findingSet(overrides map[string][]string) map[string][]finding {
	out := make(map[string][]finding, len(scanNames))
	for _, name := range scanNames {
		for _, key := range overrides[name] {
			out[name] = append(out[name], finding{Key: key})
		}
		if out[name] == nil {
			out[name] = []finding{}
		}
	}
	return out
}

func validV2Baseline(findings map[string][]string) baseline {
	empty := keySet(nil)
	return baseline{
		Version:        baselineSchemaV2,
		ScannerVersion: scannerKeyVersion,
		Findings:       findings,
		Transitions: []baselineTransition{{
			PreviousBaselineSHA256: strings.Repeat("0", 64),
			PreviousScannerVersion: scannerKeyVersion,
			ScannerVersion:         scannerKeyVersion,
			Added:                  empty,
			Removed:                keySet(nil),
		}},
	}
}

func mustMarshalBaseline(t *testing.T, bl baseline) []byte {
	t.Helper()
	data, err := marshalBaseline(bl)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func mustBaselineSHA256(t *testing.T, bl baseline) string {
	t.Helper()
	digest, err := canonicalBaselineSHA256(bl)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func baselineJSONWithExtraScan(t *testing.T) string {
	t.Helper()
	bl := baseline{Version: baselineSchemaV1, Findings: keySet(nil)}
	bl.Findings["surprise"] = []string{}
	return string(mustMarshalBaseline(t, bl))
}

func baselineJSONWithNilScan(t *testing.T) string {
	t.Helper()
	bl := baseline{Version: baselineSchemaV1, Findings: keySet(nil)}
	bl.Findings["zero-test"] = nil
	return string(mustMarshalBaseline(t, bl))
}

func baselineJSONWithDuplicate(t *testing.T) string {
	t.Helper()
	bl := baseline{Version: baselineSchemaV1, Findings: keySet(map[string][]string{
		"zero-test": {"pkg/a", "pkg/a"},
	})}
	return string(mustMarshalBaseline(t, bl))
}

func baselineJSONWithUnknownField(t *testing.T) string {
	t.Helper()
	data := mustMarshalBaseline(t, baseline{Version: baselineSchemaV1, Findings: keySet(nil)})
	return strings.Replace(string(data), "{\n", "{\n  \"unexpected\": true,\n", 1)
}

func baselineJSONWithUnsupportedScanner(t *testing.T) string {
	t.Helper()
	bl := validV2Baseline(keySet(nil))
	bl.ScannerVersion = scannerKeyVersion + 1
	return string(mustMarshalBaseline(t, bl))
}

func baselineJSONWithUnsupportedTransitionScanner(t *testing.T) string {
	t.Helper()
	bl := validV2Baseline(keySet(nil))
	bl.Transitions[0].ScannerVersion = scannerKeyVersion + 1
	return string(mustMarshalBaseline(t, bl))
}
