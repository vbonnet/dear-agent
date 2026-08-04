package steps

import (
	"context"
	"strings"
	"testing"
)

func TestCanonicalAGMInstallPlanSteps(t *testing.T) {
	ctx := context.WithValue(context.Background(), hookParityStateKey{}, &hookParityState{})
	if err := agmRendersCanonicalAGMCompanionInstallPlan(ctx); err != nil {
		t.Fatalf("render install plan: %v", err)
	}
	if err := rootAGMInstallPlanShouldBuildAndInstallCompanionPair(ctx); err != nil {
		t.Fatalf("validate install plan: %v", err)
	}
}

func TestValidateCanonicalAGMInstallPlan(t *testing.T) {
	stampFlags := strings.Join([]string{
		"-ldflags",
		"-X github.com/vbonnet/dear-agent/pkg/version.Version=${_BUILD_STAMP_VERSION}",
		"-X github.com/vbonnet/dear-agent/pkg/version.GitCommit=${_BUILD_STAMP_GIT_COMMIT}",
		"-X github.com/vbonnet/dear-agent/pkg/version.BuildDate=${_BUILD_STAMP_DATE}",
		"-X github.com/vbonnet/dear-agent/pkg/version.BuiltBy=makefile",
	}, " ")
	valid := strings.Join([]string{
		"GOFLAGS= GOENV=off GOWORK=off go run ./internal/buildstamp",
		"go build " + stampFlags + " -o bin/agm ./agm/cmd/agm/",
		"go build " + stampFlags + " -o bin/agm-reaper ./agm/cmd/agm-reaper/",
		`set -e; dir='.bdd-install-plan/go/bin'; mkdir -p "$dir"; dest="$dir/agm"; stage="$(mktemp "$dest.XXXXXX")"; trap 'rm -f "$stage"' EXIT; cp 'bin/agm' "$stage"; chmod 755 "$stage"; codesign -f -s - "$stage" 2>/dev/null || true; mv -f "$stage" "$dest"; echo "Installed: $dest"`,
		`set -e; dir='.bdd-install-plan/go/bin'; mkdir -p "$dir"; dest="$dir/agm-reaper"; stage="$(mktemp "$dest.XXXXXX")"; trap 'rm -f "$stage"' EXIT; cp 'bin/agm-reaper' "$stage"; chmod 755 "$stage"; codesign -f -s - "$stage" 2>/dev/null || true; mv -f "$stage" "$dest"; echo "Installed: $dest"`,
	}, "\n")
	if err := validateCanonicalAGMInstallPlan(valid); err != nil {
		t.Fatalf("valid install plan rejected: %v", err)
	}

	tests := []struct {
		name       string
		plan       string
		wantDetail string
	}{
		{
			name:       "missing AGM stamp",
			plan:       strings.Replace(valid, "-X github.com/vbonnet/dear-agent/pkg/version.BuiltBy=makefile", "", 1),
			wantDetail: "pkg/version.BuiltBy=makefile",
		},
		{
			name: "missing reaper promotion",
			plan: strings.Replace(valid,
				`mv -f "$stage" "$dest"`,
				`echo "no mv"`, 2),
			wantDetail: "mv -f",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateCanonicalAGMInstallPlan(tc.plan)
			if err == nil || !strings.Contains(err.Error(), tc.wantDetail) {
				t.Fatalf("invalid install plan error = %v, want detail %q", err, tc.wantDetail)
			}
		})
	}
}

func TestCanonicalAGMInstallPlanEnvironmentExcludesMakeControlInputs(t *testing.T) {
	wantEmpty := map[string]bool{
		"MAKEFLAGS":     true,
		"MFLAGS":        true,
		"MAKEOVERRIDES": true,
		"MAKEFILES":     true,
	}
	seen := make(map[string]bool, len(wantEmpty))
	for _, entry := range canonicalAGMInstallPlanEnvironment() {
		key, value, found := strings.Cut(entry, "=")
		if !found {
			t.Fatalf("malformed environment entry %q", entry)
		}
		if wantEmpty[key] {
			seen[key] = true
			if value != "" {
				t.Fatalf("%s = %q, want empty", key, value)
			}
		}
	}
	for key := range wantEmpty {
		if !seen[key] {
			t.Errorf("constrained plan environment is missing %s", key)
		}
	}
}
