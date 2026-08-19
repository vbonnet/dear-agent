package githooks_test

import "testing"

// requireCoherentAGMDeployments verifies that every lock-held AGM activation
// first derives the revision-matched helper, then stages the CLI and reaper
// from one source revision. Keeping this shape explicit prevents a count-only
// assertion from accepting interleaved concurrent pair deployments.
func requireCoherentAGMDeployments(t *testing.T, records []installRecord, wantRevisions ...string) {
	t.Helper()
	const buildsPerDeployment = 3
	if len(records) != len(wantRevisions)*buildsPerDeployment {
		t.Fatalf("AGM deployments staged %d builds, want %d coherent helper+pair deployments: %+v", len(records), len(wantRevisions), records)
	}
	wantPackages := []string{"./cmd/spec-contract-hook", "./agm/cmd/agm", "./agm/cmd/agm-reaper"}
	for deployment, wantRevision := range wantRevisions {
		for offset, wantPackage := range wantPackages {
			record := records[deployment*buildsPerDeployment+offset]
			if record.pkg != wantPackage {
				t.Fatalf("AGM deployment %d build %d = %s, want %s; records: %+v", deployment, offset, record.pkg, wantPackage, records)
			}
			if record.commit != wantRevision {
				t.Fatalf("AGM deployment %d %s used revision %s, want %s; records: %+v", deployment, record.pkg, record.commit, wantRevision, records)
			}
		}
		cli := records[deployment*buildsPerDeployment+1]
		reaper := records[deployment*buildsPerDeployment+2]
		if cli.ldflags == "" || cli.ldflags != reaper.ldflags {
			t.Fatalf("AGM deployment %d pair linker profiles are not one coherent profile: %+v", deployment, records[deployment*buildsPerDeployment:deployment*buildsPerDeployment+buildsPerDeployment])
		}
	}
}

// The governed helper digest is compiled into AGM. A helper-only source merge
// must therefore rebuild the helper-derived provenance and both installed AGM
// binaries from the same immutable revision.
func TestRebuild_SpecContractHookOnlyRebuildsCoherentAGMPair(t *testing.T) {
	repo := newRebuildRepo(t)
	mergeBranchChanging(t, repo, map[string]string{"cmd/spec-contract-hook/main.go": "package main // v2\n"})
	wantRevision := revParse(t, repo, "HEAD")

	records := installRecords(t, runRebuildRecord(t, repo))
	requireCoherentAGMDeployments(t, records, wantRevision)
}
