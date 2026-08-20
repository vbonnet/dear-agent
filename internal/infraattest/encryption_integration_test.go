package infraattest

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const fixtureEncryptionConfig = `key_provider "pbkdf2" "plan" {
  passphrase = "fixture-passphrase-32-bytes-long"
  iterations = 200000
}
method "aes_gcm" "plan" {
  keys = key_provider.pbkdf2.plan
}
plan {
  method = method.aes_gcm.plan
}
`

func TestOpenTofuEncryptedPlanRoundTrip(t *testing.T) {
	if os.Getenv("DEAR_AGENT_RUN_TOFU_ENCRYPTION_TESTS") != "1" {
		t.Skip("set DEAR_AGENT_RUN_TOFU_ENCRYPTION_TESTS=1 with the exact OpenTofu binary")
	}
	tofuPath := os.Getenv("DEAR_AGENT_TOFU_BINARY")
	if tofuPath == "" {
		t.Fatal("DEAR_AGENT_TOFU_BINARY is required")
	}
	versionOutput := runTofuFixture(t, tofuPath, "", "", "version", "-json")
	var version struct {
		Version  string `json:"terraform_version"`
		Platform string `json:"platform"`
	}
	if err := json.Unmarshal(versionOutput, &version); err != nil || version.Version != OpenTofuVersion {
		t.Fatalf("unexpected OpenTofu version metadata")
	}

	workdir := t.TempDir()
	config := readFixture(t, filepath.Join("encryption", "main.tf"))
	if err := os.WriteFile(filepath.Join(workdir, "main.tf"), config, 0o600); err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Join(t.TempDir(), "tofu-data")
	runTofuFixture(t, tofuPath, workdir, dataDir, "init", "-backend=false", "-input=false", "-no-color")

	planPath := filepath.Join(workdir, "encrypted.tfplan")
	planOutput := runTofuFixtureWithEncryption(
		t, tofuPath, workdir, dataDir, fixtureEncryptionConfig,
		"plan", "-input=false", "-lock=false", "-no-color",
		"-var=private_sentinel="+privateSentinel, "-out="+planPath,
	)
	assertNoSentinel(t, "plan output", planOutput)
	ciphertext, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	assertNoSentinel(t, "encrypted plan", ciphertext)
	if _, err := readEncryptedPlan(bytes.NewReader(ciphertext)); err != nil {
		t.Fatal("OpenTofu 1.12.5 encrypted plan did not match the bounded envelope contract")
	}
	if len(ciphertext) == 0 {
		t.Fatal("encrypted plan is empty")
	}

	privateJSON := runTofuFixtureWithEncryption(
		t, tofuPath, workdir, dataDir, fixtureEncryptionConfig,
		"show", "-json", planPath,
	)
	if !bytes.Contains(privateJSON, []byte(privateSentinel)) {
		t.Fatal("authorized in-memory decrypt did not recover the private sentinel")
	}

	for _, encryption := range []string{"", strings.Replace(fixtureEncryptionConfig, "fixture-passphrase-32-bytes-long", "wrong-passphrase-32-bytes-long", 1)} {
		output, err := runTofuFixtureFailure(tofuPath, workdir, dataDir, encryption, "show", "-json", planPath)
		if err == nil {
			t.Fatal("encrypted plan was readable without the exact key")
		}
		assertNoSentinel(t, "decrypt failure", output)
	}

	unencryptedPlan := filepath.Join(workdir, "unencrypted.tfplan")
	output, err := runTofuFixtureFailure(
		tofuPath, workdir, dataDir, "", "plan", "-input=false", "-lock=false", "-no-color",
		"-var=private_sentinel="+privateSentinel, "-out="+unencryptedPlan,
	)
	if err == nil {
		t.Fatal("plan encryption enforcement accepted a missing method")
	}
	assertNoSentinel(t, "enforcement failure", output)
}

func TestEncryptedPlanEnvelopeFailsClosed(t *testing.T) {
	validEnvelope, err := json.Marshal(encryptedPlanEnvelope{
		EncryptionVersion: "v0",
		Meta: map[string]string{
			"key_provider.pbkdf2.plan": base64.StdEncoding.EncodeToString([]byte(`{"salt":"fixture"}`)),
		},
		EncryptedData: base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x7f}, minimumCiphertextBytes)),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readEncryptedPlan(bytes.NewReader(validEnvelope)); err != nil {
		t.Fatal(err)
	}
	tests := [][]byte{
		[]byte("PK\x03\x04" + privateSentinel),
		[]byte(`{"encryption_version":"v0","meta":{},"encrypted_data":"AAAA"}`),
		bytes.Replace(validEnvelope, []byte(`"v0"`), []byte(`"future"`), 1),
		bytes.Replace(validEnvelope, []byte(`"encrypted_data":"`), []byte(`"unknown":true,"encrypted_data":"!`), 1),
		bytes.Replace(validEnvelope, []byte(`"encryption_version":"v0"`), []byte(`"encryption_version":"v0","encryption_version":"v0"`), 1),
	}
	for _, raw := range tests {
		_, err := readEncryptedPlan(bytes.NewReader(raw))
		assertCode(t, err, CodeInvalidInput)
		if strings.Contains(err.Error(), privateSentinel) {
			t.Fatal("encrypted plan rejection exposed private bytes")
		}
	}
}

func TestOpenTofuRepositorySpeculativePlanWithoutProviderSecrets(t *testing.T) {
	if os.Getenv("DEAR_AGENT_RUN_TOFU_REPOSITORY_PLAN") != "1" {
		t.Skip("set DEAR_AGENT_RUN_TOFU_REPOSITORY_PLAN=1 with exact local tool/provider paths")
	}
	tofuPath := os.Getenv("DEAR_AGENT_TOFU_BINARY")
	pluginDir := os.Getenv("DEAR_AGENT_GITHUB_PROVIDER_PLUGIN_DIR")
	if tofuPath == "" || pluginDir == "" {
		t.Fatal("exact OpenTofu binary and provider plugin directory are required")
	}

	root := t.TempDir()
	copyTestTree(t, filepath.Join("..", "..", "infra"), filepath.Join(root, "infra"), func(relative string, entry fs.DirEntry) bool {
		return relative == "backend.tf" || entry.Name() == ".terraform" || strings.HasSuffix(entry.Name(), ".tfstate")
	})
	workflowSource := filepath.Join("..", "..", ".github", "workflows", "claude-code-review.yml")
	workflowTarget := filepath.Join(root, ".github", "workflows", "claude-code-review.yml")
	copyTestFile(t, workflowSource, workflowTarget)

	var callsMu sync.Mutex
	calls := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		callsMu.Lock()
		calls[request.URL.Path]++
		callsMu.Unlock()
		writer.Header().Set("Content-Type", "application/json")
		switch {
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/orgs/vbonnet"):
			writer.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(writer, `{"message":"Not Found"}`)
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/orgs/dear-labs"):
			_, _ = io.WriteString(writer, `{"id":42,"login":"dear-labs"}`)
		default:
			writer.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(writer, `{"message":"unexpected fixture request"}`)
		}
	}))
	t.Cleanup(server.Close)

	dataDir := filepath.Join(t.TempDir(), "tofu-data")
	workdir := filepath.Join(root, "infra")
	runTofuFixtureWithExtraEnvironment(
		t, tofuPath, workdir, dataDir, "", nil,
		"init", "-backend=false", "-input=false", "-no-color", "-lockfile=readonly", "-plugin-dir="+pluginDir,
	)
	runTofuFixtureWithExtraEnvironment(t, tofuPath, workdir, dataDir, "", nil, "validate", "-no-color")

	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal("failed to obtain speculative plan randomness")
	}
	encryption := strings.Replace(fixtureEncryptionConfig, "fixture-passphrase-32-bytes-long", hex.EncodeToString(key), 1)
	output := runTofuFixtureWithExtraEnvironment(
		t,
		tofuPath,
		workdir,
		dataDir,
		encryption,
		[]string{
			"GITHUB_BASE_URL=" + server.URL + "/",
			"GITHUB_TOKEN=fixture-only-token",
			"TF_VAR_personal_owner=vbonnet",
			"TF_VAR_org_name=dear-labs",
			"TF_VAR_active_repos={}",
			"TF_VAR_archived_repos={}",
			"TF_VAR_claude_code_oauth_token=" + privateSentinel,
		},
		"plan", "-refresh=false", "-input=false", "-lock=false", "-no-color",
	)
	assertNoSentinel(t, "repository speculative plan", output)

	callsMu.Lock()
	defer callsMu.Unlock()
	var personalLookup, organizationLookup bool
	for path := range calls {
		personalLookup = personalLookup || strings.HasSuffix(path, "/orgs/vbonnet")
		organizationLookup = organizationLookup || strings.HasSuffix(path, "/orgs/dear-labs")
	}
	if !personalLookup || !organizationLookup {
		t.Fatal("exact provider did not exercise both configured owner lookups")
	}
}

func runTofuFixture(t *testing.T, tofuPath, dir, dataDir string, args ...string) []byte {
	t.Helper()
	output, err := runTofuFixtureCommand(tofuPath, dir, dataDir, "", args...)
	if err != nil {
		t.Fatalf("OpenTofu fixture command failed without exposing captured output")
	}
	return output
}

func runTofuFixtureWithEncryption(t *testing.T, tofuPath, dir, dataDir, encryption string, args ...string) []byte {
	t.Helper()
	output, err := runTofuFixtureCommand(tofuPath, dir, dataDir, encryption, args...)
	if err != nil {
		t.Fatalf("OpenTofu encrypted fixture command failed without exposing captured output")
	}
	return output
}

func runTofuFixtureFailure(tofuPath, dir, dataDir, encryption string, args ...string) ([]byte, error) {
	return runTofuFixtureCommand(tofuPath, dir, dataDir, encryption, args...)
}

func runTofuFixtureCommand(tofuPath, dir, dataDir, encryption string, args ...string) ([]byte, error) {
	command := exec.Command(tofuPath, args...)
	if dir != "" {
		command.Dir = dir
	}
	command.Env = tofuFixtureEnvironment(dataDir, encryption)
	return command.CombinedOutput()
}

func runTofuFixtureWithExtraEnvironment(
	t *testing.T,
	tofuPath, dir, dataDir, encryption string,
	extra []string,
	args ...string,
) []byte {
	t.Helper()
	command := exec.Command(tofuPath, args...)
	command.Dir = dir
	command.Env = append(tofuFixtureEnvironment(dataDir, encryption), extra...)
	output, err := command.CombinedOutput()
	if err != nil {
		assertNoSentinel(t, "OpenTofu repository command failure", output)
		t.Fatal("OpenTofu repository fixture command failed without exposing captured output")
	}
	return output
}

func tofuFixtureEnvironment(dataDir, encryption string) []string {
	environment := make([]string, 0, len(os.Environ())+3)
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		if name == "TF_ENCRYPTION" || name == "TF_DATA_DIR" || strings.HasPrefix(name, "TF_VAR_") ||
			name == "GITHUB_BASE_URL" || name == "GITHUB_OWNER" || strings.Contains(name, "TOKEN") ||
			strings.Contains(name, "SECRET") || strings.HasPrefix(name, "AWS_") {
			continue
		}
		environment = append(environment, value)
	}
	environment = append(environment, "TF_IN_AUTOMATION=1")
	if dataDir != "" {
		environment = append(environment, "TF_DATA_DIR="+dataDir)
	}
	if encryption != "" {
		environment = append(environment, "TF_ENCRYPTION="+encryption)
	}
	return environment
}

func assertNoSentinel(t *testing.T, subject string, value []byte) {
	t.Helper()
	if bytes.Contains(value, []byte(privateSentinel)) {
		t.Fatalf("%s exposed the private sentinel", subject)
	}
}

func copyTestTree(t *testing.T, source, target string, skip func(string, fs.DirEntry) bool) {
	t.Helper()
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return os.MkdirAll(target, 0o700)
		}
		if skip(relative, entry) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o700)
		}
		copyTestFile(t, path, destination)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func copyTestFile(t *testing.T, source, target string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	input, err := os.Open(source)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
}
