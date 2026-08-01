package buildstamp_test

import (
	"bufio"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

const (
	testBuildVersion = "9.9.9"
	testBuildCommit  = "0123456789ab"
	testBuildDate    = "2026-08-01T00:00:00Z"
	versionPackage   = "github.com/vbonnet/dear-agent/pkg/version"
)

type buildStamp struct {
	Version       string `json:"version"`
	GitCommit     string `json:"git_commit"`
	BuildDate     string `json:"build_date"`
	BuiltBy       string `json:"built_by"`
	Extra         string `json:"extra"`
	GOFLAGSMarker string `json:"goflags_marker"`
}

func TestGovernedBuildPreservesOrdinaryGOFLAGS(t *testing.T) {
	requireMake(t)

	tests := []struct {
		name   string
		env    map[string]string
		args   []string
		marker string
	}{
		{name: "make p", args: []string{"GOFLAGS=-p=2"}, marker: "disabled"},
		{name: "make buildvcs", args: []string{"GOFLAGS=-buildvcs=false"}, marker: "disabled"},
		{name: "make observable tag", args: []string{"GOFLAGS=-p=2 -tags=buildstamp_goflags"}, marker: "enabled"},
		{
			name:   "process environment",
			env:    map[string]string{"GOFLAGS": "-p=2 -tags=buildstamp_goflags"},
			marker: "enabled",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildProbe(t, tc.env, tc.args...)
			assertBuildStamp(t, got, "unset", tc.marker)
		})
	}
}

func TestGovernedBuildPreservesPersistedOrdinaryGOFLAGS(t *testing.T) {
	requireMake(t)
	goenv := writeGOENV(t, "-tags=buildstamp_goflags")
	got := buildProbe(t, map[string]string{"GOENV": goenv})
	assertBuildStamp(t, got, "unset", "enabled")
}

func TestGovernedBuildComposesOptionalAndProtectedLinkerFlags(t *testing.T) {
	requireMake(t)
	extra := "-s -X 'main.extra=caller  value'" +
		" -X '" + versionPackage + ".Version=caller-version'" +
		" -X '" + versionPackage + ".GitCommit=caller-commit'" +
		" -X '" + versionPackage + ".BuildDate=caller-date'" +
		" -X '" + versionPackage + ".BuiltBy=caller-built-by'"
	got := buildProbe(t, nil, "EXTRA_GO_LDFLAGS="+extra)
	assertBuildStamp(t, got, "caller  value", "disabled")
}

func TestBuildStampInputsAreOpaqueToMakeAndShell(t *testing.T) {
	requireMake(t)
	escapedOutput := filepath.Join(t.TempDir(), "unstamped")
	rendered, err := runMake(t, nil,
		"-n",
		"build-stamp-test-probe",
		"BUILD_STAMP_TEST_OUTPUT="+filepath.Join(t.TempDir(), "unused"),
		`EXTRA_GO_LDFLAGS=-s " -o `+escapedOutput+` ./tests/buildstamp/testdata/probe/ #`,
	)
	if err != nil {
		t.Fatalf("render opaque linker input: %v\n%s", err, rendered)
	}
	if strings.Contains(rendered, escapedOutput) || !strings.Contains(rendered, "${_BUILD_STAMP_EXTRA_LDFLAGS}") {
		t.Fatalf("caller linker input escaped its opaque environment boundary:\n%s", rendered)
	}

	sentinel := filepath.Join(t.TempDir(), "must-not-exist")
	extraLiteral := "$(>" + sentinel + ")"
	metadataLiteral := "review#;`literal`" + extraLiteral
	got := buildProbe(t, nil,
		"VERSION="+metadataLiteral,
		"EXTRA_GO_LDFLAGS=-X 'main.extra="+extraLiteral+"'",
	)
	want := buildStamp{
		Version:       metadataLiteral,
		GitCommit:     testBuildCommit,
		BuildDate:     testBuildDate,
		BuiltBy:       "makefile",
		Extra:         extraLiteral,
		GOFLAGSMarker: "disabled",
	}
	if got != want {
		t.Fatalf("opaque build stamp = %+v, want %+v", got, want)
	}
	if _, statErr := os.Stat(sentinel); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("Make or the shell evaluated opaque build metadata (stat error %v)", statErr)
	}
}

func TestCallerGOFLAGSAreOpaqueToMake(t *testing.T) {
	requireMake(t)
	const marker = "MAKE_MUST_NOT_EXPAND_GOFLAGS"
	output, err := runMake(t, nil,
		"-n",
		"build-stamp-test-probe",
		"BUILD_STAMP_TEST_OUTPUT="+filepath.Join(t.TempDir(), "unused"),
		"GOFLAGS=$(info "+marker+")-p=2",
	)
	if err != nil {
		t.Fatalf("opaque GOFLAGS dry run failed: %v\n%s", err, output)
	}
	if strings.Contains(output, marker) {
		t.Fatalf("Make evaluated caller GOFLAGS as Make syntax:\n%s", output)
	}
}

func TestGovernedBuildProtectsInternalMakeVariables(t *testing.T) {
	requireMake(t)
	got := buildProbe(t, nil,
		"GOFLAGS=-p=2",
		"_BUILD_STAMP_PACKAGE=example.invalid/version",
		"_BUILD_STAMP_EXTRA_LDFLAGS=-s",
		"_BUILD_STAMP_VERSION=caller",
		"_BUILD_STAMP_GIT_COMMIT=caller",
		"_BUILD_STAMP_DATE=caller",
		"_BUILD_STAMP_TEST_OUTPUT="+filepath.Join(t.TempDir(), "hijacked"),
		"_CALLER_GOFLAGS=-ldflags=-s",
		"_MANDATORY_VERSION_LDFLAGS=-s",
		"_EFFECTIVE_GOFLAGS=-ldflags=-s",
		"_NORMALIZED_EFFECTIVE_GOFLAGS=-ldflags=-s",
		"_GOFLAGS_LDFLAGS=-ldflags=-s",
		"_NORMALIZED_EXTRA_GO_LDFLAGS=example.invalid=-s",
		"_INVALID_EXTRA_GO_LDFLAGS=yes",
		"_BUILD_STAMP_LDFLAGS=-s",
		"BUILD_STAMP_FLAGS=-ldflags=-s",
	)
	assertBuildStamp(t, got, "unset", "disabled")
}

func TestGovernedBuildRejectsCompetingGOFLAGSBeforeBuilding(t *testing.T) {
	requireMake(t)
	tests := []struct {
		name string
		env  map[string]string
		arg  string
	}{
		{name: "make equals", arg: "GOFLAGS=-ldflags=-s"},
		{name: "make separated", arg: "GOFLAGS=-ldflags -s"},
		{name: "make double dash", arg: "GOFLAGS=--ldflags=-s"},
		{name: "make single quoted", arg: "GOFLAGS='-ldflags=-s'"},
		{name: "make double quoted", arg: `GOFLAGS="--ldflags=-s"`},
		{name: "make package pattern", arg: "GOFLAGS=-ldflags=github.com/vbonnet/dear-agent/...=-s"},
		{name: "make carriage return", arg: "GOFLAGS=-p=2\r--ldflags=-s"},
		{name: "process environment", env: map[string]string{"GOFLAGS": "--ldflags -s"}},
		{name: "process newline", env: map[string]string{"GOFLAGS": "-p=2\n'-ldflags=-s'"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertRejectedBeforeBuild(t, tc.env, tc.arg)
		})
	}

	t.Run("persisted GOENV", func(t *testing.T) {
		assertRejectedBeforeBuild(t, map[string]string{"GOENV": writeGOENV(t, "-ldflags=-s")}, "")
	})
}

func TestGovernedBuildRejectsUnsafeProtectedMetadataBeforeBuilding(t *testing.T) {
	requireMake(t)
	tests := []struct {
		name     string
		variable string
		value    string
	}{
		{
			name:     "linker whitespace injection",
			variable: "BUILD_DATE",
			value:    "date' -X '" + versionPackage + ".Version=overridden'",
		},
		{name: "version single quote", variable: "VERSION", value: "release'"},
		{name: "commit double quote", variable: "GIT_COMMIT", value: `commit"`},
		{name: "date tab", variable: "BUILD_DATE", value: testBuildDate + "\t-X"},
		{name: "version newline", variable: "VERSION", value: testBuildVersion + "\n-X"},
		{name: "commit carriage return", variable: "GIT_COMMIT", value: testBuildCommit + "\r-X"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			outputPath := filepath.Join(t.TempDir(), "rejected-metadata-probe")
			args := []string{
				"build-stamp-test-probe",
				"BUILD_STAMP_TEST_OUTPUT=" + outputPath,
				"VERSION=" + testBuildVersion,
				"GIT_COMMIT=" + testBuildCommit,
				"BUILD_DATE=" + testBuildDate,
			}
			for i := range args {
				if strings.HasPrefix(args[i], tc.variable+"=") {
					args[i] = tc.variable + "=" + tc.value
				}
			}
			output, err := runMake(t, nil, args...)
			if err == nil {
				t.Fatalf("unsafe protected metadata unexpectedly built probe:\n%s", output)
			}
			wantError := "build stamp metadata " + tc.variable + " must not contain space tab newline carriage return or quote"
			if !strings.Contains(output, wantError) {
				t.Fatalf("unsafe metadata rejection is not actionable:\n%s", output)
			}
			if _, statErr := os.Stat(outputPath); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("rejected metadata created output %s (stat error %v)", outputPath, statErr)
			}
		})
	}
}

func TestGovernedBuildRejectsPatternedExtraLinkerFlagsBeforeBuilding(t *testing.T) {
	requireMake(t)
	outputPath := filepath.Join(t.TempDir(), "rejected-patterned-linker-probe")
	output, err := runMake(t, nil,
		"build-stamp-test-probe",
		"BUILD_STAMP_TEST_OUTPUT="+outputPath,
		"VERSION="+testBuildVersion,
		"GIT_COMMIT="+testBuildCommit,
		"BUILD_DATE="+testBuildDate,
		"EXTRA_GO_LDFLAGS=example.invalid=-s",
		"_NORMALIZED_EXTRA_GO_LDFLAGS=-s",
		"_INVALID_EXTRA_GO_LDFLAGS=",
	)
	if err == nil {
		t.Fatalf("patterned optional linker flags unexpectedly built probe:\n%s", output)
	}
	for _, want := range []string{"EXTRA_GO_LDFLAGS must be an unpatterned linker arg list", "package-pattern forms are unsupported"} {
		if !strings.Contains(output, want) {
			t.Errorf("pattern rejection is missing %q:\n%s", want, output)
		}
	}
	if _, statErr := os.Stat(outputPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("rejected patterned linker flags created output %s (stat error %v)", outputPath, statErr)
	}
}

func TestGovernedBuildRecipesUseSharedStampSeam(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(sourceRoot(t), "Makefile"))
	if err != nil {
		t.Fatal(err)
	}

	governedBuilds := 0
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "\t") || !strings.Contains(line, "go build") || !strings.Contains(line, "-o ") {
			continue
		}
		governedBuilds++
		if !strings.Contains(line, "$(BUILD_STAMP_FLAGS)") {
			t.Errorf("governed build bypasses shared stamp seam: %s", strings.TrimSpace(line))
		}
		if strings.Contains(line, "$(GOFLAGS)") {
			t.Errorf("governed build duplicates caller GOFLAGS in argv: %s", strings.TrimSpace(line))
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if governedBuilds == 0 {
		t.Fatal("Makefile scanner found no governed Go builds")
	}
}

func buildProbe(t *testing.T, env map[string]string, extraArgs ...string) buildStamp {
	t.Helper()
	outputPath := filepath.Join(t.TempDir(), "build-stamp-probe")
	args := []string{
		"build-stamp-test-probe",
		"BUILD_STAMP_TEST_OUTPUT=" + outputPath,
		"VERSION=" + testBuildVersion,
		"GIT_COMMIT=" + testBuildCommit,
		"BUILD_DATE=" + testBuildDate,
	}
	args = append(args, extraArgs...)
	output, err := runMake(t, env, args...)
	if err != nil {
		t.Fatalf("build stamp probe failed: %v\n%s", err, output)
	}

	raw, err := exec.Command(outputPath).Output()
	if err != nil {
		t.Fatalf("run build stamp probe: %v", err)
	}
	var got buildStamp
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode build stamp probe %q: %v", raw, err)
	}
	return got
}

func assertRejectedBeforeBuild(t *testing.T, env map[string]string, goflagsArg string) {
	t.Helper()
	outputPath := filepath.Join(t.TempDir(), "rejected-probe")
	args := []string{
		"build-stamp-test-probe",
		"BUILD_STAMP_TEST_OUTPUT=" + outputPath,
		"VERSION=" + testBuildVersion,
		"GIT_COMMIT=" + testBuildCommit,
		"BUILD_DATE=" + testBuildDate,
	}
	if goflagsArg != "" {
		args = append(args, goflagsArg)
	}
	output, err := runMake(t, env, args...)
	if err == nil {
		t.Fatalf("competing GOFLAGS unexpectedly built probe:\n%s", output)
	}
	for _, want := range []string{"GOFLAGS must not contain -ldflags or --ldflags", "EXTRA_GO_LDFLAGS"} {
		if !strings.Contains(output, want) {
			t.Errorf("rejection is missing %q:\n%s", want, output)
		}
	}
	if _, statErr := os.Stat(outputPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("rejected build created output %s (stat error %v)", outputPath, statErr)
	}
}

func assertBuildStamp(t *testing.T, got buildStamp, extra, marker string) {
	t.Helper()
	want := buildStamp{
		Version:       testBuildVersion,
		GitCommit:     testBuildCommit,
		BuildDate:     testBuildDate,
		BuiltBy:       "makefile",
		Extra:         extra,
		GOFLAGSMarker: marker,
	}
	if got != want {
		t.Fatalf("build stamp = %+v, want %+v", got, want)
	}
}

func writeGOENV(t *testing.T, goflags string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "goenv")
	cmd := exec.Command("go", "env", "-w", "GOFLAGS="+goflags)
	cmd.Env = isolatedEnvironment(map[string]string{"GOENV": path})
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("write temporary GOENV: %v\n%s", err, output)
	}
	check := exec.Command("go", "env", "GOFLAGS")
	check.Env = isolatedEnvironment(map[string]string{"GOENV": path})
	output, err := check.CombinedOutput()
	if err != nil {
		t.Fatalf("read temporary GOENV: %v\n%s", err, output)
	}
	if got := strings.TrimSpace(string(output)); got != goflags {
		t.Fatalf("temporary GOENV GOFLAGS = %q, want %q", got, goflags)
	}
	return path
}

func runMake(t *testing.T, env map[string]string, args ...string) (string, error) {
	t.Helper()
	return runMakeAt(t, sourceRoot(t), env, args...)
}

func runMakeAt(t *testing.T, dir string, env map[string]string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("make", append([]string{"--no-print-directory"}, args...)...)
	cmd.Dir = dir
	cmd.Env = isolatedEnvironment(env)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func runCommand(t *testing.T, env map[string]string, name string, args ...string) (string, error) {
	t.Helper()
	return runCommandAt(t, sourceRoot(t), env, name, args...)
}

func runCommandAt(t *testing.T, dir string, env map[string]string, name string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = isolatedEnvironment(env)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func isolatedEnvironment(overrides map[string]string) []string {
	values := map[string]string{
		"GOENV":  "off",
		"GOWORK": "off",
	}
	maps.Copy(values, overrides)
	blocked := map[string]bool{
		"GOENV": true, "GOFLAGS": true, "GOWORK": true,
		"MAKEFLAGS": true, "MFLAGS": true, "MAKEOVERRIDES": true,
	}
	env := make([]string, 0, len(os.Environ())+len(values))
	for _, entry := range os.Environ() {
		key, _, found := strings.Cut(entry, "=")
		if found && !blocked[key] {
			env = append(env, entry)
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		env = append(env, key+"="+values[key])
	}
	return env
}

func sourceRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve buildstamp test source path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func requireMake(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("root Makefile contract is not supported on Windows")
	}
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make is not installed")
	}
}
