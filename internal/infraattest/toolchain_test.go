package infraattest

import (
	"bytes"
	"crypto/sha256"
	"debug/elf"
	"debug/macho"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestRepositoryToolchainContractIsExactAndInternallyConsistent(t *testing.T) {
	infraDir := filepath.Join("..", "..", "infra")
	manifestRaw := readTestFile(t, filepath.Join(infraDir, "toolchain.lock.json"))
	if digest(manifestRaw) != ToolchainManifestSHA256 {
		t.Fatal("toolchain manifest byte identity does not match the compiled lock")
	}
	var manifest toolchainManifest
	if _, err := decodeStrict(manifestRaw, &manifest); err != nil {
		t.Fatal(err)
	}
	if err := validateManifest(manifest); err != nil {
		t.Fatal(err)
	}
	lockRaw := readTestFile(t, filepath.Join(infraDir, ".terraform.lock.hcl"))
	for _, platform := range manifest.Platforms {
		if err := validateDependencyLock(lockRaw, platform.ProviderArchiveSHA256); err != nil {
			t.Fatalf("lock does not authenticate %s: %v", platform.Platform, err)
		}
	}
	if got := strings.TrimSpace(string(readTestFile(t, filepath.Join(infraDir, ".opentofu-version")))); got != OpenTofuVersion {
		t.Fatalf(".opentofu-version = %q", got)
	}
	for _, path := range []string{
		filepath.Join(infraDir, "providers.tf"),
		filepath.Join(infraDir, "modules", "managed-repo", "versions.tf"),
	} {
		contents := string(readTestFile(t, path))
		if !strings.Contains(contents, `required_version = "= `+OpenTofuVersion+`"`) ||
			!strings.Contains(contents, `version = "= `+ProviderVersion+`"`) ||
			strings.Contains(contents, `required_version = ">=`) || strings.Contains(contents, `version = "~>`) {
			t.Fatalf("%s does not use exact tool/provider pins", path)
		}
	}
	encryption := string(readTestFile(t, filepath.Join(infraDir, "encryption.tf")))
	if !strings.Contains(encryption, "plan {") || !strings.Contains(encryption, "enforced = true") ||
		strings.Contains(encryption, "state {") {
		t.Fatal("encryption.tf must enforce plan encryption without migrating state")
	}
	if strings.Contains(string(readTestFile(t, filepath.Join(infraDir, ".gitignore"))), ".terraform.lock.hcl") {
		t.Fatal("dependency lockfile is still ignored")
	}
}

func TestToolchainManifestAndDependencyLockRejectTampering(t *testing.T) {
	manifestRaw := readTestFile(t, filepath.Join("..", "..", "infra", "toolchain.lock.json"))
	_, err := evaluateToolchain(AuthorizationRequest{
		ToolchainManifest: bytes.NewReader(append(append([]byte(nil), manifestRaw...), '\n')),
	})
	assertCode(t, err, CodeUnsupportedToolchain)

	var manifest toolchainManifest
	if _, err := decodeStrict(manifestRaw, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Platforms[0].OpenTofuBinarySHA256 = strings.Repeat("f", 64)
	if err := validateManifest(manifest); err == nil {
		t.Fatal("tampered manifest was accepted")
	}

	lockRaw := readTestFile(t, filepath.Join("..", "..", "infra", ".terraform.lock.hcl"))
	tests := [][]byte{
		bytes.Replace(lockRaw, []byte(`constraints = "6.13.0"`), []byte(`constraints = "~> 6.0"`), 1),
		bytes.Replace(lockRaw, []byte(`version     = "6.13.0"`), []byte(`version     = "6.12.0"`), 1),
		append(append([]byte(nil), lockRaw...), []byte(`\nprovider "registry.opentofu.org/other/private" {}\n`)...),
	}
	for _, raw := range tests {
		if err := validateDependencyLock(raw, officialPlatformLocks["darwin_arm64"].ProviderArchiveSHA256); err == nil {
			t.Fatal("tampered dependency lock was accepted")
		}
	}
}

func TestEveryWorkflowPlanCallerUsesExactToolAndEphemeralEncryption(t *testing.T) {
	workflowRoot := filepath.Join("..", "..", ".github", "workflows")
	paths, err := filepath.Glob(filepath.Join(workflowRoot, "*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	planCall := regexp.MustCompile(`(?m)^[ \t]+tofu plan(?:[ \t]|$)`)
	callers := make([]string, 0, 2)
	for _, path := range paths {
		contents := string(readTestFile(t, path))
		calls := planCall.FindAllStringIndex(contents, -1)
		if len(calls) == 0 {
			continue
		}
		callers = append(callers, filepath.Base(path))
		if len(calls) != 1 {
			t.Fatalf("%s has %d executable tofu plan callers; each caller needs an independently audited key seam", path, len(calls))
		}
		if !strings.Contains(contents, "tofu_version: "+OpenTofuVersion) || strings.Contains(contents, "tofu_version: latest") {
			t.Fatalf("%s does not select the exact OpenTofu version", path)
		}
		keyStart := strings.Index(contents, `plan_passphrase="$(openssl rand -hex 32)"`)
		if keyStart < 0 || keyStart > calls[0][0] {
			t.Fatalf("%s does not create a fresh cryptographic plan key before planning", path)
		}
		keySeam := contents[keyStart:calls[0][0]]
		for _, required := range []string{"export TF_ENCRYPTION", `method "aes_gcm" "speculative"`, "unset plan_passphrase"} {
			if !strings.Contains(keySeam, required) {
				t.Fatalf("%s plan key seam is missing %q", path, required)
			}
		}
		if strings.Contains(keySeam, "GITHUB_") || strings.Contains(keySeam, "${{") || strings.Contains(contents, "set -x") {
			t.Fatalf("%s derives or exposes plan encryption material through public workflow context", path)
		}
		if strings.Contains(contents, "tofu init -backend=false") {
			for _, required := range []string{
				"mv -f backend.tf /tmp/tofu-plan-backend.tf",
				`TF_VAR_active_repos: "{}"`,
				`TF_VAR_archived_repos: "{}"`,
			} {
				if !strings.Contains(contents, required) {
					t.Fatalf("%s backendless caller is missing %q", path, required)
				}
			}
		}
		if filepath.Base(path) == "tofu-drift.yml" {
			for _, forbidden := range []string{"| tee", "tail -c", "/tmp/drift.txt"} {
				if strings.Contains(contents, forbidden) {
					t.Fatalf("%s publishes private drift evidence through %q", path, forbidden)
				}
			}
			for _, required := range []string{
				`private_output="${RUNNER_TEMP}/tofu-drift.txt"`,
				`trap 'rm -f "${private_output}"' EXIT`,
				`>"${private_output}" 2>&1`,
				"Private plan, state, inventory, and provider evidence were withheld",
			} {
				if !strings.Contains(contents, required) {
					t.Fatalf("%s private drift evidence seam is missing %q", path, required)
				}
			}
		}
		callLineEnd := strings.IndexByte(contents[calls[0][0]:], '\n')
		if callLineEnd < 0 {
			callLineEnd = len(contents) - calls[0][0]
		}
		if regexp.MustCompile(`(?:^|[ \t])-out(?:=|[ \t]|$)`).MatchString(contents[calls[0][0]:calls[0][0]+callLineEnd]) ||
			strings.Contains(contents, "actions/upload-artifact") {
			t.Fatalf("%s persists a speculative plan without a transport key contract", path)
		}
	}
	sort.Strings(callers)
	want := []string{"tofu-drift.yml", "tofu-plan.yml"}
	if !equalStrings(callers, want) {
		t.Fatalf("executable workflow plan caller inventory = %v, want %v", callers, want)
	}
}

func TestPrimaryChecksumManifestsAndOfficialArtifacts(t *testing.T) {
	if os.Getenv("DEAR_AGENT_RUN_INFRAATTEST_ARTIFACTS") != "1" {
		t.Skip("set DEAR_AGENT_RUN_INFRAATTEST_ARTIFACTS=1 with signed release asset roots")
	}
	tofuRoot := os.Getenv("DEAR_AGENT_OPENTOFU_ASSET_ROOT")
	providerRoot := os.Getenv("DEAR_AGENT_PROVIDER_ASSET_ROOT")
	if tofuRoot == "" || providerRoot == "" {
		t.Fatal("OpenTofu and provider release asset roots are required")
	}
	tofuChecksums := readTestFile(t, filepath.Join(tofuRoot, "tofu_1.12.5_SHA256SUMS"))
	providerChecksums := readTestFile(t, filepath.Join(providerRoot, "terraform-provider-github_6.13.0_SHA256SUMS"))
	if digest(tofuChecksums) != "120345f8a2493375aebbca072106de425b2eb227837f8064440b8d911e36f987" ||
		digest(providerChecksums) != "2d688e8383ff669297bbb6461f7eb05168f53fe76d3233fdb431e318efedb98f" {
		t.Fatal("primary checksum manifest digest mismatch")
	}
	tofuEntries := parseChecksumManifest(t, tofuChecksums)
	providerEntries := parseChecksumManifest(t, providerChecksums)
	for platform, lock := range officialPlatformLocks {
		if tofuEntries[lock.OpenTofuArchive] != lock.OpenTofuArchiveSHA256 ||
			providerEntries[lock.ProviderArchive] != lock.ProviderArchiveSHA256 {
			t.Fatalf("signed checksum manifest does not bind %s", platform)
		}
		if hashFile(t, filepath.Join(tofuRoot, lock.OpenTofuArchive)) != lock.OpenTofuArchiveSHA256 ||
			hashFile(t, filepath.Join(providerRoot, lock.ProviderArchive)) != lock.ProviderArchiveSHA256 ||
			hashFile(t, filepath.Join(tofuRoot, platform, "tofu")) != lock.OpenTofuBinarySHA256 ||
			hashFile(t, filepath.Join(providerRoot, platform, "terraform-provider-github_v"+ProviderVersion)) != lock.ProviderBinarySHA256 {
			t.Fatalf("official archive or extracted binary digest mismatch for %s", platform)
		}
		verifyExecutablePlatform(t, filepath.Join(tofuRoot, platform, "tofu"), platform)
		verifyExecutablePlatform(t, filepath.Join(providerRoot, platform, "terraform-provider-github_v"+ProviderVersion), platform)
	}
}

func TestEvaluateToolchainAgainstOfficialBinaries(t *testing.T) {
	if os.Getenv("DEAR_AGENT_RUN_INFRAATTEST_TOOLCHAIN") != "1" {
		t.Skip("set DEAR_AGENT_RUN_INFRAATTEST_TOOLCHAIN=1 with exact binary paths")
	}
	tofuPath := os.Getenv("DEAR_AGENT_TOFU_BINARY")
	providerPath := os.Getenv("DEAR_AGENT_GITHUB_PROVIDER_BINARY")
	if tofuPath == "" || providerPath == "" {
		t.Fatal("exact OpenTofu and provider binary paths are required")
	}
	tofu := openTestFile(t, tofuPath)
	provider := openTestFile(t, providerPath)
	manifest := openTestFile(t, filepath.Join("..", "..", "infra", "toolchain.lock.json"))
	lockfile := openTestFile(t, filepath.Join("..", "..", "infra", ".terraform.lock.hcl"))
	platform := runtime.GOOS + "_" + runtime.GOARCH
	claims, err := evaluateToolchain(AuthorizationRequest{
		Platform: platform, OpenTofuBinary: tofu, ProviderBinary: provider,
		ToolchainManifest: manifest, DependencyLockfile: lockfile,
	})
	if err != nil {
		t.Fatal(err)
	}
	if claims.Platform != platform || claims.OpenTofuVersion != OpenTofuVersion ||
		claims.OpenTofuBinarySHA256 != officialPlatformLocks[platform].OpenTofuBinarySHA256 ||
		claims.Providers[0].BinarySHA256 != officialPlatformLocks[platform].ProviderBinarySHA256 {
		t.Fatalf("claims = %+v", claims)
	}
}

func readTestFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func openTestFile(t *testing.T, path string) *os.File {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func parseChecksumManifest(t *testing.T, raw []byte) map[string]string {
	t.Helper()
	entries := make(map[string]string)
	for line := range strings.SplitSeq(strings.TrimSpace(string(raw)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || !validSHA256(fields[0]) || filepath.Base(fields[1]) != fields[1] {
			t.Fatal("malformed primary checksum manifest")
		}
		if _, duplicate := entries[fields[1]]; duplicate {
			t.Fatal("duplicate primary checksum manifest entry")
		}
		entries[fields[1]] = fields[0]
	}
	return entries
}

func hashFile(t *testing.T, path string) string {
	t.Helper()
	file := openTestFile(t, path)
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func verifyExecutablePlatform(t *testing.T, path, platform string) {
	t.Helper()
	switch platform {
	case "darwin_arm64":
		file, err := macho.Open(path)
		if err != nil {
			t.Fatal("darwin artifact is not Mach-O")
		}
		defer file.Close()
		if file.Cpu != macho.CpuArm64 {
			t.Fatal("darwin artifact architecture mismatch")
		}
	case "linux_amd64", "linux_arm64":
		file, err := elf.Open(path)
		if err != nil {
			t.Fatal("linux artifact is not ELF")
		}
		defer file.Close()
		want := elf.EM_X86_64
		if platform == "linux_arm64" {
			want = elf.EM_AARCH64
		}
		if file.Machine != want {
			t.Fatal("linux artifact architecture mismatch")
		}
	default:
		t.Fatal("unsupported artifact platform")
	}
}
