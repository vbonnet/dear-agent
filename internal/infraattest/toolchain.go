package infraattest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
)

type toolchainManifest struct {
	Schema                   string `json:"schema"`
	DependencyLockfileSHA256 string `json:"dependency_lockfile_sha256"`
	OpenTofu                 struct {
		SourceRepository      string `json:"source_repository"`
		Version               string `json:"version"`
		Tag                   string `json:"tag"`
		TagCommit             string `json:"tag_commit"`
		ChecksumsURL          string `json:"checksums_url"`
		ChecksumsSHA256       string `json:"checksums_sha256"`
		SigningKeyFingerprint string `json:"signing_key_fingerprint"`
	} `json:"opentofu"`
	Provider struct {
		SourceRepository      string `json:"source_repository"`
		Address               string `json:"address"`
		Version               string `json:"version"`
		Tag                   string `json:"tag"`
		TagCommit             string `json:"tag_commit"`
		ChecksumsURL          string `json:"checksums_url"`
		ChecksumsSHA256       string `json:"checksums_sha256"`
		SigningKeyFingerprint string `json:"signing_key_fingerprint"`
	} `json:"provider"`
	Platforms []platformLock `json:"platforms"`
}

type platformLock struct {
	Platform              string `json:"platform"`
	OpenTofuArchive       string `json:"opentofu_archive"`
	OpenTofuArchiveSHA256 string `json:"opentofu_archive_sha256"`
	OpenTofuBinarySHA256  string `json:"opentofu_binary_sha256"`
	ProviderArchive       string `json:"provider_archive"`
	ProviderArchiveSHA256 string `json:"provider_archive_sha256"`
	ProviderBinarySHA256  string `json:"provider_binary_sha256"`
}

var lockFieldPattern = regexp.MustCompile(`(?m)^\s*(version|constraints)\s*=\s*"([^"]+)"\s*$`)

var lockHashPattern = regexp.MustCompile(`"(zh|h1):([^"]*)"`)

var officialPlatformLocks = map[string]platformLock{
	"darwin_arm64": {
		Platform: "darwin_arm64", OpenTofuArchive: "tofu_1.12.5_darwin_arm64.zip",
		OpenTofuArchiveSHA256: "dbb5a5bae9b0cabf622cd81a80ea02230eae8a3813215400df41a2cb89b47157",
		OpenTofuBinarySHA256:  "96557429623614140cf41afeb147b8a7e1fbe53e55923b63e7b581bc608d60ca",
		ProviderArchive:       "terraform-provider-github_6.13.0_darwin_arm64.zip",
		ProviderArchiveSHA256: "c26a9bca4865665084e7f59b1402d7aff34ee63a418d7401a0658fa280cad4d4",
		ProviderBinarySHA256:  "b7e4601361cdd0afdcc83d9dfdc4a274dea693af291339c2bc9a915ec4ba62b6",
	},
	"linux_amd64": {
		Platform: "linux_amd64", OpenTofuArchive: "tofu_1.12.5_linux_amd64.zip",
		OpenTofuArchiveSHA256: "dade9650e6b74fc7a8b986bd8717497d32f9e09cf82e479afef4977fa3085536",
		OpenTofuBinarySHA256:  "36dae7ca1e4f1552a6faef27179dc16ef403203e956f31416c17b3d87a38c3f4",
		ProviderArchive:       "terraform-provider-github_6.13.0_linux_amd64.zip",
		ProviderArchiveSHA256: "5dd05dee677f6ebdbed00cbb1b9be444ab2d1062d345cbc9ec50a47cb41b8622",
		ProviderBinarySHA256:  "8b50ca47bbf54d9c3471326bc15bfa2fab748275180d20915e34cf93169d38bc",
	},
	"linux_arm64": {
		Platform: "linux_arm64", OpenTofuArchive: "tofu_1.12.5_linux_arm64.zip",
		OpenTofuArchiveSHA256: "528f4eea63452bbddb30fa4f1780b57fac8d7676f9dda0f772e847bb62c1260a",
		OpenTofuBinarySHA256:  "e4035d38b5b95fc1490c3205ad798bc77b25c6b93049a279fded4883d020420c",
		ProviderArchive:       "terraform-provider-github_6.13.0_linux_arm64.zip",
		ProviderArchiveSHA256: "0ab29fc21699f34345cf0bbbe44745fd1b143b7c73b410c1dc4abe05ffad0a84",
		ProviderBinarySHA256:  "35e6f87007ce6b686b2f327ba7597a0abd5553ef17435dc8094065bbf39e45a2",
	},
}

func evaluateToolchain(request AuthorizationRequest) (ToolchainClaims, error) {
	manifestRaw, err := readBounded(request.ToolchainManifest, MaxToolchainLockBytes)
	if err != nil {
		return ToolchainClaims{}, err
	}
	if digest(manifestRaw) != ToolchainManifestSHA256 {
		return ToolchainClaims{}, reject(CodeUnsupportedToolchain)
	}
	var manifest toolchainManifest
	if _, err := decodeStrict(manifestRaw, &manifest); err != nil {
		return ToolchainClaims{}, reject(CodeUnsupportedToolchain)
	}
	if err := validateManifest(manifest); err != nil {
		return ToolchainClaims{}, err
	}
	selected, ok := selectPlatform(manifest.Platforms, request.Platform)
	if !ok {
		return ToolchainClaims{}, reject(CodeUnsupportedToolchain)
	}

	tofuDigest, err := hashBounded(request.OpenTofuBinary, MaxToolBinaryBytes)
	if err != nil {
		return ToolchainClaims{}, err
	}
	providerDigest, err := hashBounded(request.ProviderBinary, MaxProviderBinaryBytes)
	if err != nil {
		return ToolchainClaims{}, err
	}
	if tofuDigest != selected.OpenTofuBinarySHA256 || providerDigest != selected.ProviderBinarySHA256 {
		return ToolchainClaims{}, reject(CodeUnsupportedToolchain)
	}

	lockRaw, err := readBounded(request.DependencyLockfile, MaxToolchainLockBytes)
	if err != nil {
		return ToolchainClaims{}, err
	}
	if err := validateDependencyLock(lockRaw, selected.ProviderArchiveSHA256); err != nil {
		return ToolchainClaims{}, err
	}

	return ToolchainClaims{
		Platform:                 selected.Platform,
		OpenTofuVersion:          OpenTofuVersion,
		OpenTofuTagCommit:        OpenTofuTagCommit,
		OpenTofuArchiveSHA256:    selected.OpenTofuArchiveSHA256,
		OpenTofuBinarySHA256:     tofuDigest,
		ToolchainManifestSHA256:  ToolchainManifestSHA256,
		DependencyLockfileSHA256: digest(lockRaw),
		Providers: []ProviderClaims{{
			Address:       ProviderAddress,
			Version:       ProviderVersion,
			TagCommit:     ProviderTagCommit,
			ArchiveSHA256: selected.ProviderArchiveSHA256,
			BinarySHA256:  providerDigest,
		}},
	}, nil
}

func validateManifest(manifest toolchainManifest) error {
	if manifest.Schema != ToolchainSchema || manifest.DependencyLockfileSHA256 != DependencyLockSHA256 ||
		!validOpenTofuManifest(manifest) || !validProviderManifest(manifest) || len(manifest.Platforms) != 3 {
		return reject(CodeUnsupportedToolchain)
	}
	return validatePlatformLocks(manifest.Platforms)
}

func validOpenTofuManifest(manifest toolchainManifest) bool {
	return manifest.OpenTofu.SourceRepository == "https://github.com/opentofu/opentofu" &&
		manifest.OpenTofu.Version == OpenTofuVersion && manifest.OpenTofu.Tag == "v"+OpenTofuVersion &&
		manifest.OpenTofu.TagCommit == OpenTofuTagCommit &&
		manifest.OpenTofu.ChecksumsURL == "https://github.com/opentofu/opentofu/releases/download/v1.12.5/tofu_1.12.5_SHA256SUMS" &&
		manifest.OpenTofu.ChecksumsSHA256 == "120345f8a2493375aebbca072106de425b2eb227837f8064440b8d911e36f987" &&
		manifest.OpenTofu.SigningKeyFingerprint == "E3E6E43D84CB852EADB0051D0C0AF313E5FD9F80"
}

func validProviderManifest(manifest toolchainManifest) bool {
	return manifest.Provider.SourceRepository == "https://github.com/integrations/terraform-provider-github" &&
		manifest.Provider.Address == ProviderAddress && manifest.Provider.Version == ProviderVersion &&
		manifest.Provider.Tag == "v"+ProviderVersion && manifest.Provider.TagCommit == ProviderTagCommit &&
		manifest.Provider.ChecksumsURL == "https://github.com/integrations/terraform-provider-github/releases/download/v6.13.0/terraform-provider-github_6.13.0_SHA256SUMS" &&
		manifest.Provider.ChecksumsSHA256 == "2d688e8383ff669297bbb6461f7eb05168f53fe76d3233fdb431e318efedb98f" &&
		manifest.Provider.SigningKeyFingerprint == "F31928FACE52F1A13A6C60EA38027F80D7FD5FB2"
}

func validatePlatformLocks(platforms []platformLock) error {
	wantPlatforms := []string{"darwin_arm64", "linux_amd64", "linux_arm64"}
	seen := make([]string, 0, len(platforms))
	for _, platform := range platforms {
		official, known := officialPlatformLocks[platform.Platform]
		if platform.Platform == "" || platform.OpenTofuArchive != fmt.Sprintf("tofu_%s_%s.zip", OpenTofuVersion, platform.Platform) ||
			platform.ProviderArchive != fmt.Sprintf("terraform-provider-github_%s_%s.zip", ProviderVersion, platform.Platform) ||
			!validSHA256(platform.OpenTofuArchiveSHA256) || !validSHA256(platform.OpenTofuBinarySHA256) ||
			!validSHA256(platform.ProviderArchiveSHA256) || !validSHA256(platform.ProviderBinarySHA256) ||
			!known || platform != official {
			return reject(CodeUnsupportedToolchain)
		}
		seen = append(seen, platform.Platform)
	}
	sort.Strings(seen)
	if !equalStrings(seen, wantPlatforms) {
		return reject(CodeUnsupportedToolchain)
	}
	return nil
}

func selectPlatform(platforms []platformLock, name string) (platformLock, bool) {
	for _, platform := range platforms {
		if platform.Platform == name {
			return platform, true
		}
	}
	return platformLock{}, false
}

func validateDependencyLock(raw []byte, providerArchiveSHA256 string) error {
	if !validSHA256(providerArchiveSHA256) {
		return reject(CodeMalformedLockfile)
	}
	if len(raw) == 0 || bytes.IndexByte(raw, 0) >= 0 ||
		digest(raw) != DependencyLockSHA256 ||
		strings.Count(string(raw), `provider "`) != 1 ||
		!strings.Contains(string(raw), `provider "`+ProviderAddress+`" {`) {
		return reject(CodeMalformedLockfile)
	}
	fields, err := lockFields(raw)
	if err != nil {
		return err
	}
	archives, moduleHashes := lockHashes(raw)
	if fields["version"] != ProviderVersion || fields["constraints"] != ProviderVersion ||
		!slices.Contains(archives, providerArchiveSHA256) || len(archives) < 3 || moduleHashes < 1 {
		return reject(CodeMalformedLockfile)
	}
	return nil
}

func lockFields(raw []byte) (map[string]string, error) {
	fields := make(map[string]string)
	for _, match := range lockFieldPattern.FindAllSubmatch(raw, -1) {
		key, value := string(match[1]), string(match[2])
		if _, duplicate := fields[key]; duplicate {
			return nil, reject(CodeMalformedLockfile)
		}
		fields[key] = value
	}
	return fields, nil
}

// lockHashes returns the recorded provider archive digests and the count of
// module hashes. Callers compare digests as values: interpolating a
// manifest-supplied digest into a quoted probe would let an embedded quote
// change the shape of the pattern being matched.
func lockHashes(raw []byte) (archives []string, moduleHashes int) {
	for _, match := range lockHashPattern.FindAllSubmatch(raw, -1) {
		if string(match[1]) == "zh" {
			archives = append(archives, string(match[2]))
			continue
		}
		moduleHashes++
	}
	return archives, moduleHashes
}

func equalStrings(left, right []string) bool {
	return slices.Equal(left, right)
}

func canonicalStruct(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, reject(CodeInvalidInput)
	}
	return raw, nil
}
