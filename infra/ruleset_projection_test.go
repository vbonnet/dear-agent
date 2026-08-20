package infra

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// setupTofuMutationFixture copies the real OpenTofu sources and canonical
// ruleset into a temporary repository wired to an ephemeral local backend, so
// a mutation plan exercises the production module without any credentials.
func setupTofuMutationFixture(t *testing.T) (tofuEnv []string, tempInfra, tempCanonicalPath string, canonicalBytes []byte) {
	t.Helper()
	if os.Getenv("DEAR_AGENT_RUN_TOFU_MUTATION_TESTS") != "1" {
		t.Skip("set DEAR_AGENT_RUN_TOFU_MUTATION_TESTS=1 to run real OpenTofu mutation plans")
	}
	if _, err := exec.LookPath("tofu"); err != nil {
		t.Fatalf("OpenTofu is required for the mutation plans: %v", err)
	}

	_, file, _, _ := runtime.Caller(0)
	sourceInfra := filepath.Dir(file)
	tempRepo := t.TempDir()
	tempInfra = filepath.Join(tempRepo, "infra")
	copyTofuSources(t, sourceInfra, tempInfra)

	canonicalPath := filepath.Join(sourceInfra, "..", ".github", "rulesets", "main.json")
	canonicalBytes, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatalf("read canonical ruleset: %v", err)
	}
	tempCanonicalPath = filepath.Join(tempRepo, ".github", "rulesets", "main.json")
	writeTestFile(t, tempCanonicalPath, canonicalBytes, 0o644)

	// claude_review.tf reads this workflow through file(). Without it the plan
	// aborts while evaluating locals, before any resource precondition runs,
	// so a mutation test could never reach the module's own guarantees.
	reviewWorkflow, err := os.ReadFile(filepath.Join(sourceInfra, "..", ".github", "workflows", "claude-code-review.yml"))
	if err != nil {
		t.Fatalf("read claude review workflow: %v", err)
	}
	writeTestFile(t, filepath.Join(tempRepo, ".github", "workflows", "claude-code-review.yml"), reviewWorkflow, 0o644)

	backendFixture, err := os.ReadFile(filepath.Join(sourceInfra, "testdata", "ci_backend_override.tf.fixture"))
	if err != nil {
		t.Fatalf("read fixture backend: %v", err)
	}
	writeTestFile(t, filepath.Join(tempInfra, "ci_backend_override.tf"), backendFixture, 0o644)

	tofuEnv = credentialFreeTofuEnv()
	if providerCache := filepath.Join(sourceInfra, ".terraform", "providers"); directoryExists(providerCache) {
		tofuEnv = append(tofuEnv, "TF_PLUGIN_CACHE_DIR="+providerCache)
	}
	runTofuMutationCommand(t, tempInfra, tofuEnv, true, "init", "-reconfigure", "-input=false", "-no-color")
	return tofuEnv, tempInfra, tempCanonicalPath, canonicalBytes
}

func TestCanonicalRulesetProjectionRejectsInvalidRequiredReviewers(t *testing.T) {
	tofuEnv, tempInfra, tempCanonicalPath, canonicalBytes := setupTofuMutationFixture(t)

	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{
			name: "omitted",
			mutate: func(parameters map[string]any) {
				delete(parameters, "required_reviewers")
			},
		},
		{
			name: "null",
			mutate: func(parameters map[string]any) {
				parameters["required_reviewers"] = nil
			},
		},
		{
			name: "wrong type",
			mutate: func(parameters map[string]any) {
				parameters["required_reviewers"] = "not-an-array"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var canonical map[string]any
			if err := json.Unmarshal(canonicalBytes, &canonical); err != nil {
				t.Fatalf("parse canonical ruleset: %v", err)
			}
			parameters := pullRequestParameters(t, canonical)
			tt.mutate(parameters)
			mutated, err := json.Marshal(canonical)
			if err != nil {
				t.Fatalf("encode mutated canonical ruleset: %v", err)
			}
			writeTestFile(t, tempCanonicalPath, mutated, 0o644)

			output := runTofuMutationCommand(t, tempInfra, tofuEnv, false,
				"plan",
				"-refresh=false",
				"-input=false",
				"-lock=false",
				"-no-color",
				"-var-file=testdata/ci.tfvars.fixture",
				"-var=claude_code_oauth_token=plan-only-placeholder",
				"-out="+filepath.Join(t.TempDir(), "fixture.tfplan"),
			)
			if !strings.Contains(output, "required_reviewers") {
				t.Fatalf("failed plan did not identify required_reviewers:\n%s", output)
			}
		})
	}
}

// TestCanonicalRulesetProjectionRequiresDefaultBranchTarget proves the module
// refuses to apply a ruleset that does not actually protect the default
// branch. A length-only check on ref_name.include let a canonical edit move
// the zero-bypass policy onto some other ref, or exclude the default branch
// back out, while the saved plan still applied cleanly.
func TestCanonicalRulesetProjectionRequiresDefaultBranchTarget(t *testing.T) {
	tofuEnv, tempInfra, tempCanonicalPath, canonicalBytes := setupTofuMutationFixture(t)

	tests := []struct {
		name    string
		include []any
		exclude []any
	}{
		{name: "include without the default branch marker", include: []any{"refs/heads/release"}, exclude: []any{}},
		{name: "empty include", include: []any{}, exclude: []any{}},
		{name: "default branch marker excluded", include: []any{"~DEFAULT_BRANCH"}, exclude: []any{"~DEFAULT_BRANCH"}},
		{name: "default branch excluded by literal ref", include: []any{"~DEFAULT_BRANCH"}, exclude: []any{"refs/heads/main"}},
		{name: "any exclusion at all", include: []any{"~DEFAULT_BRANCH"}, exclude: []any{"refs/heads/scratch/**"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var canonical map[string]any
			if err := json.Unmarshal(canonicalBytes, &canonical); err != nil {
				t.Fatalf("parse canonical ruleset: %v", err)
			}
			refName := refNameConditions(t, canonical)
			refName["include"] = tt.include
			refName["exclude"] = tt.exclude
			mutated, err := json.Marshal(canonical)
			if err != nil {
				t.Fatalf("encode mutated canonical ruleset: %v", err)
			}
			writeTestFile(t, tempCanonicalPath, mutated, 0o644)

			output := runTofuMutationCommand(t, tempInfra, tofuEnv, false,
				"plan",
				"-refresh=false",
				"-input=false",
				"-lock=false",
				"-no-color",
				"-var-file=testdata/ci.tfvars.fixture",
				"-var=claude_code_oauth_token=plan-only-placeholder",
				"-out="+filepath.Join(t.TempDir(), "fixture.tfplan"),
			)
			if !strings.Contains(output, "~DEFAULT_BRANCH") && !strings.Contains(output, "exclude must be empty") {
				t.Fatalf("failed plan did not identify the default-branch target:\n%s", output)
			}
		})
	}
}

// TestCanonicalRulesetProjectionPlansCleanly is the positive control for both
// mutation tests: the unmutated canonical ruleset must still satisfy every
// module precondition, so a failing mutation proves the guard fired rather
// than that the fixture is broken.
func TestCanonicalRulesetProjectionPlansCleanly(t *testing.T) {
	tofuEnv, tempInfra, _, _ := setupTofuMutationFixture(t)
	runTofuMutationCommand(t, tempInfra, tofuEnv, true,
		"plan",
		"-refresh=false",
		"-input=false",
		"-lock=false",
		"-no-color",
		"-var-file=testdata/ci.tfvars.fixture",
		"-var=claude_code_oauth_token=plan-only-placeholder",
		"-out="+filepath.Join(t.TempDir(), "fixture.tfplan"),
	)
}

func refNameConditions(t *testing.T, canonical map[string]any) map[string]any {
	t.Helper()
	conditions, ok := canonical["conditions"].(map[string]any)
	if !ok {
		t.Fatal("canonical ruleset has no conditions object")
	}
	refName, ok := conditions["ref_name"].(map[string]any)
	if !ok {
		t.Fatal("canonical ruleset has no conditions.ref_name object")
	}
	return refName
}

func copyTofuSources(t *testing.T, sourceInfra, tempInfra string) {
	t.Helper()
	err := filepath.WalkDir(sourceInfra, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path == filepath.Join(sourceInfra, ".terraform") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".tf" || filepath.Base(path) == "ci_backend_override.tf" {
			return nil
		}
		rel, err := filepath.Rel(sourceInfra, path)
		if err != nil {
			return err
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return writeFile(filepath.Join(tempInfra, rel), contents, 0o644)
	})
	if err != nil {
		t.Fatalf("copy OpenTofu sources from %s: %v", sourceInfra, err)
	}

	for _, rel := range []string{
		filepath.Join("testdata", "ci.tfvars.fixture"),
		filepath.Join("testdata", "ci_backend_override.tf.fixture"),
		".terraform.lock.hcl",
	} {
		contents, err := os.ReadFile(filepath.Join(sourceInfra, rel))
		if os.IsNotExist(err) && rel == ".terraform.lock.hcl" {
			continue
		}
		if err != nil {
			t.Fatalf("read OpenTofu fixture %s: %v", rel, err)
		}
		writeTestFile(t, filepath.Join(tempInfra, rel), contents, 0o644)
	}
}

func pullRequestParameters(t *testing.T, canonical map[string]any) map[string]any {
	t.Helper()
	rules, ok := canonical["rules"].([]any)
	if !ok {
		t.Fatalf("canonical rules are %T, want array", canonical["rules"])
	}
	for _, raw := range rules {
		rule, ok := raw.(map[string]any)
		if !ok || rule["type"] != "pull_request" {
			continue
		}
		parameters, ok := rule["parameters"].(map[string]any)
		if !ok {
			t.Fatalf("pull-request parameters are %T, want object", rule["parameters"])
		}
		return parameters
	}
	t.Fatal("canonical ruleset has no pull_request rule")
	return nil
}

func runTofuMutationCommand(t *testing.T, dir string, env []string, wantSuccess bool, args ...string) string {
	t.Helper()
	cmd := exec.Command("tofu", args...)
	cmd.Dir = dir
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if wantSuccess && err != nil {
		t.Fatalf("tofu %s failed: %v\n%s", args[0], err, output)
	}
	if !wantSuccess && err == nil {
		t.Fatalf("tofu %s unexpectedly succeeded\n%s", args[0], output)
	}
	return string(output)
}

func credentialFreeTofuEnv() []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		if name == "GITHUB_TOKEN" || name == "GH_TOKEN" || strings.HasPrefix(name, "TF_VAR_") {
			continue
		}
		env = append(env, value)
	}
	// infra/encryption.tf sets `plan { enforced = true }`, so OpenTofu refuses
	// every plan without an encryption method. These plans are written to a
	// t.TempDir() and discarded, so an ephemeral passphrase generated per run
	// is the right scope: it never leaves the process and guards nothing that
	// outlives the test.
	return append(env, "TF_IN_AUTOMATION=1", "TF_ENCRYPTION="+ephemeralPlanEncryption())
}

// ephemeralPlanEncryption builds an OpenTofu encryption configuration whose
// passphrase exists only for this test run.
func ephemeralPlanEncryption() string {
	passphrase := make([]byte, 32)
	if _, err := rand.Read(passphrase); err != nil {
		// crypto/rand failing is not a condition a test can meaningfully
		// continue through, and a fixed fallback would be worse than stopping.
		panic("generate ephemeral plan passphrase: " + err.Error())
	}
	return fmt.Sprintf(`key_provider "pbkdf2" "fixture" {
  passphrase = %q
}
method "aes_gcm" "fixture" {
  keys = key_provider.pbkdf2.fixture
}
plan {
  method = method.aes_gcm.fixture
}`, hex.EncodeToString(passphrase))
}

func directoryExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func writeTestFile(t *testing.T, path string, contents []byte, mode fs.FileMode) {
	t.Helper()
	if err := writeFile(path, contents, mode); err != nil {
		t.Fatal(err)
	}
}

func writeFile(path string, contents []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, contents, mode)
}
