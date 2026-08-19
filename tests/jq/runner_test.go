// Package jqtest runs the checked-in jq programs against recorded input and
// expected-output fixtures.
//
// jq programs in this repository encode policy decisions: which ruleset
// documents are well formed, which required checks are missing, which
// repository inventories are safe to act on. They are executed by workflows
// and by infra/import.sh, where a silent behavior change surfaces as a
// mis-audited ruleset rather than as a test failure. This package is the gate
// that makes such a change visible.
//
// It is a Go test rather than a shell runner so it needs no new CI wiring
// (ci.yml already runs `go test`), no new shell under the 20-line policy, and
// so a malformed fixture is a loud failure instead of a skipped case.
//
// A case is a directory under testdata/<suite>/<name>/ holding:
//
//	case.json           how to invoke jq (see caseSpec)
//	input.json          the document piped to jq
//	expected.json       the expected output, compared as JSON, or
//	expected.txt        the expected output, compared as raw text, or
//	expected-error.txt  a substring jq's stderr must contain
//
// Exactly one expected-* file must be present. To add a program, add a suite
// directory; the runner discovers it with no code change.
package jqtest

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// caseSpec is the contents of a case.json file.
type caseSpec struct {
	// Program is the jq source this case exercises, relative to the
	// repository root. It is always set, because the coverage check at the
	// end of TestJQPrograms uses it to prove no .jq file is untested.
	Program string `json:"program"`
	// Filter is an inline jq expression to run instead of `-f Program`. It is
	// how a library of bare `def`s gets exercised: `jq -f` needs a final
	// expression, which such a file deliberately does not have. The filter is
	// expected to `include` the library named by Program.
	Filter string `json:"filter,omitempty"`
	// Description explains what the case pins down. It is required: a fixture
	// nobody can explain is a fixture nobody can correct.
	Description string `json:"description"`
	// Args are --arg name value pairs (string arguments).
	Args map[string]string `json:"args,omitempty"`
	// JSONArgs are --argjson name value pairs (parsed as JSON).
	JSONArgs map[string]any `json:"jsonargs,omitempty"`
	// LibraryPaths are -L include directories, relative to the repository root.
	LibraryPaths []string `json:"library_paths,omitempty"`
	// Raw requests -r, so output is compared against expected.txt as text.
	Raw bool `json:"raw,omitempty"`
}

func TestJQPrograms(t *testing.T) {
	jq, err := exec.LookPath("jq")
	if err != nil {
		// CI installs jq explicitly, so this skip cannot hide a CI failure.
		t.Skip("jq is not installed; the jq gate runs in CI")
	}

	root := repoRoot(t)
	cases := discoverCases(t, filepath.Join(root, "tests", "jq", "testdata"))
	if len(cases) == 0 {
		t.Fatal("no jq fixture cases found; the gate would pass vacuously")
	}
	t.Logf("running %d jq fixture case(s)", len(cases))

	programs := map[string]bool{}
	for _, dir := range cases {
		t.Run(caseName(t, root, dir), func(t *testing.T) {
			spec := loadSpec(t, root, dir)
			programs[spec.Program] = true
			runCase(t, jq, root, dir, spec)
		})
	}

	t.Run("every checked-in jq program has at least one case", func(t *testing.T) {
		for _, program := range discoverPrograms(t, root) {
			if !programs[program] {
				t.Errorf("%s has no fixture case under tests/jq/testdata/", program)
			}
		}
	})
}

// runCase executes one fixture and compares jq's behavior against the recorded
// expectation.
func runCase(t *testing.T, jq, root, dir string, spec caseSpec) {
	t.Helper()

	args := []string{}
	for _, lib := range spec.LibraryPaths {
		args = append(args, "-L", filepath.Join(root, lib))
	}
	if spec.Raw {
		args = append(args, "-r")
	}
	// Sorted so the command line is deterministic and a failure message is
	// reproducible by hand.
	for _, name := range sortedKeys(spec.Args) {
		args = append(args, "--arg", name, spec.Args[name])
	}
	for _, name := range sortedKeys(spec.JSONArgs) {
		encoded, err := json.Marshal(spec.JSONArgs[name])
		if err != nil {
			t.Fatalf("encode jsonarg %s: %v", name, err)
		}
		args = append(args, "--argjson", name, string(encoded))
	}
	if spec.Filter != "" {
		args = append(args, spec.Filter)
	} else {
		args = append(args, "-f", filepath.Join(root, spec.Program))
	}

	input, err := os.ReadFile(filepath.Join(dir, "input.json"))
	if err != nil {
		t.Fatalf("read input.json: %v", err)
	}

	cmd := exec.Command(jq, args...)
	cmd.Stdin = bytes.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	switch {
	case fileExists(filepath.Join(dir, "expected-error.txt")):
		want := strings.TrimSpace(readFile(t, filepath.Join(dir, "expected-error.txt")))
		if runErr == nil {
			t.Fatalf("jq unexpectedly succeeded; expected an error containing %q.\nstdout:\n%s", want, stdout.String())
		}
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("jq stderr does not contain %q.\nstderr:\n%s", want, stderr.String())
		}

	case fileExists(filepath.Join(dir, "expected.txt")):
		requireSuccess(t, runErr, &stderr)
		got := strings.TrimRight(stdout.String(), "\n")
		want := strings.TrimRight(readFile(t, filepath.Join(dir, "expected.txt")), "\n")
		if got != want {
			t.Fatalf("raw output mismatch.\n got:\n%s\nwant:\n%s", got, want)
		}

	case fileExists(filepath.Join(dir, "expected.json")):
		requireSuccess(t, runErr, &stderr)
		got := decodeJSON(t, "jq output", stdout.Bytes())
		want := decodeJSON(t, "expected.json", []byte(readFile(t, filepath.Join(dir, "expected.json"))))
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("JSON output mismatch.\n got: %s\nwant: %s", compact(t, got), compact(t, want))
		}

	default:
		t.Fatalf("case %s has no expected.json, expected.txt or expected-error.txt", dir)
	}
}

func requireSuccess(t *testing.T, err error, stderr *bytes.Buffer) {
	t.Helper()
	if err != nil {
		t.Fatalf("jq failed: %v\nstderr:\n%s", err, stderr.String())
	}
}

// discoverCases returns every directory holding a case.json, so adding a
// fixture never means editing this file.
func discoverCases(t *testing.T, casesRoot string) []string {
	t.Helper()
	var dirs []string
	err := filepath.WalkDir(casesRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "case.json" {
			dirs = append(dirs, filepath.Dir(path))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", casesRoot, err)
	}
	sort.Strings(dirs)
	return dirs
}

// discoverPrograms lists every checked-in jq program, so a new one cannot be
// added without a fixture.
func discoverPrograms(t *testing.T, root string) []string {
	t.Helper()
	var programs []string
	err := filepath.WalkDir(filepath.Join(root, ".github"), func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".jq" {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		programs = append(programs, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walk .github: %v", err)
	}
	sort.Strings(programs)
	return programs
}

func loadSpec(t *testing.T, root, dir string) caseSpec {
	t.Helper()
	var spec caseSpec
	raw := readFile(t, filepath.Join(dir, "case.json"))
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		t.Fatalf("parse case.json: %v", err)
	}
	if spec.Program == "" {
		t.Fatal("case.json has no program")
	}
	if strings.TrimSpace(spec.Description) == "" {
		t.Fatal("case.json has no description")
	}
	if _, err := os.Stat(filepath.Join(root, spec.Program)); err != nil {
		// A typo in program would silently exempt a real jq file from the
		// coverage check below while still passing its own case.
		t.Fatalf("case.json names a program that does not exist: %s", spec.Program)
	}
	return spec
}

func repoRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func caseName(t *testing.T, root, dir string) string {
	t.Helper()
	rel, err := filepath.Rel(filepath.Join(root, "tests", "jq", "testdata"), dir)
	if err != nil {
		t.Fatalf("relativize %s: %v", dir, err)
	}
	return filepath.ToSlash(rel)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}

func decodeJSON(t *testing.T, label string, raw []byte) any {
	t.Helper()
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("%s is not valid JSON: %v\n%s", label, err, raw)
	}
	return value
}

func compact(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return string(encoded)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
