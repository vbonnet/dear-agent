//go:build darwin && arm64

package buildauthority

import (
	"context"
	"encoding/binary"
	"testing"

	"golang.org/x/sys/unix"
)

func TestDarwinNullDeviceSnapshotRequiresExactPolicy(t *testing.T) {
	t.Parallel()

	valid := fileSnapshot{
		identity:  FileIdentity{UID: 0, Mode: platformModeCharacter | 0o666},
		linkCount: 1,
		rdev:      darwinNullDeviceMajor<<24 | darwinNullDeviceMinor,
	}
	if err := validateNullDeviceSnapshot(valid); err != nil {
		t.Fatalf("validate exact null device: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*fileSnapshot)
		cause  CauseCode
	}{
		{name: "regular", mutate: func(snapshot *fileSnapshot) {
			snapshot.identity.Mode = platformModeRegular | 0o666
		}, cause: CauseUnsupported},
		{name: "owner", mutate: func(snapshot *fileSnapshot) {
			snapshot.identity.UID = 1
		}, cause: CausePermission},
		{name: "owner-write-removed", mutate: func(snapshot *fileSnapshot) {
			snapshot.identity.Mode = platformModeCharacter | 0o466
		}, cause: CausePermission},
		{name: "other-execute-added", mutate: func(snapshot *fileSnapshot) {
			snapshot.identity.Mode = platformModeCharacter | 0o667
		}, cause: CausePermission},
		{name: "setgid-added", mutate: func(snapshot *fileSnapshot) {
			snapshot.identity.Mode = platformModeCharacter | 0o2666
		}, cause: CausePermission},
		{name: "second-link", mutate: func(snapshot *fileSnapshot) {
			snapshot.linkCount = 2
		}, cause: CauseIdentity},
		{name: "wrong-major", mutate: func(snapshot *fileSnapshot) {
			snapshot.rdev = 4<<24 | darwinNullDeviceMinor
		}, cause: CauseIdentity},
		{name: "wrong-minor", mutate: func(snapshot *fileSnapshot) {
			snapshot.rdev = darwinNullDeviceMajor<<24 | 3
		}, cause: CauseIdentity},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			current := valid
			test.mutate(&current)
			requirePrivateCauses(t, validateNullDeviceSnapshot(current), test.cause)
		})
	}
}

func TestDarwinDeviceNumbersUseRawKernelEncoding(t *testing.T) {
	t.Parallel()

	major, minor := darwinDeviceNumbers(0x0300_0002)
	if major != 3 || minor != 2 {
		t.Fatalf("device numbers = (%d, %d), want (3, 2)", major, minor)
	}
}

func TestDarwinNullDeviceACLReturnedAttributeProof(t *testing.T) {
	t.Parallel()

	valid := make([]byte, darwinNullACLResultSize)
	binary.LittleEndian.PutUint32(valid[0:4], darwinNullACLResultSize)
	binary.LittleEndian.PutUint32(valid[4:8], unix.ATTR_CMN_RETURNED_ATTRS)
	binary.LittleEndian.PutUint32(valid[24:28], 8)
	got, err := digestNullDeviceACLResult(valid)
	if err != nil {
		t.Fatalf("digest exact null-device ACL proof: %v", err)
	}
	want, err := digestDarwinFilesec(nil)
	if err != nil {
		t.Fatalf("digest absent ACL state: %v", err)
	}
	if got != want {
		t.Fatalf("null-device ACL digest = %x, want absent-state digest %x", got, want)
	}

	tests := []struct {
		name   string
		mutate func([]byte) []byte
		cause  CauseCode
	}{
		{name: "short", mutate: func(buffer []byte) []byte {
			return buffer[:len(buffer)-1]
		}, cause: CauseMalformed},
		{name: "reported-length", mutate: func(buffer []byte) []byte {
			binary.LittleEndian.PutUint32(buffer[0:4], darwinNullACLResultSize-1)
			return buffer
		}, cause: CauseMalformed},
		{name: "extended-security-returned", mutate: func(buffer []byte) []byte {
			binary.LittleEndian.PutUint32(
				buffer[4:8],
				unix.ATTR_CMN_RETURNED_ATTRS|unix.ATTR_CMN_EXTENDED_SECURITY,
			)
			return buffer
		}, cause: CauseUnsupported},
		{name: "volume-group-returned", mutate: func(buffer []byte) []byte {
			binary.LittleEndian.PutUint32(buffer[8:12], 1)
			return buffer
		}, cause: CauseMalformed},
		{name: "noncanonical-reference", mutate: func(buffer []byte) []byte {
			binary.LittleEndian.PutUint32(buffer[24:28], 4)
			return buffer
		}, cause: CauseMalformed},
		{name: "nonempty-default", mutate: func(buffer []byte) []byte {
			binary.LittleEndian.PutUint32(buffer[28:32], 1)
			return buffer
		}, cause: CauseMalformed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			buffer := test.mutate(append([]byte(nil), valid...))
			_, err := digestNullDeviceACLResult(buffer)
			requirePrivateCauses(t, err, test.cause)
		})
	}
}

func TestDarwinRetainsAndRevalidatesPhysicalRoot(t *testing.T) {
	t.Parallel()

	authority, err := retainPhysicalRoot(context.Background())
	if err != nil {
		t.Fatalf("retain physical root: %v", err)
	}
	defer func() {
		if err := authority.close(); err != nil {
			t.Errorf("close physical root: %v", err)
		}
	}()

	if authority.directory.path != physicalRootPath || len(authority.directory.pathClaims) != 1 {
		t.Fatalf("physical-root shape = (%q, %d claims)", authority.directory.path, len(authority.directory.pathClaims))
	}
	if err := validatePhysicalRootAuthority(authority); err != nil {
		t.Fatalf("revalidate physical root: %v", err)
	}

	// Sibling churn changes directory size and times on some filesystems. Those
	// fields are deliberately outside the retained physical-root claim.
	authority.directory.snapshot.size++
	authority.directory.snapshot.mtimeNsec++
	if err := validatePhysicalRootAuthority(authority); err != nil {
		t.Fatalf("revalidate after volatile-only fixture drift: %v", err)
	}
}

func TestDarwinRetainsAndRevalidatesNullDevice(t *testing.T) {
	t.Parallel()

	device, err := retainNullDevice(context.Background())
	if err != nil {
		t.Fatalf("retain null device: %v", err)
	}
	if device.leaf.path != nullDevicePath || len(device.leaf.pathClaims) != 2 {
		t.Fatalf("null-device shape = (%q, %d claims)", device.leaf.path, len(device.leaf.pathClaims))
	}
	if err := validateRetainedNullDevice(device); err != nil {
		t.Fatalf("revalidate null device: %v", err)
	}
	if err := device.close(); err != nil {
		t.Fatalf("close null device: %v", err)
	}
	if _, err := device.leaf.descriptor.Stat(); err == nil {
		t.Fatal("retained null descriptor remained usable after owner close")
	}
}
