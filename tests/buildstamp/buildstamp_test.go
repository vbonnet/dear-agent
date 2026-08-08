package buildstamp_test

import (
	"bufio"
	"crypto/sha256"
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

	"github.com/vbonnet/dear-agent/internal/gittest"
)

const (
	testBuildVersion        = "9.9.9"
	testBuildCommit         = "0123456789ab"
	testBuildDate           = "2026-08-01T00:00:00Z"
	unknownGitCommitForTest = "unknown"
	versionPackage          = "github.com/vbonnet/dear-agent/pkg/version"
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

func TestGovernedBuildPreservesQuotedTOOLEXECField(t *testing.T) {
	requireMake(t)
	tempDir := t.TempDir()
	wrapperPath := filepath.Join(tempDir, "toolexec-wrapper")
	markerPath := filepath.Join(tempDir, "toolexec-invocations")
	wrapper := `#!/bin/sh
set -eu
if test "$#" -lt 2 || test "$1" != "-ldflags-helper"; then
	echo "unexpected toolexec wrapper arguments" >&2
	exit 2
fi
shift
printf '%s\n' "$1" >> "$BUILDSTAMP_TOOLEXEC_MARKER"
exec "$@"
`
	if err := os.WriteFile(wrapperPath, []byte(wrapper), 0o700); err != nil {
		t.Fatal(err)
	}

	goflags := "'-toolexec=" + wrapperPath + " -ldflags-helper'"
	got := buildProbe(t, map[string]string{
		"BUILDSTAMP_TOOLEXEC_MARKER": markerPath,
		"GOCACHE":                    filepath.Join(tempDir, "gocache"),
	}, "GOFLAGS="+goflags)
	assertBuildStamp(t, got, "unset", "disabled")

	invocations, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("read toolexec liveness marker: %v", err)
	}
	if len(strings.TrimSpace(string(invocations))) == 0 {
		t.Fatal("quoted GOFLAGS build succeeded without invoking the requested toolexec wrapper")
	}
}

func TestWhitespaceOnlyDirectGOFLAGSShadowsPersistedGOENV(t *testing.T) {
	requireMake(t)
	goenv := writeGOENV(t, "-ldflags=-s")
	got := buildProbe(t, map[string]string{
		"GOENV":   goenv,
		"GOFLAGS": " \t\r\n",
	})
	assertBuildStamp(t, got, "unset", "disabled")
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
		"build-stamp-goflags-guard",
		"GOFLAGS=$(info "+marker+")-p=2",
	)
	if err != nil {
		t.Fatalf("opaque GOFLAGS guard failed: %v\n%s", err, output)
	}
	if strings.Contains(output, marker) {
		t.Fatalf("Make evaluated caller GOFLAGS as Make syntax:\n%s", output)
	}
}

func TestCallerGitCommitOverrideBypassesDefaultGitDiscovery(t *testing.T) {
	requireMake(t)
	tempDir := t.TempDir()
	marker := filepath.Join(tempDir, "git-was-invoked")
	fakeGit := filepath.Join(tempDir, "git")
	if err := os.WriteFile(fakeGit, []byte("#!/bin/sh\n: > \"$BUILDSTAMP_GIT_MARKER\"\nexit 97\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	got := buildProbe(t, map[string]string{
		"BUILDSTAMP_GIT_MARKER": marker,
		"PATH":                  tempDir + string(os.PathListSeparator) + os.Getenv("PATH"),
	}, "GOFLAGS=-buildvcs=false")
	assertBuildStamp(t, got, "unset", "disabled")
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("caller GIT_COMMIT override invoked default Git discovery (stat error %v)", err)
	}
}

func TestDefaultGitCommitFlowsThroughGovernedMakeBuild(t *testing.T) {
	requireMake(t)

	tests := []struct {
		name       string
		mutateRepo func(t *testing.T, sandbox *gittest.Sandbox, repo string)
		buildEnv   func(t *testing.T) map[string]string
		wantCommit func(revision string) string
	}{
		{
			name:       "clean branch",
			wantCommit: func(revision string) string { return revision },
		},
		{
			name: "tracked modification",
			mutateRepo: func(t *testing.T, _ *gittest.Sandbox, repo string) {
				t.Helper()
				path := filepath.Join(repo, "pkg", "version", "version.go")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, append(data, []byte("\n// tracked build input change\n")...), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			wantCommit: func(revision string) string { return revision + "-dirty" },
		},
		{
			name: "untracked Go input",
			mutateRepo: func(t *testing.T, _ *gittest.Sandbox, repo string) {
				t.Helper()
				path := filepath.Join(repo, "tests", "buildstamp", "testdata", "probe", "untracked_default.go")
				if err := os.WriteFile(path, []byte("package main\n\nconst untrackedBuildInput = true\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			wantCommit: func(revision string) string { return revision + "-dirty" },
		},
		{
			name: "detached HEAD",
			mutateRepo: func(t *testing.T, sandbox *gittest.Sandbox, repo string) {
				t.Helper()
				sandbox.Run(t, repo, "checkout", "--detach", "HEAD")
			},
			wantCommit: func(revision string) string { return revision },
		},
		{
			name: "status failure",
			buildEnv: func(t *testing.T) map[string]string {
				t.Helper()
				realGit, err := exec.LookPath("git")
				if err != nil {
					t.Fatal(err)
				}
				shimDir := t.TempDir()
				shim := filepath.Join(shimDir, "git")
				const script = "#!/bin/sh\nif [ \"$1\" = status ]; then\n\texit 97\nfi\nexec \"$BUILDSTAMP_REAL_GIT\" \"$@\"\n"
				if err := os.WriteFile(shim, []byte(script), 0o700); err != nil {
					t.Fatal(err)
				}
				return map[string]string{
					"BUILDSTAMP_REAL_GIT": realGit,
					"PATH":                shimDir + string(os.PathListSeparator) + os.Getenv("PATH"),
				}
			},
			wantCommit: func(string) string { return unknownGitCommitForTest },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sandbox := gittest.New(t)
			repo := newMinimalBuildStampRepo(t, sandbox)
			revision := strings.TrimSpace(sandbox.Run(t, repo, "rev-parse", "--short=12", "HEAD"))
			if tc.mutateRepo != nil {
				tc.mutateRepo(t, sandbox, repo)
			}
			var env map[string]string
			if tc.buildEnv != nil {
				env = tc.buildEnv(t)
			}

			got := buildDefaultGitProbe(t, repo, env)
			want := buildStamp{
				Version:       testBuildVersion,
				GitCommit:     tc.wantCommit(revision),
				BuildDate:     testBuildDate,
				BuiltBy:       "makefile",
				Extra:         "unset",
				GOFLAGSMarker: "disabled",
			}
			if got != want {
				t.Fatalf("default Git build stamp = %+v, want %+v", got, want)
			}
		})
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
		"_BUILD_STAMP_RAW_GOFLAGS=-ldflags=-s",
		"_BUILD_STAMP_RAW_GOENV="+filepath.Join(t.TempDir(), "hijacked-goenv"),
		"_MANDATORY_VERSION_LDFLAGS=-s",
		"_NORMALIZED_EXTRA_GO_LDFLAGS=example.invalid=-s",
		"_INVALID_EXTRA_GO_LDFLAGS=yes",
		"_BUILD_STAMP_LDFLAGS=-s",
		"_GOVERNED_BUILD_TARGETS=",
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
		{name: "make adjacent quoted fields", arg: "GOFLAGS='-p=2''--ldflags=-s'"},
		{name: "make adjacent quoted and plain fields", arg: "GOFLAGS='-p=2'--ldflags=-s"},
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

func TestGovernedBuildRejectsMalformedGOFLAGSBeforeBuilding(t *testing.T) {
	requireMake(t)
	outputPath := filepath.Join(t.TempDir(), "malformed-goflags-probe")
	output, err := runMake(t, map[string]string{
		"GOFLAGS": "'-toolexec=/tmp/wrapper -ldflags-helper",
	},
		"build-stamp-test-probe",
		"BUILD_STAMP_TEST_OUTPUT="+outputPath,
		"VERSION="+testBuildVersion,
		"GIT_COMMIT="+testBuildCommit,
		"BUILD_DATE="+testBuildDate,
	)
	if err == nil {
		t.Fatalf("malformed GOFLAGS unexpectedly built probe:\n%s", output)
	}
	if !strings.Contains(output, "GOFLAGS uses invalid Go quoted-field syntax") {
		t.Fatalf("malformed GOFLAGS rejection is not actionable:\n%s", output)
	}
	if strings.Contains(output, "GOFLAGS must not contain -ldflags") {
		t.Fatalf("malformed wrapper argument was misclassified as linker ingress:\n%s", output)
	}
	if _, statErr := os.Stat(outputPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("malformed GOFLAGS created output %s (stat error %v)", outputPath, statErr)
	}
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

	const registryPrefix = "override _GOVERNED_BUILD_TARGETS :="
	declaredTargets := map[string]bool{}
	recipeOwners := map[string]bool{}
	governedBuilds := 0
	readingRegistry := false
	var currentTargets []string
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, registryPrefix) {
			readingRegistry = true
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, registryPrefix))
		}
		if readingRegistry {
			continued := strings.HasSuffix(trimmed, "\\")
			trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "\\"))
			for target := range strings.FieldsSeq(trimmed) {
				declaredTargets[target] = true
			}
			readingRegistry = continued
			continue
		}
		if !strings.HasPrefix(line, "\t") && !strings.HasPrefix(trimmed, "#") {
			beforeColon, afterColon, found := strings.Cut(line, ":")
			if found && !strings.HasPrefix(afterColon, "=") && !strings.Contains(beforeColon, "=") {
				currentTargets = strings.Fields(strings.TrimSpace(beforeColon))
			}
		}
		if !strings.HasPrefix(line, "\t") || !strings.Contains(line, "go build") || !strings.Contains(line, "-o ") {
			continue
		}
		governedBuilds++
		if len(currentTargets) == 0 {
			t.Errorf("governed build has no parseable owning target: %s", strings.TrimSpace(line))
		}
		for _, target := range currentTargets {
			recipeOwners[target] = true
		}
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
	if missing := sortedSetDifference(recipeOwners, declaredTargets); len(missing) > 0 {
		t.Errorf("governed build targets missing from _GOVERNED_BUILD_TARGETS: %v", missing)
	}
	if stale := sortedSetDifference(declaredTargets, recipeOwners); len(stale) > 0 {
		t.Errorf("_GOVERNED_BUILD_TARGETS entries without a governed build recipe: %v", stale)
	}
	if !strings.Contains(string(data), "$(_GOVERNED_BUILD_TARGETS): | build-stamp-goflags-guard") {
		t.Error("governed target registry is not attached to build-stamp-goflags-guard")
	}
	if !strings.Contains(string(data), "GOFLAGS=-buildvcs=false GOENV=off GOWORK=off GOOS= GOARCH= go run ./internal/buildstamp") {
		t.Error("build-stamp GOFLAGS guard bootstrap does not isolate caller Go configuration")
	}
	if !strings.Contains(string(data), "go run ./internal/buildstamp git-commit") {
		t.Error("default Git commit does not use the internal shell-free provenance adapter")
	}
	for _, required := range []string{
		`source_date="$$(git show -s --format=%cI HEAD 2>/dev/null || printf unknown)"`,
		`export _BUILD_STAMP_DATE="$$source_date"`,
		`mkdir -p "$(dir $(SPEC_CONTRACT_HOOK_ARTIFACT))"`,
		`CGO_ENABLED=0 GOWORK=off go build -trimpath -buildvcs=false $(BUILD_STAMP_FLAGS) -o "$(SPEC_CONTRACT_HOOK_ARTIFACT)"`,
		`status-spec-contract-hook: build-spec-contract-hook build-spec-contract-hook-status`,
		`build-agm: build-spec-contract-hook`,
		`-X github.com/vbonnet/dear-agent/agm/internal/codexhooks.expectedSpecContractHookSHA256=$$expected_spec_hook_hash`,
	} {
		if !strings.Contains(string(data), required) {
			t.Errorf("SPEC helper expected-artifact build omits reproducibility seam %q", required)
		}
	}
}

func TestSpecContractHookExpectedArtifactIsReproducible(t *testing.T) {
	requireMake(t)
	root := sourceRoot(t)
	first := filepath.Join(t.TempDir(), "first", "spec-contract-hook")
	second := filepath.Join(t.TempDir(), "second", "spec-contract-hook")
	build := func(artifact, wallClockDate string) [sha256.Size]byte {
		t.Helper()
		output, err := runMakeAt(t, root, nil,
			"build-spec-contract-hook",
			"SPEC_CONTRACT_HOOK_ARTIFACT="+artifact,
			"VERSION="+testBuildVersion,
			"GIT_COMMIT="+testBuildCommit,
			"BUILD_DATE="+wallClockDate,
			"GOFLAGS=-p=2",
		)
		if err != nil {
			t.Fatalf("build reproducible SPEC helper: %v\n%s", err, output)
		}
		body, err := os.ReadFile(artifact)
		if err != nil {
			t.Fatal(err)
		}
		return sha256.Sum256(body)
	}
	firstDigest := build(first, "2026-01-01T00:00:00Z")
	secondDigest := build(second, "2026-12-31T23:59:59Z")
	if firstDigest != secondDigest {
		t.Fatalf("source-identical SPEC helper digests differ: %x != %x", firstDigest, secondDigest)
	}
}

func TestSpecContractHookStatusArtifactPreservesDirectExitContract(t *testing.T) {
	requireMake(t)
	root := sourceRoot(t)
	directory := t.TempDir()
	expected := filepath.Join(directory, "spec-contract-hook")
	statusArtifact := filepath.Join(directory, "spec-contract-hook-status")
	missing := filepath.Join(directory, "missing-deployment")
	output, err := runMakeAt(t, root, nil,
		"build-spec-contract-hook",
		"build-spec-contract-hook-status",
		"SPEC_CONTRACT_HOOK_ARTIFACT="+expected,
		"SPEC_CONTRACT_HOOK_STATUS_ARTIFACT="+statusArtifact,
		"VERSION="+testBuildVersion,
		"GIT_COMMIT="+testBuildCommit,
		"BUILD_DATE="+testBuildDate,
		"GOFLAGS=-p=2",
	)
	if err != nil {
		t.Fatalf("build SPEC helper status artifacts: %v\n%s", err, output)
	}

	command := exec.Command(statusArtifact, "--artifact", expected, "--deployed", missing)
	var stderr strings.Builder
	command.Stderr = &stderr
	stdout, runErr := command.Output()
	var exitErr *exec.ExitError
	if !errors.As(runErr, &exitErr) || exitErr.ExitCode() != 1 {
		t.Fatalf("direct missing-helper status error = %v, want exit 1; stdout=%s stderr=%s", runErr, stdout, stderr.String())
	}
	if stderr.Len() != 0 || strings.Contains(string(stdout), "exit status 1") || strings.Count(string(stdout), "\n") != 1 {
		t.Fatalf("direct status transport was not one clean JSON line: stdout=%q stderr=%q", stdout, stderr.String())
	}
	var status struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(stdout, &status); err != nil || status.Status != "missing" {
		t.Fatalf("direct status output = %q, status=%q, error=%v", stdout, status.Status, err)
	}
}

func TestAGMFamilyBuildEntrypointsUseSharedVersionPackage(t *testing.T) {
	entrypoints := []string{
		".github/workflows/ci.yml",
		".github/workflows/cross-platform-test.yml",
		"scripts/preflight.sh",
	}
	canonicalAssignments := []string{
		"-X " + versionPackage + ".Version=",
		"-X " + versionPackage + ".GitCommit=",
		"-X " + versionPackage + ".BuildDate=",
		"-X " + versionPackage + ".BuiltBy=",
	}
	obsoleteAssignments := []string{
		"-X main.Version=",
		"-X main.GitCommit=",
		"-X main.BuildDate=",
		"-X main.BuiltBy=",
	}

	for _, entrypoint := range entrypoints {
		t.Run(strings.ReplaceAll(entrypoint, "/", "_"), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(sourceRoot(t), filepath.FromSlash(entrypoint)))
			if err != nil {
				t.Fatal(err)
			}
			contents := string(data)
			for _, assignment := range canonicalAssignments {
				if !strings.Contains(contents, assignment) {
					t.Errorf("%s is missing canonical linker assignment %q", entrypoint, assignment)
				}
			}
			for _, assignment := range obsoleteAssignments {
				if strings.Contains(contents, assignment) {
					t.Errorf("%s retains obsolete linker assignment %q", entrypoint, assignment)
				}
			}
		})
	}
}

func buildProbe(t *testing.T, env map[string]string, extraArgs ...string) buildStamp {
	t.Helper()
	args := []string{
		"VERSION=" + testBuildVersion,
		"GIT_COMMIT=" + testBuildCommit,
		"BUILD_DATE=" + testBuildDate,
	}
	args = append(args, extraArgs...)
	return buildProbeAt(t, sourceRoot(t), env, args...)
}

func buildDefaultGitProbe(t *testing.T, repo string, env map[string]string) buildStamp {
	t.Helper()
	return buildProbeAt(t, repo, env,
		"VERSION="+testBuildVersion,
		"BUILD_DATE="+testBuildDate,
		"GOFLAGS=-buildvcs=false",
	)
}

func buildProbeAt(t *testing.T, repo string, env map[string]string, args ...string) buildStamp {
	t.Helper()
	outputPath := filepath.Join(t.TempDir(), "build-stamp-probe")
	makeArgs := []string{
		"build-stamp-test-probe",
		"BUILD_STAMP_TEST_OUTPUT=" + outputPath,
	}
	makeArgs = append(makeArgs, args...)
	output, err := runMakeAt(t, repo, env, makeArgs...)
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

func newMinimalBuildStampRepo(t *testing.T, sandbox *gittest.Sandbox) string {
	t.Helper()
	repo := sandbox.NewRepo(t)
	source := sourceRoot(t)
	for _, path := range []string{
		"Makefile",
		"go.mod",
		"go.sum",
		"mk/install-go-bin.mk",
		"internal/buildstamp/git.go",
		"internal/buildstamp/main.go",
		"pkg/version/staleness.go",
		"pkg/version/version.go",
		"tests/buildstamp/testdata/probe/goflags_disabled.go",
		"tests/buildstamp/testdata/probe/goflags_enabled.go",
		"tests/buildstamp/testdata/probe/main.go",
	} {
		copyBuildSandboxEntry(t, filepath.Join(source, filepath.FromSlash(path)), filepath.Join(repo, filepath.FromSlash(path)))
	}
	sandbox.Run(t, repo, "add", ".")
	sandbox.Run(t, repo, "commit", "-m", "build stamp fixture")
	return repo
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
	for key := range overrides {
		blocked[key] = true
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

func sortedSetDifference(left, right map[string]bool) []string {
	var difference []string
	for value := range left {
		if !right[value] {
			difference = append(difference, value)
		}
	}
	sort.Strings(difference)
	return difference
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
