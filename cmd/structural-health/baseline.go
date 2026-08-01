package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	baselineSchemaV1 = 1
	baselineSchemaV2 = 2
	legacyKeyVersion = 1

	// scannerKeyVersion changes only when a scan's stable-key semantics change.
	// It is independent from the baseline JSON schema version.
	scannerKeyVersion = 1
)

// baseline is the accepted structural-debt snapshot. Version 1 contained only
// Findings. Version 2 adds the stable-key version and append-only transition
// provenance while keeping Findings as the single accepted-key authority.
type baseline struct {
	Version        int                  `json:"version"`
	ScannerVersion int                  `json:"scanner_version,omitempty"`
	Findings       map[string][]string  `json:"findings"`
	Transitions    []baselineTransition `json:"transitions,omitempty"`
}

type baselineTransition struct {
	PreviousBaselineSHA256 string              `json:"previous_baseline_sha256"`
	PreviousScannerVersion int                 `json:"previous_scanner_version"`
	ScannerVersion         int                 `json:"scanner_version"`
	Added                  map[string][]string `json:"added"`
	Removed                map[string][]string `json:"removed"`
	Reason                 string              `json:"reason,omitempty"`
	Reference              string              `json:"reference,omitempty"`
}

type baselineChange struct {
	Added   map[string][]string
	Removed map[string][]string
}

func (c baselineChange) addedCount() int {
	return keyMapCount(c.Added)
}

func (c baselineChange) removedCount() int {
	return keyMapCount(c.Removed)
}

func (c baselineChange) count() int {
	return c.addedCount() + c.removedCount()
}

type updateRequest struct {
	AcceptNew           bool
	AcceptScannerChange bool
	Reason              string
	Reference           string
}

type baselineUpdatePlan struct {
	Baseline baseline
	Change   baselineChange
	Write    bool
}

// readBaseline preserves the scan-mode interface while readBaselineFile also
// exposes literal bytes for update provenance.
func readBaseline(path string) (baseline, error) {
	bl, _, err := readBaselineFile(path)
	if err == nil && effectiveScannerVersion(bl) != scannerKeyVersion {
		return baseline{}, fmt.Errorf(
			"baseline scanner version %d is not current version %d; migrate with --update-baseline --accept-scanner-change",
			effectiveScannerVersion(bl), scannerKeyVersion,
		)
	}
	return bl, err
}

func readBaselineFile(path string) (baseline, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return baseline{}, nil, err
	}

	var bl baseline
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&bl); err != nil {
		return baseline{}, nil, fmt.Errorf("parse: %w", err)
	}
	if err := ensureJSONEOF(dec); err != nil {
		return baseline{}, nil, fmt.Errorf("parse: %w", err)
	}
	if err := validateBaseline(bl); err != nil {
		return baseline{}, nil, err
	}
	return bl, data, nil
}

func ensureJSONEOF(dec *json.Decoder) error {
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func validateBaseline(bl baseline) error {
	switch bl.Version {
	case baselineSchemaV1:
		if bl.ScannerVersion != 0 || len(bl.Transitions) != 0 {
			return errors.New("baseline schema v1 cannot contain non-empty v2 metadata")
		}
	case baselineSchemaV2:
		if !supportedHistoricalScannerVersion(bl.ScannerVersion) {
			return fmt.Errorf("unsupported scanner version %d (supported range %d-%d)", bl.ScannerVersion, legacyKeyVersion, scannerKeyVersion)
		}
		if len(bl.Transitions) == 0 {
			return errors.New("baseline schema v2 requires transition history")
		}
	default:
		return fmt.Errorf("unsupported baseline schema version %d", bl.Version)
	}

	if err := validateKeyMap("findings", bl.Findings); err != nil {
		return err
	}
	for i, transition := range bl.Transitions {
		if err := validateTransition(transition); err != nil {
			return fmt.Errorf("transition %d: %w", i, err)
		}
	}
	return nil
}

func validateTransition(transition baselineTransition) error {
	if err := validateTransitionVersions(transition); err != nil {
		return err
	}
	if err := validateSHA256(transition.PreviousBaselineSHA256); err != nil {
		return fmt.Errorf("previous_baseline_sha256: %w", err)
	}
	if err := validateKeyMap("added", transition.Added); err != nil {
		return err
	}
	if err := validateKeyMap("removed", transition.Removed); err != nil {
		return err
	}
	if err := validateDisjointTransitionKeys(transition); err != nil {
		return err
	}
	return validateTransitionProvenance(transition)
}

func validateTransitionVersions(transition baselineTransition) error {
	if !supportedHistoricalScannerVersion(transition.PreviousScannerVersion) {
		return fmt.Errorf("unsupported previous scanner version %d", transition.PreviousScannerVersion)
	}
	if !supportedHistoricalScannerVersion(transition.ScannerVersion) {
		return fmt.Errorf("unsupported scanner version %d", transition.ScannerVersion)
	}
	if transition.PreviousScannerVersion > transition.ScannerVersion {
		return fmt.Errorf(
			"scanner version cannot decrease from %d to %d",
			transition.PreviousScannerVersion,
			transition.ScannerVersion,
		)
	}
	return nil
}

func validateDisjointTransitionKeys(transition baselineTransition) error {
	for _, scan := range scanNames {
		removed := make(map[string]bool, len(transition.Removed[scan]))
		for _, key := range transition.Removed[scan] {
			removed[key] = true
		}
		for _, key := range transition.Added[scan] {
			if removed[key] {
				return fmt.Errorf("key %q appears in both added and removed for scan %q", key, scan)
			}
		}
	}
	return nil
}

func validateTransitionProvenance(transition baselineTransition) error {
	requiresAuthorization := keyMapCount(transition.Added) > 0 ||
		transition.PreviousScannerVersion != transition.ScannerVersion
	reason := strings.TrimSpace(transition.Reason)
	reference := strings.TrimSpace(transition.Reference)
	if requiresAuthorization && (reason == "" || reference == "") {
		return errors.New("added keys and scanner changes require non-blank reason and reference")
	}
	if !requiresAuthorization && (reason != "" || reference != "") {
		return errors.New("reason and reference are only valid for added keys or scanner changes")
	}
	if reason != transition.Reason || reference != transition.Reference {
		return errors.New("reason and reference must not have surrounding whitespace")
	}
	if reference != "" && !isDurableReference(reference) {
		return errors.New("reference must contain a tracker ID, PR reference, HTTPS URL, or commit ID")
	}
	return nil
}

func supportedHistoricalScannerVersion(version int) bool {
	return version >= legacyKeyVersion && version <= scannerKeyVersion
}

func effectiveScannerVersion(bl baseline) int {
	if bl.Version == baselineSchemaV1 {
		return legacyKeyVersion
	}
	return bl.ScannerVersion
}

func validateSHA256(value string) error {
	if len(value) != sha256.Size*2 {
		return fmt.Errorf("must contain %d hexadecimal characters", sha256.Size*2)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return fmt.Errorf("must be hexadecimal: %w", err)
	}
	return nil
}

func validateKeyMap(label string, keysByScan map[string][]string) error {
	if keysByScan == nil {
		return fmt.Errorf("%s must not be null", label)
	}
	known := make(map[string]bool, len(scanNames))
	for _, scan := range scanNames {
		known[scan] = true
		keys, ok := keysByScan[scan]
		if !ok {
			return fmt.Errorf("%s missing canonical scan %q", label, scan)
		}
		if keys == nil {
			return fmt.Errorf("%s scan %q must be an array, not null", label, scan)
		}
		for i, key := range keys {
			if key == "" {
				return fmt.Errorf("%s scan %q contains an empty key", label, scan)
			}
			if i > 0 && keys[i-1] >= key {
				return fmt.Errorf("%s scan %q keys must be sorted and unique", label, scan)
			}
		}
	}
	for scan := range keysByScan {
		if !known[scan] {
			return fmt.Errorf("%s contains unknown scan %q", label, scan)
		}
	}
	return nil
}

func findingKeys(current map[string][]finding) (map[string][]string, error) {
	known := make(map[string]bool, len(scanNames))
	for _, scan := range scanNames {
		known[scan] = true
	}
	for scan := range current {
		if !known[scan] {
			return nil, fmt.Errorf("current findings contain unknown scan %q", scan)
		}
	}

	out := emptyKeyMap()
	for _, scan := range scanNames {
		keys := make([]string, 0, len(current[scan]))
		for _, finding := range current[scan] {
			if finding.Key == "" {
				return nil, fmt.Errorf("current scan %q contains an empty key", scan)
			}
			keys = append(keys, finding.Key)
		}
		sort.Strings(keys)
		for i := 1; i < len(keys); i++ {
			if keys[i-1] == keys[i] {
				return nil, fmt.Errorf("current scan %q contains duplicate key %q", scan, keys[i])
			}
		}
		out[scan] = keys
	}
	return out, nil
}

func planBaselineUpdate(
	previous baseline,
	current map[string][]finding,
	previousSHA string,
	request updateRequest,
) (baselineUpdatePlan, error) {
	if err := validateBaseline(previous); err != nil {
		return baselineUpdatePlan{}, fmt.Errorf("validate previous baseline: %w", err)
	}
	if err := validateSHA256(previousSHA); err != nil {
		return baselineUpdatePlan{}, fmt.Errorf("previous baseline digest: %w", err)
	}
	currentKeys, err := findingKeys(current)
	if err != nil {
		return baselineUpdatePlan{}, err
	}
	change := compareKeyMaps(previous.Findings, currentKeys)
	previousScannerVersion := effectiveScannerVersion(previous)
	reason, reference, err := validateUpdateAuthorization(
		change,
		previousScannerVersion,
		scannerKeyVersion,
		request,
	)
	if err != nil {
		return baselineUpdatePlan{}, err
	}

	if previous.Version == baselineSchemaV2 && change.count() == 0 &&
		previousScannerVersion == scannerKeyVersion {
		return baselineUpdatePlan{
			Baseline: cloneBaseline(previous),
			Change:   change,
			Write:    false,
		}, nil
	}

	transitions := cloneTransitions(previous.Transitions)
	transitions = append(transitions, baselineTransition{
		PreviousBaselineSHA256: previousSHA,
		PreviousScannerVersion: previousScannerVersion,
		ScannerVersion:         scannerKeyVersion,
		Added:                  cloneKeyMap(change.Added),
		Removed:                cloneKeyMap(change.Removed),
		Reason:                 reason,
		Reference:              reference,
	})
	planned := baseline{
		Version:        baselineSchemaV2,
		ScannerVersion: scannerKeyVersion,
		Findings:       currentKeys,
		Transitions:    transitions,
	}
	if err := validateBaseline(planned); err != nil {
		return baselineUpdatePlan{}, fmt.Errorf("validate planned baseline: %w", err)
	}
	return baselineUpdatePlan{Baseline: planned, Change: change, Write: true}, nil
}

func validateUpdateAuthorization(
	change baselineChange,
	previousScannerVersion int,
	targetScannerVersion int,
	request updateRequest,
) (string, string, error) {
	reason := strings.TrimSpace(request.Reason)
	reference := strings.TrimSpace(request.Reference)
	if err := validateUnboundAdmissionMetadata(request, reason, reference); err != nil {
		return "", "", err
	}
	if err := validateAddedKeyAuthorization(change.addedCount(), request.AcceptNew); err != nil {
		return "", "", err
	}
	if err := validateScannerChangeAuthorization(
		previousScannerVersion,
		targetScannerVersion,
		request.AcceptScannerChange,
	); err != nil {
		return "", "", err
	}
	if err := validateAdmissionProvenance(request, reason, reference); err != nil {
		return "", "", err
	}
	return reason, reference, nil
}

func validateUnboundAdmissionMetadata(request updateRequest, reason, reference string) error {
	if !request.AcceptNew && !request.AcceptScannerChange && (reason != "" || reference != "") {
		return errors.New("--reason and --reference require --accept-new or --accept-scanner-change")
	}
	return nil
}

func validateAddedKeyAuthorization(addedCount int, acceptNew bool) error {
	if addedCount > 0 && !acceptNew {
		return fmt.Errorf(
			"%d new finding(s) require --accept-new with --reason and --reference",
			addedCount,
		)
	}
	if addedCount == 0 && acceptNew {
		return errors.New("--accept-new is invalid when the update adds no findings")
	}
	return nil
}

func validateScannerChangeAuthorization(previousVersion, targetVersion int, acceptChange bool) error {
	scannerChanged := previousVersion != targetVersion
	if scannerChanged && !acceptChange {
		return fmt.Errorf(
			"scanner version change %d to %d requires --accept-scanner-change with --reason and --reference",
			previousVersion,
			targetVersion,
		)
	}
	if !scannerChanged && acceptChange {
		return errors.New("--accept-scanner-change is invalid when the scanner version is unchanged")
	}
	return nil
}

func validateAdmissionProvenance(request updateRequest, reason, reference string) error {
	if (request.AcceptNew || request.AcceptScannerChange) && (reason == "" || reference == "") {
		return errors.New("acceptance requires non-blank --reason and --reference")
	}
	if reference != "" && !isDurableReference(reference) {
		return errors.New("--reference must contain a tracker ID, PR reference, HTTPS URL, or commit ID")
	}
	return nil
}

var (
	trackerReferencePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*-[A-Za-z0-9]+(?:\.[A-Za-z0-9]+)*$`)
	prReferencePattern      = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+#[0-9]+$`)
	commitReferencePattern  = regexp.MustCompile(`^(?:[A-Za-z0-9_.-]+@)?[0-9a-fA-F]{7,64}$`)
)

func isDurableReference(reference string) bool {
	for _, field := range strings.FieldsFunc(reference, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == ';' || r == ','
	}) {
		token := strings.Trim(field, "()[]{}")
		if trackerReferencePattern.MatchString(token) ||
			prReferencePattern.MatchString(token) ||
			commitReferencePattern.MatchString(token) {
			return true
		}
		parsed, err := url.Parse(token)
		if err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.Path != "" {
			return true
		}
	}
	return false
}

func compareKeyMaps(previous, current map[string][]string) baselineChange {
	change := baselineChange{Added: emptyKeyMap(), Removed: emptyKeyMap()}
	for _, scan := range scanNames {
		previousSet := make(map[string]bool, len(previous[scan]))
		currentSet := make(map[string]bool, len(current[scan]))
		for _, key := range previous[scan] {
			previousSet[key] = true
		}
		for _, key := range current[scan] {
			currentSet[key] = true
			if !previousSet[key] {
				change.Added[scan] = append(change.Added[scan], key)
			}
		}
		for _, key := range previous[scan] {
			if !currentSet[key] {
				change.Removed[scan] = append(change.Removed[scan], key)
			}
		}
	}
	return change
}

func updateBaselineFile(
	path string,
	current map[string][]finding,
	request updateRequest,
) (baselineUpdatePlan, error) {
	previous, previousBytes, err := readBaselineFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return baselineUpdatePlan{}, err
		}
		previous = baseline{Version: baselineSchemaV1, Findings: emptyKeyMap()}
		previousBytes = nil
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(previousBytes))
	plan, err := planBaselineUpdate(previous, current, digest, request)
	if err != nil {
		return baselineUpdatePlan{}, err
	}
	if !plan.Write {
		return plan, nil
	}
	if err := writeBaselineAtomic(path, plan.Baseline); err != nil {
		return baselineUpdatePlan{}, err
	}
	return plan, nil
}

func writeBaselineAtomic(path string, bl baseline) error {
	return writeBaselineAtomicWithRename(path, bl, os.Rename)
}

func writeBaselineAtomicWithRename(path string, bl baseline, rename func(string, string) error) error {
	if bl.Version != baselineSchemaV2 {
		return fmt.Errorf("refusing to write baseline schema version %d", bl.Version)
	}
	if err := validateBaseline(bl); err != nil {
		return err
	}
	data, err := marshalBaseline(bl)
	if err != nil {
		return err
	}

	mode := os.FileMode(0o644)
	info, err := os.Stat(path)
	if err == nil {
		mode = info.Mode().Perm()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	closed := false
	defer func() {
		if !closed {
			_ = temp.Close()
		}
		if tempPath != "" {
			_ = os.Remove(tempPath)
		}
	}()

	if err := temp.Chmod(mode); err != nil {
		return err
	}
	if _, err := temp.Write(data); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		closed = true
		return err
	}
	closed = true
	if err := rename(tempPath, path); err != nil {
		return err
	}
	tempPath = ""
	return nil
}

func marshalBaseline(bl baseline) ([]byte, error) {
	data, err := json.MarshalIndent(bl, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func emptyKeyMap() map[string][]string {
	out := make(map[string][]string, len(scanNames))
	for _, scan := range scanNames {
		out[scan] = []string{}
	}
	return out
}

func keyMapCount(keysByScan map[string][]string) int {
	total := 0
	for _, keys := range keysByScan {
		total += len(keys)
	}
	return total
}

func cloneBaseline(bl baseline) baseline {
	return baseline{
		Version:        bl.Version,
		ScannerVersion: bl.ScannerVersion,
		Findings:       cloneKeyMap(bl.Findings),
		Transitions:    cloneTransitions(bl.Transitions),
	}
}

func cloneTransitions(transitions []baselineTransition) []baselineTransition {
	if transitions == nil {
		return nil
	}
	out := make([]baselineTransition, len(transitions))
	for i, transition := range transitions {
		out[i] = baselineTransition{
			PreviousBaselineSHA256: transition.PreviousBaselineSHA256,
			PreviousScannerVersion: transition.PreviousScannerVersion,
			ScannerVersion:         transition.ScannerVersion,
			Added:                  cloneKeyMap(transition.Added),
			Removed:                cloneKeyMap(transition.Removed),
			Reason:                 transition.Reason,
			Reference:              transition.Reference,
		}
	}
	return out
}

func cloneKeyMap(keysByScan map[string][]string) map[string][]string {
	if keysByScan == nil {
		return nil
	}
	out := make(map[string][]string, len(keysByScan))
	for scan, keys := range keysByScan {
		out[scan] = append([]string{}, keys...)
		if out[scan] == nil {
			out[scan] = []string{}
		}
	}
	return out
}
