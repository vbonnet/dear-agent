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

	manifestFormatDomain = "buildauthority-manifest/v1"
	domainGOROOT         = "goroot/v1"
	domainRepository     = "repository/v1"
	domainSourceObjects  = "source-object-content/v1"
	domainSourceTree     = "source-tree/v1"
	domainGOMODCACHE     = "gomodcache/v1"
	domainSelectedModule = "selected-modules/v1"

	maxExecutableBytes     = 128 << 20
	maxGOROOTEntries       = 65_536
	maxGOROOTBytes         = 2 << 30
	maxGOROOTFileBytes     = 128 << 20
	maxGOMODCACHEEntries   = 500_000
	maxGOMODCACHEBytes     = 16 << 30
	maxGOMODCACHEFileBytes = 512 << 20
	maxRepositoryEntries   = 1_000_000
	maxRepositoryBytes     = 64 << 30
	maxRepositoryFileBytes = 8 << 30
	maxSymlinkBytes        = 4 << 10
	maxRelativePathBytes   = 1_023
	maxPathComponentBytes  = 255

	goGOROOTRelativePath       = "bin/go"
	compilerGOROOTRelativePath = "pkg/tool/darwin_arm64/compile"
	goExecutableBase           = "go"
	gitExecutableBase          = "git"
)
