package override

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"time"
)

const (
	// LaunchCapabilityVersion is the root-attested handoff capability format.
	LaunchCapabilityVersion = 1
	// PrivilegedLaunchCapabilityOperation distinguishes capability issuance
	// from the existing append-only ledger request on the fixed helper stdin.
	PrivilegedLaunchCapabilityOperation = "issue_launch_capability"
	// PrivilegedConsumeLaunchCapabilityOperation atomically consumes one
	// previously issued capability before its handoff may commit effects.
	PrivilegedConsumeLaunchCapabilityOperation = "consume_launch_capability"
	// MaxPrivilegedLaunchCapabilityBytes bounds one helper request.
	MaxPrivilegedLaunchCapabilityBytes = 16 << 10
	// MaxLaunchCapabilityBytes bounds one root-owned sidecar.
	MaxLaunchCapabilityBytes = 8 << 10
	maxLaunchCapabilityAge   = 10 * time.Minute
	launchCapabilityDirPath  = "/var/run/dear-agent-launch-capabilities"
)

// LaunchCapabilityClaim is the exact non-secret launch state attested by the
// privileged helper before a private executor may accept override claims.
type LaunchCapabilityClaim struct {
	Protocol       string               `json:"protocol"`
	HandoffPath    string               `json:"handoff_path"`
	HandoffDigest  string               `json:"handoff_digest"`
	OverrideProofs []AuthorizationProof `json:"override_proofs,omitempty"`
	RecordSpawn    bool                 `json:"record_spawn,omitempty"`
	ExpiresUTC     time.Time            `json:"expires_utc"`
}

// LaunchCapability is a short-lived, root-attested parent-issued capability.
// Its contents are non-secret; authenticity comes from the root-owned sidecar.
type LaunchCapability struct {
	Version int    `json:"version"`
	ID      string `json:"id"`
	LaunchCapabilityClaim
	IssuedUTC time.Time `json:"issued_utc"`
}

type privilegedLaunchCapabilityRequest struct {
	Version     int              `json:"version"`
	Operation   string           `json:"operation"`
	LauncherPID int              `json:"launcher_pid"`
	Capability  LaunchCapability `json:"capability"`
}

// LaunchCapabilityDir is the fixed root-owned sidecar directory.
func LaunchCapabilityDir() string { return launchCapabilityDirPath }

// LaunchCapabilityPath returns the fixed sidecar path for a validated ID.
func LaunchCapabilityPath(id string) (string, error) {
	if !authorizationIDRE.MatchString(id) {
		return "", errors.New("launch capability ID is not a lowercase 128-bit value")
	}
	return filepath.Join(LaunchCapabilityDir(), id+".json"), nil
}

// IssueLaunchCapability asks the fixed privileged helper to attest an exact
// launch claim. The sidecar is intentionally separate from user-writable
// handoff storage.
func IssueLaunchCapability(claim LaunchCapabilityClaim) (LaunchCapability, error) {
	now := time.Now().UTC()
	id, err := newAuthorizationID()
	if err != nil {
		return LaunchCapability{}, err
	}
	capability := LaunchCapability{
		Version:               LaunchCapabilityVersion,
		ID:                    id,
		LaunchCapabilityClaim: cloneLaunchCapabilityClaim(claim),
		IssuedUTC:             now,
	}
	if err := capability.Validate(now); err != nil {
		return LaunchCapability{}, err
	}
	if err := issueLaunchCapability(capability); err != nil {
		return LaunchCapability{}, err
	}
	return capability, nil
}

// Validate checks the canonical capability fields and bounded lifetime.
func (c LaunchCapability) Validate(now time.Time) error {
	if c.Version != LaunchCapabilityVersion {
		return fmt.Errorf("unsupported launch capability version %d", c.Version)
	}
	if _, err := LaunchCapabilityPath(c.ID); err != nil {
		return err
	}
	if err := validateLaunchCapabilityClaim(c.LaunchCapabilityClaim); err != nil {
		return err
	}
	if c.IssuedUTC.IsZero() ||
		c.ExpiresUTC.Before(c.IssuedUTC) ||
		c.ExpiresUTC.Sub(c.IssuedUTC) > maxLaunchCapabilityAge {
		return errors.New("launch capability has an invalid lifetime")
	}
	if now.Before(c.IssuedUTC.Add(-time.Minute)) || !now.Before(c.ExpiresUTC) {
		return errors.New("launch capability is not currently valid")
	}
	return nil
}

// Authorizes requires exact equality with the executor-derived handoff claim.
func (c LaunchCapability) Authorizes(claim LaunchCapabilityClaim, now time.Time) error {
	if err := c.Validate(now); err != nil {
		return err
	}
	if c.Protocol != claim.Protocol ||
		c.HandoffPath != claim.HandoffPath ||
		c.HandoffDigest != claim.HandoffDigest ||
		c.RecordSpawn != claim.RecordSpawn ||
		!c.ExpiresUTC.Equal(claim.ExpiresUTC) ||
		!slices.Equal(c.OverrideProofs, claim.OverrideProofs) {
		return errors.New("root-attested launch capability does not match the private handoff")
	}
	return nil
}

// EncodeLaunchCapability returns the canonical sidecar representation.
func EncodeLaunchCapability(capability LaunchCapability) ([]byte, error) {
	if err := capability.Validate(capability.IssuedUTC); err != nil {
		return nil, err
	}
	data, err := json.Marshal(capability)
	if err != nil {
		return nil, fmt.Errorf("encode launch capability: %w", err)
	}
	data = append(data, '\n')
	if len(data) > MaxLaunchCapabilityBytes {
		return nil, fmt.Errorf("launch capability has %d bytes, maximum is %d",
			len(data), MaxLaunchCapabilityBytes)
	}
	return data, nil
}

// DecodeLaunchCapability accepts only the canonical sidecar representation.
func DecodeLaunchCapability(data []byte) (LaunchCapability, error) {
	var capability LaunchCapability
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&capability); err != nil {
		return LaunchCapability{}, err
	}
	if err := requireLedgerEOF(decoder); err != nil {
		return LaunchCapability{}, err
	}
	canonical, err := EncodeLaunchCapability(capability)
	if err != nil {
		return LaunchCapability{}, err
	}
	if !bytes.Equal(data, canonical) {
		return LaunchCapability{}, errors.New("launch capability is not canonical")
	}
	return capability, nil
}

// EncodePrivilegedLaunchCapabilityRequest binds a capability to the live AGM
// or separately attested co-installed companion PID that invokes the helper.
func EncodePrivilegedLaunchCapabilityRequest(
	capability LaunchCapability,
	launcherPID int,
) ([]byte, error) {
	return encodePrivilegedLaunchCapabilityRequest(
		capability,
		launcherPID,
		PrivilegedLaunchCapabilityOperation,
	)
}

// EncodePrivilegedConsumeLaunchCapabilityRequest binds a one-shot consume
// request to the live AGM PID that invokes the fixed root-owned helper.
func EncodePrivilegedConsumeLaunchCapabilityRequest(
	capability LaunchCapability,
	launcherPID int,
) ([]byte, error) {
	return encodePrivilegedLaunchCapabilityRequest(
		capability,
		launcherPID,
		PrivilegedConsumeLaunchCapabilityOperation,
	)
}

func encodePrivilegedLaunchCapabilityRequest(
	capability LaunchCapability,
	launcherPID int,
	operation string,
) ([]byte, error) {
	if launcherPID <= 1 {
		return nil, errors.New("privileged launch capability launcher PID is invalid")
	}
	if err := capability.Validate(capability.IssuedUTC); err != nil {
		return nil, err
	}
	request := privilegedLaunchCapabilityRequest{
		Version:     LaunchCapabilityVersion,
		Operation:   operation,
		LauncherPID: launcherPID,
		Capability:  capability,
	}
	data, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode privileged launch capability request: %w", err)
	}
	data = append(data, '\n')
	if len(data) > MaxPrivilegedLaunchCapabilityBytes {
		return nil, fmt.Errorf("privileged launch capability request has %d bytes, maximum is %d",
			len(data), MaxPrivilegedLaunchCapabilityBytes)
	}
	return data, nil
}

// DecodePrivilegedLaunchCapabilityRequest accepts only the canonical helper
// request produced by EncodePrivilegedLaunchCapabilityRequest.
func DecodePrivilegedLaunchCapabilityRequest(
	data []byte,
) (LaunchCapability, int, error) {
	return decodePrivilegedLaunchCapabilityRequest(
		data,
		PrivilegedLaunchCapabilityOperation,
	)
}

// DecodePrivilegedConsumeLaunchCapabilityRequest accepts only the canonical
// one-shot consume request produced by its matching encoder.
func DecodePrivilegedConsumeLaunchCapabilityRequest(
	data []byte,
) (LaunchCapability, int, error) {
	return decodePrivilegedLaunchCapabilityRequest(
		data,
		PrivilegedConsumeLaunchCapabilityOperation,
	)
}

func decodePrivilegedLaunchCapabilityRequest(
	data []byte,
	operation string,
) (LaunchCapability, int, error) {
	var request privilegedLaunchCapabilityRequest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return LaunchCapability{}, 0, err
	}
	if err := requireLedgerEOF(decoder); err != nil {
		return LaunchCapability{}, 0, err
	}
	if request.Version != LaunchCapabilityVersion ||
		request.Operation != operation {
		return LaunchCapability{}, 0, errors.New("unsupported privileged launch capability request")
	}
	canonical, err := encodePrivilegedLaunchCapabilityRequest(
		request.Capability,
		request.LauncherPID,
		operation,
	)
	if err != nil {
		return LaunchCapability{}, 0, err
	}
	if !bytes.Equal(data, canonical) {
		return LaunchCapability{}, 0, errors.New("privileged launch capability request is not canonical")
	}
	return request.Capability, request.LauncherPID, nil
}

// PrivilegedRequestOperation reads only the dispatch marker. The operation-
// specific decoder remains responsible for strict canonical validation.
func PrivilegedRequestOperation(data []byte) (string, error) {
	var marker struct {
		Operation string `json:"operation"`
	}
	if err := json.Unmarshal(data, &marker); err != nil {
		return "", err
	}
	return marker.Operation, nil
}

func validateLaunchCapabilityClaim(claim LaunchCapabilityClaim) error {
	if err := validateLaunchCapabilityTarget(claim); err != nil {
		return err
	}
	if len(claim.OverrideProofs) == 0 {
		return errors.New("launch capability carries no override proof")
	}
	if len(claim.OverrideProofs) > MaxLedgerUsesPerTransaction {
		return errors.New("launch capability carries too many override proofs")
	}
	seen := make(map[Kind]struct{}, len(claim.OverrideProofs))
	for _, proof := range claim.OverrideProofs {
		if _, duplicate := seen[proof.Kind]; duplicate {
			return fmt.Errorf("launch capability repeats override kind %q", proof.Kind)
		}
		seen[proof.Kind] = struct{}{}
		if err := validateLaunchCapabilityProof(proof); err != nil {
			return err
		}
	}
	if claim.ExpiresUTC.IsZero() {
		return errors.New("launch capability expiry is required")
	}
	return nil
}

func validateLaunchCapabilityTarget(claim LaunchCapabilityClaim) error {
	switch claim.Protocol {
	case "__exec-codex", "__exec-claude", "__exec-harness":
	default:
		return errors.New("launch capability protocol is invalid")
	}
	if !filepath.IsAbs(claim.HandoffPath) ||
		filepath.Clean(claim.HandoffPath) != claim.HandoffPath {
		return errors.New("launch capability handoff path is not a clean absolute path")
	}
	if !fullSHA256.MatchString(claim.HandoffDigest) {
		return errors.New("launch capability handoff digest is not a lowercase SHA-256 value")
	}
	return nil
}

func validateLaunchCapabilityProof(proof AuthorizationProof) error {
	if !proof.Kind.Valid() ||
		proof.Actor == "" ||
		proof.AuthorizationID == "" ||
		!authorizationIDRE.MatchString(proof.AuthorizationID) {
		return errors.New("launch capability contains an incomplete override proof")
	}
	reason, err := ValidateReason(proof.Reason)
	if err != nil || reason != proof.Reason {
		return errors.New("launch capability contains an invalid override reason")
	}
	return validateLedgerSubject(proof.Kind, proof.Subject)
}

func cloneLaunchCapabilityClaim(claim LaunchCapabilityClaim) LaunchCapabilityClaim {
	claim.OverrideProofs = append([]AuthorizationProof(nil), claim.OverrideProofs...)
	return claim
}
