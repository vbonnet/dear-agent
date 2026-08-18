package override

import (
	"strings"
	"testing"
	"time"
)

func TestLaunchCapabilityCanonicalRoundTripAndExactClaim(t *testing.T) {
	now := time.Date(2026, 7, 30, 17, 0, 0, 0, time.UTC)
	claim := LaunchCapabilityClaim{
		Protocol:      "__exec-harness",
		HandoffPath:   "/tmp/agm/private-launch/launch-123.json",
		HandoffDigest: strings.Repeat("a", 64),
		OverrideProofs: []AuthorizationProof{{
			Kind:            KindAdmissionBrake,
			Reason:          "operator reviewed host recovery before this launch",
			Actor:           "dispatcher-test",
			Session:         "worker-1",
			AuthorizationID: strings.Repeat("b", 32),
		}},
		RecordSpawn: true,
		ExpiresUTC:  now.Add(5 * time.Minute),
	}
	capability := LaunchCapability{
		Version:               LaunchCapabilityVersion,
		ID:                    strings.Repeat("c", 32),
		LaunchCapabilityClaim: claim,
		IssuedUTC:             now,
	}
	encoded, err := EncodeLaunchCapability(capability)
	if err != nil {
		t.Fatalf("EncodeLaunchCapability() error: %v", err)
	}
	decoded, err := DecodeLaunchCapability(encoded)
	if err != nil {
		t.Fatalf("DecodeLaunchCapability() error: %v", err)
	}
	if err := decoded.Authorizes(claim, now.Add(time.Minute)); err != nil {
		t.Fatalf("Authorizes() exact claim error: %v", err)
	}
	mutated := claim
	mutated.HandoffDigest = strings.Repeat("d", 64)
	if err := decoded.Authorizes(mutated, now.Add(time.Minute)); err == nil {
		t.Fatal("Authorizes() accepted a modified handoff digest")
	}
	unknownProtocol := capability
	unknownProtocol.Protocol = "__exec-attacker"
	if _, err := EncodeLaunchCapability(unknownProtocol); err == nil {
		t.Fatal("EncodeLaunchCapability() accepted an unknown private protocol")
	}
}

func TestPrivilegedLaunchCapabilityRequestIsCanonicalAndLauncherBound(t *testing.T) {
	now := time.Date(2026, 7, 30, 17, 0, 0, 0, time.UTC)
	capability := LaunchCapability{
		Version: LaunchCapabilityVersion,
		ID:      strings.Repeat("e", 32),
		LaunchCapabilityClaim: LaunchCapabilityClaim{
			Protocol:      "__exec-claude",
			HandoffPath:   "/tmp/agm/private-launch/launch-456.json",
			HandoffDigest: strings.Repeat("f", 64),
			OverrideProofs: []AuthorizationProof{{
				Kind:            KindSupervisorOAuthCheck,
				Reason:          "development supervisor has no stored OAuth credentials",
				Actor:           "operator-test",
				Session:         "supervisor-s1",
				AuthorizationID: strings.Repeat("1", 32),
			}},
			ExpiresUTC: now.Add(5 * time.Minute),
		},
		IssuedUTC: now,
	}
	encoded, err := EncodePrivilegedLaunchCapabilityRequest(capability, 4242)
	if err != nil {
		t.Fatalf("EncodePrivilegedLaunchCapabilityRequest() error: %v", err)
	}
	if operation, err := PrivilegedRequestOperation(encoded); err != nil ||
		operation != PrivilegedLaunchCapabilityOperation {
		t.Fatalf("PrivilegedRequestOperation() = %q, %v", operation, err)
	}
	decoded, launcherPID, err := DecodePrivilegedLaunchCapabilityRequest(encoded)
	if err != nil {
		t.Fatalf("DecodePrivilegedLaunchCapabilityRequest() error: %v", err)
	}
	if launcherPID != 4242 || decoded.ID != capability.ID {
		t.Fatalf("decoded capability = %+v, launcher PID %d", decoded, launcherPID)
	}
	tampered := append([]byte(nil), encoded...)
	tampered[len(tampered)-2] = ' '
	if _, _, err := DecodePrivilegedLaunchCapabilityRequest(tampered); err == nil {
		t.Fatal("DecodePrivilegedLaunchCapabilityRequest() accepted tampering")
	}

	consume, err := EncodePrivilegedConsumeLaunchCapabilityRequest(capability, 4242)
	if err != nil {
		t.Fatalf("EncodePrivilegedConsumeLaunchCapabilityRequest() error: %v", err)
	}
	if operation, err := PrivilegedRequestOperation(consume); err != nil ||
		operation != PrivilegedConsumeLaunchCapabilityOperation {
		t.Fatalf("consume PrivilegedRequestOperation() = %q, %v", operation, err)
	}
	if _, _, err := DecodePrivilegedConsumeLaunchCapabilityRequest(consume); err != nil {
		t.Fatalf("DecodePrivilegedConsumeLaunchCapabilityRequest() error: %v", err)
	}
	if _, _, err := DecodePrivilegedLaunchCapabilityRequest(consume); err == nil {
		t.Fatal("issue decoder accepted a consume request")
	}
}
