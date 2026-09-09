package buildauthority

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestNonSourcePlansAreClosedNominalValues(t *testing.T) {
	environment, claim := testPreallocationPlanInputs(t)
	plans, err := newNonSourcePlans(environment, claim)
	if err != nil {
		t.Fatalf("construct non-source plans: %v", err)
	}
	if !plans.valid() {
		t.Fatal("constructed non-source plans are invalid")
	}
	if (nonSourcePlans{}).valid() {
		t.Fatal("zero non-source plans are valid")
	}

	planTypes := []reflect.Type{
		reflect.TypeFor[goVersionPlan](),
		reflect.TypeFor[goEnvironmentPlan](),
		reflect.TypeFor[compilerVersionPlan](),
		reflect.TypeFor[gitVersionPlan](),
		reflect.TypeFor[gitBuiltinInventoryPlan](),
	}
	for left, planType := range planTypes {
		if planType.Kind() != reflect.Struct || planType.Name() == "" {
			t.Fatalf("plan type %s is not a named concrete struct", planType)
		}
		for right := left + 1; right < len(planTypes); right++ {
			if planType == planTypes[right] || planType.ConvertibleTo(planTypes[right]) {
				t.Fatalf("plan types %s and %s are interchangeable", planType, planTypes[right])
			}
		}
		for field := range planType.Fields() {
			if field.Anonymous || field.PkgPath == "" ||
				field.Type.Kind() == reflect.Func ||
				field.Type.Kind() == reflect.Interface ||
				field.Type.Kind() == reflect.String ||
				field.Type.Kind() == reflect.Slice ||
				field.Type == reflect.TypeFor[processRequest]() {
				t.Fatalf("plan %s field %s exposes an open command seam", planType, field.Name)
			}
		}
	}
	bundleType := reflect.TypeFor[nonSourcePlans]()
	if bundleType.NumField() != 6 {
		t.Fatalf("non-source bundle fields = %d, want five plans plus seal", bundleType.NumField())
	}
	for index, want := range planTypes {
		if bundleType.Field(index).Type != want {
			t.Fatalf("bundle field %d = %s, want %s", index, bundleType.Field(index).Type, want)
		}
	}

	wrong := mustTestPreallocationEnvironment(
		t,
		"/private/other-go",
		"/private/other-modules",
	)
	if _, err := newGoVersionPlan(environment, wrong.authorities.goExecutable); err == nil {
		t.Fatal("Go plan admitted an authority from another environment")
	}
	if _, err := newGitVersionPlan(environment, wrong.authorities.gitExecutable); err == nil {
		t.Fatal("Git plan admitted an authority from another environment")
	}
	if _, err := newNonSourcePlans(environment, nil); err == nil {
		t.Fatal("plans admitted no GOROOT go.env claim")
	}
	foreignClaim := *claim
	foreignClaim.goroot = wrong.authorities.goroot
	if _, err := newNonSourcePlans(environment, &foreignClaim); err == nil {
		t.Fatal("plans admitted a foreign GOROOT go.env claim")
	}
	wrongDigest := *claim
	wrongDigest.gorootDigest[0] ^= 0xff
	if _, err := newNonSourcePlans(environment, &wrongDigest); err == nil {
		t.Fatal("plans admitted a go.env claim with the wrong GOROOT digest")
	}
}

func TestRequiredGoEnvironmentEntryHasExactAttribution(t *testing.T) {
	environment, claim := testPreallocationPlanInputs(t)
	goroot := environment.authorities.goroot
	entry, failure := requiredGoEnvironmentEntry(goroot)
	if failure != nil || entry != claim.entry {
		t.Fatalf("required go.env entry = %+v / %+v", entry, failure)
	}

	missing := *goroot
	missing.entries = make([]entrySnapshot, 0, len(goroot.entries)-1)
	for _, candidate := range goroot.entries {
		if candidate.path != goEnvironmentFileName {
			missing.entries = append(missing.entries, candidate)
		}
	}
	_, failure = requiredGoEnvironmentEntry(&missing)
	requireAuthorityRecord(t, failure, OperationOpen, CauseNotFound)

	duplicate := *goroot
	duplicate.entries = append([]entrySnapshot(nil), goroot.entries...)
	duplicate.entries = append(duplicate.entries, claim.entry)
	_, failure = requiredGoEnvironmentEntry(&duplicate)
	requireAuthorityRecord(t, failure, OperationValidate, CauseInternalInvariant)

	wrongKind := *goroot
	wrongKind.entries = append([]entrySnapshot(nil), goroot.entries...)
	for index := range wrongKind.entries {
		if wrongKind.entries[index].path == goEnvironmentFileName {
			wrongKind.entries[index].kind = entryDirectory
		}
	}
	_, failure = requiredGoEnvironmentEntry(&wrongKind)
	requireAuthorityRecord(t, failure, OperationValidate, CauseUnsupported)

	_, failure = requiredGoEnvironmentEntry(&treeCapture{})
	requireAuthorityRecord(t, failure, OperationValidate, CauseInternalInvariant)
}

func TestUnownedPreflightAuthoritiesHaveNoProductionConstructors(t *testing.T) {
	targets := map[string]bool{
		"darwinProcessRun":       true,
		"goEnvironmentFileClaim": true,
		"processRequest":         true,
	}
	owners := map[string]map[string]bool{
		"darwinProcessRun":       {"process_darwin.go": true},
		"goEnvironmentFileClaim": {"preflight.go": true},
		"processRequest": {
			"process.go":        true,
			"process_darwin.go": true,
		},
	}
	allowedContainers := map[string]map[string]bool{
		"goEnvironmentPlan": {"goEnvironmentFileClaim": true},
		"processSupervisor": {"processRequest": true},
	}
	darwinRunLiterals := 0
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read buildauthority package: %v", err)
	}
	files := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(files, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			switch current := node.(type) {
			case *ast.Ident:
				if targets[current.Name] && !owners[current.Name][name] {
					t.Fatalf(
						"%s references unowned authority %s outside its owner",
						files.Position(current.Pos()),
						current.Name,
					)
				}
			case *ast.CompositeLit:
				name := targetTypeName(current.Type, targets)
				if name == "darwinProcessRun" {
					darwinRunLiterals++
					if !canonicalDarwinProcessRunLiteral(name, current) {
						t.Fatalf(
							"%s constructs mutable process-request carrier",
							files.Position(current.Pos()),
						)
					}
					break
				}
				if name != "" {
					t.Fatalf(
						"%s constructs unowned authority %s",
						files.Position(current.Pos()),
						name,
					)
				}
			case *ast.CallExpr:
				if identifier, ok := current.Fun.(*ast.Ident); ok && targets[identifier.Name] {
					t.Fatalf(
						"%s converts to unowned authority %s",
						files.Position(current.Pos()),
						identifier.Name,
					)
				}
				if identifier, ok := current.Fun.(*ast.Ident); ok &&
					(identifier.Name == "new" || identifier.Name == "make") {
					for _, argument := range current.Args {
						if name := targetTypeName(argument, targets); name != "" {
							t.Fatalf(
								"%s allocates unowned authority %s",
								files.Position(current.Pos()),
								name,
							)
						}
					}
				}
				if name := targetTypeName(current.Fun, targets); name != "" {
					t.Fatalf(
						"%s instantiates unowned authority %s",
						files.Position(current.Pos()),
						name,
					)
				}
			case *ast.TypeSpec:
				if targets[current.Name.Name] {
					break
				}
				name := targetTypeName(current.Type, targets)
				if name != "" && !allowedContainers[current.Name.Name][name] {
					t.Fatalf(
						"%s defines surrogate unowned authority %s",
						files.Position(current.Pos()),
						name,
					)
				}
			case *ast.TypeAssertExpr:
				if name := targetTypeName(current.Type, targets); name != "" {
					t.Fatalf(
						"%s asserts unowned authority %s",
						files.Position(current.Pos()),
						name,
					)
				}
			case *ast.ValueSpec:
				if targetTypeName(current.Type, targets) != "" {
					t.Fatalf(
						"%s declares mutable unowned authority %s",
						files.Position(current.Pos()),
						targetTypeName(current.Type, targets),
					)
				}
			case *ast.FuncType:
				if current.Results == nil {
					break
				}
				for _, field := range current.Results.List {
					if name := targetTypeName(field.Type, targets); name != "" {
						t.Fatalf(
							"%s returns unowned authority %s",
							files.Position(current.Pos()),
							name,
						)
					}
				}
			}
			return true
		})
	}
	if darwinRunLiterals != 1 {
		t.Fatalf("production darwinProcessRun literals = %d, want exact owner", darwinRunLiterals)
	}
}

func targetTypeName(expression ast.Expr, targets map[string]bool) string {
	if expression == nil {
		return ""
	}
	found := ""
	ast.Inspect(expression, func(node ast.Node) bool {
		identifier, ok := node.(*ast.Ident)
		if ok && targets[identifier.Name] {
			found = identifier.Name
			return false
		}
		return found == ""
	})
	return found
}

func canonicalDarwinProcessRunLiteral(name string, literal *ast.CompositeLit) bool {
	identifier, ok := literal.Type.(*ast.Ident)
	if !ok || identifier.Name != name {
		return false
	}
	want := map[string]string{
		"supervisor":    "supervisor",
		"request":       "request",
		"command":       "command",
		"pid":           "pid",
		"pipes":         "pipes",
		"notifications": "notifications",
		"workerStop":    "workerStop",
		"stdoutDone":    "stdoutDone",
		"stderrDone":    "stderrDone",
	}
	if len(literal.Elts) != len(want) {
		return false
	}
	seen := make(map[string]bool, len(want))
	for _, element := range literal.Elts {
		field, ok := element.(*ast.KeyValueExpr)
		if !ok {
			return false
		}
		key, keyOK := field.Key.(*ast.Ident)
		value, valueOK := field.Value.(*ast.Ident)
		if !keyOK || !valueOK || seen[key.Name] || want[key.Name] != value.Name {
			return false
		}
		seen[key.Name] = true
	}
	return len(seen) == len(want)
}

func TestPreallocationProbePolicyAndStaticWiresArePinned(t *testing.T) {
	tests := []struct {
		name   string
		wire   string
		length int
		digest string
	}{
		{
			name:   "go",
			wire:   goVersionWire,
			length: 33,
			digest: "c528ba02b39880f1357f51dee70e41c1ef03e3a70c7a03055c080532b9635f58",
		},
		{
			name:   "compiler",
			wire:   compilerVersionWire,
			length: 96,
			digest: "2ecca23a89d79ae35458843c1c1d9f3b037ad8fcfa864184169279c2787fa380",
		},
		{
			name:   "git",
			wire:   gitVersionWire,
			length: 225,
			digest: "70c39c3d0e3fa158a62a5602140a4370d77caa639269a6bd7ec324773d08aafa",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			digest := sha256.Sum256([]byte(test.wire))
			if len(test.wire) != test.length || hex.EncodeToString(digest[:]) != test.digest {
				t.Fatalf("wire length/digest = %d/%x", len(test.wire), digest)
			}
		})
	}
	builtinDigest := gitBuiltinWireDigest()
	if hex.EncodeToString(builtinDigest[:]) !=
		"7712b98c146e53d5a26119533901a748176f9743e60e8675a396c4f103dd8625" {
		t.Fatal("Git builtin digest drifted")
	}
}

func TestGoEnvironmentArgumentsAreExactAndFresh(t *testing.T) {
	want := []string{
		"env",
		"-json",
		"GOROOT",
		"GOMODCACHE",
		"GOCACHE",
		"GOCACHEPROG",
		"GOENV",
		"GOFLAGS",
		"GOWORK",
		"GOTOOLCHAIN",
		"GOPROXY",
		"GOSUMDB",
		"GOVCS",
		"GOTELEMETRY",
		"GOTELEMETRYDIR",
		"GO111MODULE",
		"GOEXPERIMENT",
		"GOFIPS140",
		"GO_EXTLINK_ENABLED",
		"CGO_ENABLED",
		"GOHOSTOS",
		"GOHOSTARCH",
		"GOTOOLDIR",
		"GOVERSION",
		"GOOS",
		"GOARCH",
		"GOAMD64",
		"GOARM64",
	}
	got := goEnvironmentArguments()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Go environment arguments = %q", got)
	}
	got[0] = "tampered"
	if fresh := goEnvironmentArguments(); !reflect.DeepEqual(fresh, want) {
		t.Fatalf("fresh Go environment arguments = %q", fresh)
	}
}

func TestStaticPreallocationOutputValidatorsSplitParseAndCompare(t *testing.T) {
	environment, claim := testPreallocationPlanInputs(t)
	plans, err := newNonSourcePlans(environment, claim)
	if err != nil {
		t.Fatalf("construct non-source plans: %v", err)
	}

	if failure := plans.goVersion.validateOutput([]byte(goVersionWire)); failure != nil {
		t.Fatalf("exact Go version refused: %+v", failure)
	}
	requireAuthorityRecord(
		t,
		plans.goVersion.validateOutput([]byte("go version go1.27.2 darwin/arm64\n")),
		OperationCompare,
		CauseUnsupported,
	)
	requireAuthorityRecord(
		t,
		plans.goVersion.validateOutput([]byte("go version go1.27.1 darwin/arm64")),
		OperationParse,
		CauseMalformed,
	)
	requireAuthorityRecord(
		t,
		plans.goVersion.validateOutput([]byte("go version go1.27.1 darwin/arm64\x07\n")),
		OperationParse,
		CauseMalformed,
	)

	if failure := plans.compilerVersion.validateOutput([]byte(compilerVersionWire)); failure != nil {
		t.Fatalf("exact compiler version refused: %+v", failure)
	}
	requireAuthorityRecord(
		t,
		plans.compilerVersion.validateOutput(
			[]byte("compile version go1.27.2 X:nojsonv2\n"),
		),
		OperationCompare,
		CauseUnsupported,
	)
	requireAuthorityRecord(
		t,
		plans.compilerVersion.validateOutput([]byte("compile\tversion go1.27.1 options\n")),
		OperationParse,
		CauseMalformed,
	)

	if failure := plans.gitVersion.validateOutput([]byte(gitVersionWire)); failure != nil {
		t.Fatalf("exact Git version refused: %+v", failure)
	}
	wrongGit := strings.Replace(gitVersionWire, "cpu: arm64", "cpu: x86_64", 1)
	requireAuthorityRecord(
		t,
		plans.gitVersion.validateOutput([]byte(wrongGit)),
		OperationCompare,
		CauseUnsupported,
	)
	reorderedGit := strings.Replace(
		gitVersionWire,
		"cpu: arm64\nno commit associated with this build\n",
		"no commit associated with this build\ncpu: arm64\n",
		1,
	)
	requireAuthorityRecord(
		t,
		plans.gitVersion.validateOutput([]byte(reorderedGit)),
		OperationParse,
		CauseMalformed,
	)
	doubleSpaceGit := strings.Replace(gitVersionWire, "cpu: arm64", "cpu:  arm64", 1)
	requireAuthorityRecord(
		t,
		plans.gitVersion.validateOutput([]byte(doubleSpaceGit)),
		OperationParse,
		CauseMalformed,
	)

	builtins := pinnedGitBuiltinOutput()
	if len(builtins) != gitBuiltinWireBytes ||
		Digest(sha256.Sum256(builtins)) != gitBuiltinWireDigest() {
		t.Fatal("test Git builtin fixture drifted")
	}
	if failure := plans.gitBuiltins.validateOutput(builtins); failure != nil {
		t.Fatalf("exact Git builtins refused: %+v", failure)
	}
	sameShape := append([]byte(nil), builtins...)
	copy(sameShape[:3], "aaa")
	requireAuthorityRecord(
		t,
		plans.gitBuiltins.validateOutput(sameShape),
		OperationCompare,
		CauseUnsupported,
	)
	missing := bytes.Replace(builtins, []byte("annotate\n"), nil, 1)
	requireAuthorityRecord(
		t,
		plans.gitBuiltins.validateOutput(missing),
		OperationParse,
		CauseMalformed,
	)
	reordered := bytes.Replace(
		builtins,
		[]byte("add\nam\n"),
		[]byte("am\nadd\n"),
		1,
	)
	requireAuthorityRecord(
		t,
		plans.gitBuiltins.validateOutput(reordered),
		OperationParse,
		CauseMalformed,
	)
	requiredMissing := bytes.Replace(builtins, []byte("cat-file\n"), []byte("cat-fild\n"), 1)
	requireAuthorityRecord(
		t,
		plans.gitBuiltins.validateOutput(requiredMissing),
		OperationCompare,
		CauseUnsupported,
	)
}

func TestGoEnvironmentOutputRequiresCanonicalExactProjection(t *testing.T) {
	environment, claim := testPreallocationPlanInputs(t)
	plans, err := newNonSourcePlans(environment, claim)
	if err != nil {
		t.Fatalf("construct non-source plans: %v", err)
	}
	wire, values, err := expectedGoEnvironmentOutput(environment)
	if err != nil {
		t.Fatalf("render expected Go environment: %v", err)
	}
	wantValues := map[string]string{
		"CGO_ENABLED":        "0",
		"GO111MODULE":        "on",
		"GOAMD64":            "",
		"GOARCH":             "arm64",
		"GOARM64":            "v8.0",
		"GOCACHE":            "off",
		"GOCACHEPROG":        "",
		"GOENV":              "",
		"GOEXPERIMENT":       "none",
		"GOFIPS140":          "off",
		"GOFLAGS":            "-mod=readonly",
		"GOHOSTARCH":         "arm64",
		"GOHOSTOS":           "darwin",
		"GOMODCACHE":         "/private/pinned-modules",
		"GOOS":               "darwin",
		"GOPROXY":            "off",
		"GOROOT":             "/private/pinned-go",
		"GOSUMDB":            "off",
		"GOTELEMETRY":        "off",
		"GOTELEMETRYDIR":     "",
		"GOTOOLCHAIN":        "local",
		"GOTOOLDIR":          "/private/pinned-go/pkg/tool/darwin_arm64",
		"GOVCS":              "*:off",
		"GOVERSION":          "go1.27.1",
		"GOWORK":             "off",
		"GO_EXTLINK_ENABLED": "0",
	}
	if !reflect.DeepEqual(values, wantValues) {
		t.Fatalf("Go environment values = %#v", values)
	}
	wantKeyOrder := [26]string{
		"CGO_ENABLED",
		"GO111MODULE",
		"GOAMD64",
		"GOARCH",
		"GOARM64",
		"GOCACHE",
		"GOCACHEPROG",
		"GOENV",
		"GOEXPERIMENT",
		"GOFIPS140",
		"GOFLAGS",
		"GOHOSTARCH",
		"GOHOSTOS",
		"GOMODCACHE",
		"GOOS",
		"GOPROXY",
		"GOROOT",
		"GOSUMDB",
		"GOTELEMETRY",
		"GOTELEMETRYDIR",
		"GOTOOLCHAIN",
		"GOTOOLDIR",
		"GOVCS",
		"GOVERSION",
		"GOWORK",
		"GO_EXTLINK_ENABLED",
	}
	lines := strings.Split(string(wire), "\n")
	if len(lines) != len(wantKeyOrder)+3 || lines[0] != "{" ||
		lines[len(lines)-2] != "}" || lines[len(lines)-1] != "" {
		t.Fatalf("canonical Go environment framing = %q", lines)
	}
	for index, key := range wantKeyOrder {
		if !strings.HasPrefix(lines[index+1], "\t\""+key+"\": ") {
			t.Fatalf("Go environment row %d = %q, want key %s", index, lines[index+1], key)
		}
	}
	if failure := plans.goEnvironment.validateOutput(wire); failure != nil {
		t.Fatalf("exact Go environment refused: %+v", failure)
	}
	if len(values) != 26 || !bytes.HasSuffix(wire, []byte("}\n")) {
		t.Fatalf("Go environment shape = %d keys, final bytes %q", len(values), wire[len(wire)-2:])
	}

	wrongPath := cloneStringMap(values)
	wrongPath["GOROOT"] = "/private/wrong-go"
	requireAuthorityRecord(
		t,
		plans.goEnvironment.validateOutput(mustEncodeStringMap(t, wrongPath)),
		OperationCompare,
		CauseIdentity,
	)
	wrongSetting := cloneStringMap(values)
	wrongSetting["GOPROXY"] = "https://proxy.invalid"
	requireAuthorityRecord(
		t,
		plans.goEnvironment.validateOutput(mustEncodeStringMap(t, wrongSetting)),
		OperationCompare,
		CauseUnsupported,
	)
	missing := cloneStringMap(values)
	delete(missing, "GOAMD64")
	requireAuthorityRecord(
		t,
		plans.goEnvironment.validateOutput(mustEncodeStringMap(t, missing)),
		OperationParse,
		CauseMalformed,
	)
	extra := cloneStringMap(values)
	extra["UNEXPECTED"] = "1"
	requireAuthorityRecord(
		t,
		plans.goEnvironment.validateOutput(mustEncodeStringMap(t, extra)),
		OperationParse,
		CauseMalformed,
	)

	duplicateLine := []byte("\t\"CGO_ENABLED\": \"0\",\n")
	duplicate := bytes.Replace(wire, duplicateLine, append(duplicateLine, duplicateLine...), 1)
	requireAuthorityRecord(
		t,
		plans.goEnvironment.validateOutput(duplicate),
		OperationParse,
		CauseMalformed,
	)
	first := []byte("\t\"CGO_ENABLED\": \"0\",\n")
	second := []byte("\t\"GO111MODULE\": \"on\",\n")
	reordered := bytes.Replace(wire, append(first, second...), append(second, first...), 1)
	requireAuthorityRecord(
		t,
		plans.goEnvironment.validateOutput(reordered),
		OperationParse,
		CauseMalformed,
	)
	compact, err := json.Marshal(values)
	if err != nil {
		t.Fatalf("compact JSON: %v", err)
	}
	requireAuthorityRecord(
		t,
		plans.goEnvironment.validateOutput(append(compact, '\n')),
		OperationParse,
		CauseMalformed,
	)
	wrongThenMalformed := mustEncodeStringMap(t, wrongSetting)
	wrongThenMalformed = bytes.Replace(
		wrongThenMalformed,
		[]byte("\t\"GOROOT\": \"/private/pinned-go\",\n"),
		[]byte("\t\"GOROOT\": <invalid>,\n"),
		1,
	)
	requireAuthorityRecord(
		t,
		plans.goEnvironment.validateOutput(wrongThenMalformed),
		OperationParse,
		CauseMalformed,
	)
}

func TestCanonicalJSONStringMatchesPinnedEncoderForm(t *testing.T) {
	acceptedValues := []string{
		"plain",
		"café \"quoted\" \\ path",
		"\x00\x07\b\t\n\v\f\r\x1f<>&",
		"\u2028\u2029�",
	}
	for _, value := range acceptedValues {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal %q: %v", value, err)
		}
		if !canonicalJSONString(encoded) {
			t.Fatalf("encoder output refused: %q", encoded)
		}
	}

	rawLineSeparator := append([]byte{'"'}, []byte("\u2028")...)
	rawLineSeparator = append(rawLineSeparator, '"')
	rejected := [][]byte{
		[]byte(`"<"`),
		rawLineSeparator,
		[]byte(`"\/"`),
		[]byte(`"\u0061"`),
		[]byte(`"\u003C"`),
		[]byte(`"\ud800"`),
		[]byte(`"\ufffd"`),
		[]byte(`"\u00"`),
		{'"', 0xff, '"'},
	}
	for _, encoded := range rejected {
		if canonicalJSONString(encoded) {
			t.Fatalf("noncanonical JSON string admitted: %q", encoded)
		}
	}
}

func TestStructuredOutputScannersDoNotAllocateFromInput(t *testing.T) {
	const payloadBytes = 1 << 20
	payload := bytes.Repeat([]byte{'x'}, payloadBytes)
	hugeSimple := make([]byte, 0, payloadBytes+len("go version  darwin/arm64\n"))
	hugeSimple = append(hugeSimple, "go version "...)
	hugeSimple = append(hugeSimple, payload...)
	hugeSimple = append(hugeSimple, " darwin/arm64\n"...)
	newlineFlood := bytes.Repeat([]byte{'\n'}, payloadBytes)

	environment, claim := testPreallocationPlanInputs(t)
	plans, err := newNonSourcePlans(environment, claim)
	if err != nil {
		t.Fatalf("construct non-source plans: %v", err)
	}
	wire, _, err := expectedGoEnvironmentOutput(environment)
	if err != nil {
		t.Fatalf("render Go environment: %v", err)
	}
	escaped := make([]byte, 1, 2*payloadBytes+2)
	escaped[0] = '"'
	for range payloadBytes {
		escaped = append(escaped, '\\', 'n')
	}
	escaped = append(escaped, '"')
	replacement := append([]byte("\t\"GOPROXY\": "), escaped...)
	replacement = append(replacement, ',', '\n')
	hugeJSON := bytes.Replace(
		wire,
		[]byte("\t\"GOPROXY\": \"off\",\n"),
		replacement,
		1,
	)
	if len(hugeJSON) <= len(wire)+payloadBytes {
		t.Fatal("large Go environment fixture was not inserted")
	}
	hugeGitVersionRow := append([]byte("cpu: "), payload...)
	hugeGitVersionRow = append(hugeGitVersionRow, '\n')
	hugeGitVersion := bytes.Replace(
		[]byte(gitVersionWire),
		[]byte("cpu: arm64\n"),
		hugeGitVersionRow,
		1,
	)
	hugeBuiltinRow := append([]byte("write-tree"), payload...)
	hugeBuiltinRow = append(hugeBuiltinRow, '\n')
	hugeBuiltins := bytes.Replace(
		pinnedGitBuiltinOutput(),
		[]byte("write-tree\n"),
		hugeBuiltinRow,
		1,
	)

	requireAuthorityRecord(
		t,
		plans.goVersion.validateOutput(hugeSimple),
		OperationCompare,
		CauseUnsupported,
	)
	requireAuthorityRecord(
		t,
		plans.gitVersion.validateOutput(hugeGitVersion),
		OperationCompare,
		CauseUnsupported,
	)
	requireAuthorityRecord(
		t,
		plans.gitBuiltins.validateOutput(hugeBuiltins),
		OperationCompare,
		CauseUnsupported,
	)
	requireAuthorityRecord(
		t,
		plans.goEnvironment.validateOutput(hugeJSON),
		OperationCompare,
		CauseUnsupported,
	)

	checks := []struct {
		name string
		want bool
		run  func() bool
	}{
		{
			name: "simple",
			want: true,
			run: func() bool {
				line, ok := oneCanonicalLine(hugeSimple)
				var fields [4][]byte
				return ok && splitExactByteSeparated(line, ' ', fields[:])
			},
		},
		{
			name: "git-version-huge-value",
			want: true,
			run: func() bool {
				return validGitVersionWireShape(hugeGitVersion)
			},
		},
		{
			name: "git-builtins-huge-token",
			want: true,
			run: func() bool {
				_, ok := parseGitBuiltinLines(hugeBuiltins)
				return ok
			},
		},
		{
			name: "git-version-newline-flood",
			run: func() bool {
				return validGitVersionWireShape(newlineFlood)
			},
		},
		{
			name: "git-builtins-newline-flood",
			run: func() bool {
				_, ok := parseGitBuiltinLines(newlineFlood)
				return ok
			},
		},
		{
			name: "go-json",
			want: true,
			run: func() bool {
				_, ok := parseCanonicalGoEnvironmentOutput(hugeJSON)
				return ok
			},
		},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if got := check.run(); got != check.want {
				t.Fatalf("shape = %t, want %t", got, check.want)
			}
			if allocations := testing.AllocsPerRun(5, func() {
				if check.run() != check.want {
					panic("unstable parser result")
				}
			}); allocations != 0 {
				t.Fatalf("allocations/run = %v, want 0", allocations)
			}
		})
	}
}

func TestGoEnvironmentFileGrammarIsClosed(t *testing.T) {
	valid := []byte(
		"# defaults\n\n" +
			"GOPROXY=https://proxy.golang.org,direct\n" +
			"GOSUMDB=sum.golang.org\n" +
			"# toolchain\n" +
			"GOTOOLCHAIN=auto\n",
	)
	if err := parseGoEnvironmentFile(valid); err != nil {
		t.Fatalf("valid go.env refused: %v", err)
	}
	tests := []struct {
		name    string
		content []byte
		cause   CauseCode
	}{
		{
			name:    "missing",
			content: bytes.Replace(valid, []byte("GOSUMDB=sum.golang.org\n"), nil, 1),
			cause:   CauseMalformed,
		},
		{
			name:    "duplicate",
			content: append(append([]byte(nil), valid...), []byte("GOSUMDB=sum.golang.org\n")...),
			cause:   CauseMalformed,
		},
		{
			name:    "wrong-value",
			content: bytes.Replace(valid, []byte("GOTOOLCHAIN=auto"), []byte("GOTOOLCHAIN=local"), 1),
			cause:   CauseUnsupported,
		},
		{
			name:    "unknown",
			content: append(append([]byte(nil), valid...), []byte("GOFLAGS=-mod=mod\n")...),
			cause:   CauseUnsupported,
		},
		{
			name:    "gocacheprog",
			content: append(append([]byte(nil), valid...), []byte("GOCACHEPROG=helper\n")...),
			cause:   CauseUnsupported,
		},
		{
			name:    "leading-space",
			content: append(append([]byte(nil), valid...), []byte(" comment\n")...),
			cause:   CauseMalformed,
		},
		{
			name:    "carriage-return",
			content: bytes.Replace(valid, []byte("\n"), []byte("\r\n"), 1),
			cause:   CauseMalformed,
		},
		{
			name:    "invalid-utf8",
			content: append(append([]byte(nil), valid...), 0xff),
			cause:   CauseMalformed,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requirePrivateCauses(t, parseGoEnvironmentFile(test.content), test.cause)
		})
	}
}

func TestGoEnvironmentFileParserDoesNotCopyInput(t *testing.T) {
	content := make([]byte, 0, 1<<20)
	content = append(content, '#')
	content = append(content, bytes.Repeat([]byte{'x'}, 1<<20)...)
	content = append(content, '\n')
	content = append(content,
		"GOPROXY=https://proxy.golang.org,direct\n"...,
	)
	content = append(content, "GOSUMDB=sum.golang.org\n"...)
	content = append(content, "GOTOOLCHAIN=auto\n"...)
	if err := parseGoEnvironmentFile(content); err != nil {
		t.Fatalf("large valid go.env refused: %v", err)
	}
	if allocations := testing.AllocsPerRun(5, func() {
		if err := parseGoEnvironmentFile(content); err != nil {
			panic(err)
		}
	}); allocations != 0 {
		t.Fatalf("allocations/run = %v, want 0", allocations)
	}
}

func TestPhysicalRootSentinelsAreFixedAndNoFollow(t *testing.T) {
	root := testSentinelRoot(t)
	var names []string
	var descriptor int
	claim, failure := admitPhysicalRootSentinelsWith(
		context.Background(),
		root,
		func(fd int, name string) (entryKind, error) {
			if len(names) == 0 {
				descriptor = fd
			} else if fd != descriptor {
				t.Fatalf("sentinel descriptor changed from %d to %d", descriptor, fd)
			}
			names = append(names, name)
			return 0, fs.ErrNotExist
		},
	)
	if failure != nil {
		t.Fatalf("absent sentinels refused: %+v", failure)
	}
	if !reflect.DeepEqual(names, []string{"go.mod", "go.work", ".git"}) {
		t.Fatalf("sentinel order = %q", names)
	}
	if failure := claim.revalidateWith(
		context.Background(),
		func(fd int, name string) (entryKind, error) {
			if fd != descriptor {
				t.Fatalf("revalidation descriptor = %d, want %d", fd, descriptor)
			}
			return 0, fs.ErrNotExist
		},
	); failure != nil {
		t.Fatalf("continued sentinel absence refused: %+v", failure)
	}
}

func TestPhysicalRootSentinelFailuresHaveExactAttribution(t *testing.T) {
	root := testSentinelRoot(t)
	for _, present := range []struct {
		name string
		kind entryKind
		err  error
	}{
		{name: "directory", kind: entryDirectory},
		{name: "regular", kind: entryRegular},
		{name: "symlink", kind: entrySymlink},
		{name: "special", err: errUnsupportedAuthorityEntry},
	} {
		t.Run("initial-"+present.name, func(t *testing.T) {
			_, failure := admitPhysicalRootSentinelsWith(
				context.Background(),
				root,
				func(int, string) (entryKind, error) {
					return present.kind, present.err
				},
			)
			requireAuthorityRecord(t, failure, OperationValidate, CauseUnsupported)
		})
	}

	permission := func(int, string) (entryKind, error) {
		return 0, fs.ErrPermission
	}
	_, failure := admitPhysicalRootSentinelsWith(context.Background(), root, permission)
	requireAuthorityRecord(t, failure, OperationProbe, CausePermission)

	unstable := func(int, string) (entryKind, error) {
		return 0, errors.New("lookup changed")
	}
	_, failure = admitPhysicalRootSentinelsWith(context.Background(), root, unstable)
	requireAuthorityRecord(t, failure, OperationProbe, CauseUnstable)

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	_, failure = admitPhysicalRootSentinelsWith(
		canceled,
		root,
		func(int, string) (entryKind, error) {
			calls++
			return 0, fs.ErrNotExist
		},
	)
	requireAuthorityRecord(t, failure, OperationProbe, CauseCanceled)
	if calls != 0 {
		t.Fatalf("sentinel lookup ran %d times after cancellation", calls)
	}

	claim, failure := admitPhysicalRootSentinelsWith(
		context.Background(),
		root,
		func(int, string) (entryKind, error) {
			return 0, fs.ErrNotExist
		},
	)
	if failure != nil {
		t.Fatalf("admit absent sentinels: %+v", failure)
	}
	seen := 0
	failure = claim.revalidateWith(
		context.Background(),
		func(int, string) (entryKind, error) {
			seen++
			if seen == 3 {
				return entryDirectory, nil
			}
			return 0, fs.ErrNotExist
		},
	)
	requireAuthorityRecord(t, failure, OperationCompare, CauseUnstable)
	if seen != 3 {
		t.Fatalf("post-block sentinel lookups = %d, want 3", seen)
	}
	requireAuthorityRecord(
		t,
		(physicalRootSentinelClaim{}).revalidateWith(
			context.Background(),
			func(int, string) (entryKind, error) { return 0, fs.ErrNotExist },
		),
		OperationValidate,
		CauseInternalInvariant,
	)
}

func testPreallocationPlanInputs(
	t *testing.T,
) (preallocationEnvironment, *goEnvironmentFileClaim) {
	t.Helper()
	environment := mustTestPreallocationEnvironment(
		t,
		"/private/pinned-go",
		"/private/pinned-modules",
	)
	content := []byte(
		"GOPROXY=https://proxy.golang.org,direct\n" +
			"GOSUMDB=sum.golang.org\n" +
			"GOTOOLCHAIN=auto\n",
	)
	goroot := environment.authorities.goroot
	snapshot := fileSnapshot{
		identity: FileIdentity{
			Device:     goroot.root.snapshot.identity.Device,
			Inode:      91,
			UID:        goroot.root.pathClaims[0].identity.UID,
			Mode:       platformModeRegular | 0o600,
			Filesystem: goroot.root.mount.filesystem,
		},
		size: int64(len(content)),
	}
	entry := entrySnapshot{
		path:    goEnvironmentFileName,
		kind:    entryRegular,
		file:    snapshot,
		content: Digest(sha256.Sum256(content)),
	}
	goroot.entries = append(goroot.entries, entry)
	sortEntrySnapshots(goroot.entries)
	claim := &goEnvironmentFileClaim{
		goroot:       goroot,
		gorootDigest: goroot.digest,
		entry:        entry,
		seal:         validGoEnvironmentFileClaim,
	}
	if !environment.valid() || !claim.validFor(goroot) {
		t.Fatal("test plan authority fixture is invalid")
	}
	return environment, claim
}

func testSentinelRoot(t *testing.T) *physicalRootAuthority {
	t.Helper()
	root := testEnvironmentAuthorityInputs(
		"/private/go",
		"/private/modules",
	).physicalRoot
	descriptor, err := os.Open(physicalRootPath)
	if err != nil {
		t.Fatalf("open physical root test descriptor: %v", err)
	}
	t.Cleanup(func() {
		if err := descriptor.Close(); err != nil {
			t.Errorf("close physical root test descriptor: %v", err)
		}
	})
	root.directory.descriptor = descriptor
	return root
}

func cloneStringMap(source map[string]string) map[string]string {
	cloned := make(map[string]string, len(source))
	maps.Copy(cloned, source)
	return cloned
}

func mustEncodeStringMap(t *testing.T, values map[string]string) []byte {
	t.Helper()
	wire, err := encodeCanonicalStringMap(values)
	if err != nil {
		t.Fatalf("encode string map: %v", err)
	}
	return wire
}

func requireAuthorityRecord(
	t *testing.T,
	record *FailureRecord,
	operation Operation,
	cause CauseCode,
) {
	t.Helper()
	if record == nil || record.Phase != PhaseAuthority ||
		record.Operation != operation || len(record.Causes) != 1 ||
		record.Causes[0] != cause || record.Child != nil {
		t.Fatalf("record = %+v, want authority/%s/%s", record, operation, cause)
	}
}
func pinnedGitBuiltinOutput() []byte {
	return []byte(`add
am
annotate
apply
archive
backfill
bisect
blame
branch
bugreport
bundle
cat-file
check-attr
check-ignore
check-mailmap
check-ref-format
checkout
checkout--worker
checkout-index
cherry
cherry-pick
clean
clone
column
commit
commit-graph
commit-tree
config
count-objects
credential
credential-cache
credential-cache--daemon
credential-store
describe
diagnose
diff
diff-files
diff-index
diff-pairs
diff-tree
difftool
fast-export
fast-import
fetch
fetch-pack
fmt-merge-msg
for-each-ref
for-each-repo
format-patch
fsck
fsck-objects
fsmonitor--daemon
gc
get-tar-commit-id
grep
hash-object
help
hook
index-pack
init
init-db
interpret-trailers
log
ls-files
ls-remote
ls-tree
mailinfo
mailsplit
maintenance
merge
merge-base
merge-file
merge-index
merge-ours
merge-recursive
merge-recursive-ours
merge-recursive-theirs
merge-subtree
merge-tree
mktag
mktree
multi-pack-index
mv
name-rev
notes
pack-objects
pack-redundant
pack-refs
patch-id
pickaxe
prune
prune-packed
pull
push
range-diff
read-tree
rebase
receive-pack
reflog
refs
remote
remote-ext
remote-fd
repack
replace
replay
rerere
reset
restore
rev-list
rev-parse
revert
rm
send-pack
shortlog
show
show-branch
show-index
show-ref
sparse-checkout
stage
stash
status
stripspace
submodule--helper
switch
symbolic-ref
tag
unpack-file
unpack-objects
update-index
update-ref
update-server-info
upload-archive
upload-archive--writer
upload-pack
var
verify-commit
verify-pack
verify-tag
version
whatchanged
worktree
write-tree
`)
}
