package buildauthority

import (
	"context"
	"errors"
	"os"
	"testing"
)

func TestPhysicalRootSnapshotRequiresExactPolicy(t *testing.T) {
	t.Parallel()

	valid := fileSnapshot{identity: FileIdentity{UID: 0, Mode: platformModeDirectory | 0o755}}
	if err := validatePhysicalRootSnapshot(valid); err != nil {
		t.Fatalf("validate exact physical root: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*fileSnapshot)
		cause  CauseCode
	}{
		{name: "regular", mutate: func(snapshot *fileSnapshot) {
			snapshot.identity.Mode = platformModeRegular | 0o755
		}, cause: CauseUnsupported},
		{name: "owner", mutate: func(snapshot *fileSnapshot) {
			snapshot.identity.UID = 1
		}, cause: CausePermission},
		{name: "owner-write-removed", mutate: func(snapshot *fileSnapshot) {
			snapshot.identity.Mode = platformModeDirectory | 0o655
		}, cause: CausePermission},
		{name: "group-write-added", mutate: func(snapshot *fileSnapshot) {
			snapshot.identity.Mode = platformModeDirectory | 0o775
		}, cause: CausePermission},
		{name: "setuid-added", mutate: func(snapshot *fileSnapshot) {
			snapshot.identity.Mode = platformModeDirectory | 0o4755
		}, cause: CausePermission},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			current := valid
			test.mutate(&current)
			requirePrivateCauses(t, validatePhysicalRootSnapshot(current), test.cause)
		})
	}
}

func TestSingleAuthorityClaimDistinguishesIdentityAndSecurityDrift(t *testing.T) {
	t.Parallel()

	expected := authorityPathClaim{
		identity: FileIdentity{Device: 1, Inode: 2, UID: 0, Mode: platformModeDirectory | 0o755},
		gid:      3,
	}
	if err := compareSingleAuthorityClaim(expected, expected, "fixture"); err != nil {
		t.Fatalf("compare equal claim: %v", err)
	}

	identityDrift := expected
	identityDrift.identity.Inode++
	requirePrivateCauses(
		t,
		compareSingleAuthorityClaim(expected, identityDrift, "fixture"),
		CauseIdentity,
	)

	securityDrift := expected
	securityDrift.gid++
	requirePrivateCauses(
		t,
		compareSingleAuthorityClaim(expected, securityDrift, "fixture"),
		CauseUnstable,
	)
}

func TestNullDeviceParentTransitionRequiresSameMount(t *testing.T) {
	t.Parallel()

	mount := mountSnapshot{filesystem: [2]int32{1, 2}, flags: 3}
	snapshot := fileSnapshot{identity: FileIdentity{
		Device:     4,
		Filesystem: mount.filesystem,
	}}
	claims := []authorityPathClaim{{
		identity: FileIdentity{Device: snapshot.identity.Device, Filesystem: mount.filesystem},
		mount:    mount,
	}}
	if err := validateNullDeviceParentTransition(snapshot, mount, claims); err != nil {
		t.Fatalf("validate same parent mount: %v", err)
	}
	requirePrivateCauses(
		t,
		validateNullDeviceParentTransition(snapshot, mount, nil),
		CauseInternalInvariant,
	)

	tests := []struct {
		name   string
		mutate func(*fileSnapshot, *mountSnapshot)
	}{
		{name: "device", mutate: func(snapshot *fileSnapshot, _ *mountSnapshot) {
			snapshot.identity.Device++
		}},
		{name: "filesystem", mutate: func(snapshot *fileSnapshot, _ *mountSnapshot) {
			snapshot.identity.Filesystem[0]++
		}},
		{name: "mount-flags", mutate: func(_ *fileSnapshot, mount *mountSnapshot) {
			mount.flags++
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			currentSnapshot := snapshot
			currentMount := mount
			test.mutate(&currentSnapshot, &currentMount)
			requirePrivateCauses(
				t,
				validateNullDeviceParentTransition(currentSnapshot, currentMount, claims),
				CauseUnsupported,
			)
		})
	}
}

func TestNullDeviceInternalOpenSeamCannotReturnMissingParent(t *testing.T) {
	t.Parallel()

	inspect := func(context.Context, *os.File) (fileSnapshot, mountSnapshot, Digest, error) {
		return fileSnapshot{}, mountSnapshot{}, Digest{}, nil
	}
	openPath := func(
		context.Context,
		string,
		descriptorInspectFunc,
	) (*os.File, []authorityPathClaim, error) {
		return nil, nil, nil
	}
	openNull := func(int) (*os.File, error) {
		t.Fatal("null opener called without a parent")
		return nil, nil
	}
	_, err := retainNullDeviceContextWith(
		context.Background(),
		inspect,
		inspect,
		openPath,
		openNull,
	)
	requirePrivateCauses(t, err, CauseInternalInvariant)
}

func TestPhysicalRootClosesInReverseAcquisitionOrder(t *testing.T) {
	t.Parallel()

	events := make([]string, 0, 2)
	descriptorFailure := errors.New("descriptor close")
	rootFailure := errors.New("root close")
	descriptor := &recordingCloser{name: "descriptor", events: &events, err: descriptorFailure}
	root := &recordingCloser{name: "root", events: &events, err: rootFailure}

	err := closePhysicalRootHandles(descriptor, root)
	if len(events) != 2 || events[0] != "descriptor" || events[1] != "root" {
		t.Fatalf("close order = %#v, want [descriptor root]", events)
	}
	if !errors.Is(err, descriptorFailure) || !errors.Is(err, rootFailure) {
		t.Fatalf("close error = %v, want both failures", err)
	}
}

type recordingCloser struct {
	name   string
	events *[]string
	err    error
}

func (closer *recordingCloser) Close() error {
	*closer.events = append(*closer.events, closer.name)
	return closer.err
}
