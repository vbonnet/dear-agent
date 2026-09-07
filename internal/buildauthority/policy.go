package buildauthority

const (
	// AuthorityVersion identifies the complete staging policy represented by a
	// Receipt. Changing an admitted authority, command, or output contract
	// requires a new version.
	AuthorityVersion = "sandbox-gc-build-authority/v1"
	// PlatformDarwinARM64 is the only platform admitted by this authority.
	PlatformDarwinARM64 = "darwin/arm64"

	agmOutputName           = "agm"
	agmMainPackage          = "github.com/vbonnet/dear-agent/agm/cmd/agm"
	diskWatchdogOutputName  = "disk-watchdog"
	diskWatchdogMainPackage = "github.com/vbonnet/dear-agent/cmd/disk-watchdog"
)
