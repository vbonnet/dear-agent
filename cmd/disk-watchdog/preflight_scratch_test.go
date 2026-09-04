package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFindPreflightScratchDirs_Locations(t *testing.T) {
	temp := t.TempDir()
	realTemp, err := filepath.EvalSymlinks(temp)
	if err != nil {
		realTemp = temp
	}
	euid := int64(os.Geteuid())

	fakeHome := filepath.Join(realTemp, "home")
	fakeCache := filepath.Join(realTemp, "cache")
	preflightTmp := filepath.Join(fakeCache, "dear-agent", "preflight-tmp")
	preflightRuns := filepath.Join(fakeCache, "dear-agent", "preflight-runs")

	mustMkdir := func(p string) {
		t.Helper()
		if err := os.MkdirAll(p, 0o700); err != nil {
			t.Fatalf("MkdirAll(%q): %v", p, err)
		}
	}

	// 1. Preflight tmp root
	runDir1 := filepath.Join(preflightTmp, "run.123456")
	mustMkdir(runDir1)

	// 2. Preflight runs root
	runDir2 := filepath.Join(preflightRuns, "ce-1hu9-66-current")
	mustMkdir(runDir2)

	// 3. Fake home: legacy scratch and real directories
	homePreflightHome := filepath.Join(fakeHome, ".preflight-home-1133.FPJk1d")
	homePreflight := filepath.Join(fakeHome, ".preflight-1133.64fu8h")
	homeCePreflight := filepath.Join(fakeHome, "ce1209-preflight.XdLYt8")
	homeCe68 := filepath.Join(fakeHome, ".ce68")
	homeTmp := filepath.Join(fakeHome, ".tmp")
	homeTmpChild := filepath.Join(homeTmp, "ce-1205-preflight-diagnostic")
	homeNormalDir := filepath.Join(fakeHome, "Documents")
	homeDotGit := filepath.Join(fakeHome, ".git")

	mustMkdir(homePreflightHome)
	mustMkdir(homePreflight)
	mustMkdir(homeCePreflight)
	mustMkdir(homeCe68)
	mustMkdir(homeTmpChild)
	mustMkdir(homeNormalDir)
	mustMkdir(homeDotGit)

	// 4. Fake cache: legacy scratch and real directories
	cacheCePreflight := filepath.Join(fakeCache, "ce1133-preflight.CT8Vmy")
	cacheDearPreflight := filepath.Join(fakeCache, "dear-agent-preflight-ce-3knl")
	cacheHostTmp := filepath.Join(fakeCache, "ce-emxwa-host-tmp")
	cacheNormalDir := filepath.Join(fakeCache, "unrelated-cache")

	mustMkdir(cacheCePreflight)
	mustMkdir(cacheDearPreflight)
	mustMkdir(cacheHostTmp)
	mustMkdir(cacheNormalDir)

	// Test preflight-tmp scanning
	tmpCandidates, err := findPreflightScratchDirs(preflightTmp, fakeHome, fakeCache, euid)
	if err != nil {
		t.Fatalf("findPreflightScratchDirs(preflightTmp): %v", err)
	}
	if len(tmpCandidates) != 1 || tmpCandidates[0] != runDir1 {
		t.Errorf("preflightTmp candidates = %v, want [%q]", tmpCandidates, runDir1)
	}

	// Test preflight-runs scanning
	runsCandidates, err := findPreflightScratchDirs(preflightRuns, fakeHome, fakeCache, euid)
	if err != nil {
		t.Fatalf("findPreflightScratchDirs(preflightRuns): %v", err)
	}
	if len(runsCandidates) != 1 || runsCandidates[0] != runDir2 {
		t.Errorf("preflightRuns candidates = %v, want [%q]", runsCandidates, runDir2)
	}

	// Test home scanning
	homeCandidates, err := findPreflightScratchDirs(fakeHome, fakeHome, fakeCache, euid)
	if err != nil {
		t.Fatalf("findPreflightScratchDirs(fakeHome): %v", err)
	}
	wantHome := map[string]bool{
		homePreflightHome: true,
		homePreflight:     true,
		homeCePreflight:   true,
		homeCe68:          true,
		homeTmp:           true,
		homeTmpChild:      true,
	}
	for _, c := range homeCandidates {
		if !wantHome[c] {
			t.Errorf("unexpected home candidate: %q", c)
		}
		delete(wantHome, c)
	}
	if len(wantHome) > 0 {
		t.Errorf("missing expected home candidates: %v", wantHome)
	}

	// Test cache scanning
	cacheCandidates, err := findPreflightScratchDirs(fakeCache, fakeHome, fakeCache, euid)
	if err != nil {
		t.Fatalf("findPreflightScratchDirs(fakeCache): %v", err)
	}
	wantCache := map[string]bool{
		cacheCePreflight:   true,
		cacheDearPreflight: true,
		cacheHostTmp:       true,
	}
	for _, c := range cacheCandidates {
		if !wantCache[c] {
			t.Errorf("unexpected cache candidate: %q", c)
		}
		delete(wantCache, c)
	}
	if len(wantCache) > 0 {
		t.Errorf("missing expected cache candidates: %v", wantCache)
	}
}

func TestReapPreflightScratch_AgeAndLivenessGates(t *testing.T) {
	temp := t.TempDir()
	realTemp, err := filepath.EvalSymlinks(temp)
	if err != nil {
		realTemp = temp
	}
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	minAge := 24 * time.Hour

	oldDir := filepath.Join(realTemp, "old-scratch")
	recentDir := filepath.Join(realTemp, "recent-scratch")
	busyDir := filepath.Join(realTemp, "busy-scratch")
	errorDir := filepath.Join(realTemp, "error-scratch")

	for _, d := range []string{oldDir, recentDir, busyDir, errorDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(filepath.Join(d, "file.txt"), []byte("data"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	// Set mtime
	oldTime := now.Add(-48 * time.Hour)
	recentTime := now.Add(-1 * time.Hour)
	_ = os.Chtimes(oldDir, oldTime, oldTime)
	_ = os.Chtimes(busyDir, oldTime, oldTime)
	_ = os.Chtimes(errorDir, oldTime, oldTime)
	_ = os.Chtimes(recentDir, recentTime, recentTime)

	inUseSeam := func(path string) (bool, error) {
		switch path {
		case busyDir:
			return true, nil
		case errorDir:
			return false, errors.New("lsof failed")
		default:
			return false, nil
		}
	}

	var removedPaths []string
	removeSeam := func(path string) error {
		removedPaths = append(removedPaths, path)
		return os.RemoveAll(path)
	}

	cfg := preflightScratchConfig{
		Roots:  []string{realTemp},
		MinAge: minAge,
		Reap:   true,
		now:    func() time.Time { return now },
		inUse:  inUseSeam,
		remove: removeSeam,
		sizeOf: func(path string) int64 { return 1024 },
	}

	res := reapPreflightScratch(cfg)

	if len(res.Reaped) != 1 || res.Reaped[0] != oldDir {
		t.Errorf("Reaped = %v, want [%q]", res.Reaped, oldDir)
	}
	if len(removedPaths) != 1 || removedPaths[0] != oldDir {
		t.Errorf("removedPaths = %v, want [%q]", removedPaths, oldDir)
	}
	if res.BytesReclaimed != 1024 {
		t.Errorf("BytesReclaimed = %d, want 1024", res.BytesReclaimed)
	}

	// Verify skipped reasons
	if !strings.Contains(res.Skipped[recentDir], "too recent") {
		t.Errorf("recentDir skipped reason = %q, want 'too recent'", res.Skipped[recentDir])
	}
	if !strings.Contains(res.Skipped[busyDir], "a process holds a file open inside") {
		t.Errorf("busyDir skipped reason = %q, want 'a process holds a file open inside'", res.Skipped[busyDir])
	}
	if !strings.Contains(res.Skipped[errorDir], "liveness probe failed") {
		t.Errorf("errorDir skipped reason = %q, want 'liveness probe failed'", res.Skipped[errorDir])
	}
}

func TestReapPreflightScratch_DryRun(t *testing.T) {
	temp := t.TempDir()
	realTemp, err := filepath.EvalSymlinks(temp)
	if err != nil {
		realTemp = temp
	}
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	scratchDir := filepath.Join(realTemp, "abandoned-scratch")
	if err := os.MkdirAll(scratchDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	oldTime := now.Add(-48 * time.Hour)
	_ = os.Chtimes(scratchDir, oldTime, oldTime)

	cfg := preflightScratchConfig{
		Roots:  []string{realTemp},
		MinAge: 24 * time.Hour,
		Reap:   false,
		now:    func() time.Time { return now },
		inUse:  func(_ string) (bool, error) { return false, nil },
		sizeOf: func(_ string) int64 { return 2048 },
	}

	res := reapPreflightScratch(cfg)

	if len(res.Reaped) != 0 {
		t.Errorf("dry-run Reaped = %v, want empty", res.Reaped)
	}
	if res.BytesReclaimed != 2048 {
		t.Errorf("dry-run BytesReclaimed = %d, want 2048", res.BytesReclaimed)
	}
	if res.Skipped[scratchDir] != "scan only" {
		t.Errorf("dry-run Skipped[%q] = %q, want 'scan only'", scratchDir, res.Skipped[scratchDir])
	}
	if _, err := os.Stat(scratchDir); err != nil {
		t.Errorf("dry-run removed directory: %v", err)
	}
}

func TestReapPreflightScratch_EmptyRootsDisablesReaper(t *testing.T) {
	cfg := config{preflightScratchRoots: ""}
	res := reapAbandonedPreflightScratch(cfg)
	if res != nil {
		t.Errorf("reapAbandonedPreflightScratch with empty roots = %+v, want nil", res)
	}
}

func TestRun_InvalidPreflightScratchMinAgeExitsTwo(t *testing.T) {
	var out bytes.Buffer
	code, err := run([]string{
		"--preflight-scratch-roots", "/tmp",
		"--preflight-scratch-min-age", "0s",
	}, &out)
	if code != 2 || err == nil || !strings.Contains(err.Error(), "invalid -preflight-scratch-min-age") {
		t.Fatalf("run() code = %d, err = %v; want exit 2 with invalid -preflight-scratch-min-age error", code, err)
	}
}
