package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"
	"time"
)

func fixedTime() time.Time {
	return time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)
}

func captureStdout(fn func()) string {
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()

	outC := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		outC <- buf.String()
	}()

	defer func() {
		if r := recover(); r != nil {
			_ = w.Close()
			panic(r)
		}
	}()

	fn()
	_ = w.Close()
	return <-outC
}

type mockDirInfo struct{}

func (mockDirInfo) Name() string       { return ".beads" }
func (mockDirInfo) Size() int64        { return 0 }
func (mockDirInfo) Mode() os.FileMode  { return os.ModeDir }
func (mockDirInfo) ModTime() time.Time { return fixedTime() }
func (mockDirInfo) IsDir() bool        { return true }
func (mockDirInfo) Sys() any           { return nil }

func mockStatDir(string) (os.FileInfo, error) {
	return mockDirInfo{}, nil
}

func testDeps(query func(ctx context.Context, db string) ([]BeadRecord, error)) deps {
	return deps{
		now:               fixedTime,
		statDB:            mockStatDir,
		queryLatestClosed: query,
	}
}

func TestRun_HealthyInsideLookback(t *testing.T) {
	now := fixedTime()
	d := testDeps(func(ctx context.Context, db string) ([]BeadRecord, error) {
		return []BeadRecord{
			{
				ID:       "ce-test1",
				Title:    "Test bead closure",
				Status:   "closed",
				ClosedAt: now.Add(-2 * time.Hour).Format(time.RFC3339),
			},
		}, nil
	})

	out := captureStdout(func() {
		code := run([]string{"--lookback", "24h"}, d)
		if code != 0 {
			t.Fatalf("run() = %d, want 0", code)
		}
	})

	if !bytes.Contains([]byte(out), []byte("HEALTHY")) {
		t.Errorf("stdout = %q, want HEALTHY", out)
	}
}

func TestRun_DegradedOlderThanLookback(t *testing.T) {
	now := fixedTime()
	d := testDeps(func(ctx context.Context, db string) ([]BeadRecord, error) {
		return []BeadRecord{
			{
				ID:       "ce-test2",
				Title:    "Old bead closure",
				Status:   "closed",
				ClosedAt: now.Add(-50 * time.Hour).Format(time.RFC3339),
			},
		}, nil
	})

	out := captureStdout(func() {
		code := run([]string{"--lookback", "48h"}, d)
		if code != 1 {
			t.Fatalf("run() = %d, want 1", code)
		}
	})

	if !bytes.Contains([]byte(out), []byte("DEGRADED")) {
		t.Errorf("stdout = %q, want DEGRADED", out)
	}
}

func TestRun_DegradedZeroClosedBeads(t *testing.T) {
	d := testDeps(func(ctx context.Context, db string) ([]BeadRecord, error) {
		return []BeadRecord{}, nil
	})

	out := captureStdout(func() {
		code := run([]string{"--lookback", "24h"}, d)
		if code != 1 {
			t.Fatalf("run() = %d, want 1", code)
		}
	})

	if !bytes.Contains([]byte(out), []byte("DEGRADED: no closed beads found")) {
		t.Errorf("stdout = %q, want no closed beads found", out)
	}
}

func TestRun_DegradedEmptyClosedAt(t *testing.T) {
	d := testDeps(func(ctx context.Context, db string) ([]BeadRecord, error) {
		return []BeadRecord{
			{
				ID:       "ce-empty-ts",
				Title:    "Missing closed_at",
				Status:   "closed",
				ClosedAt: "",
			},
		}, nil
	})

	out := captureStdout(func() {
		code := run([]string{"--lookback", "24h"}, d)
		if code != 1 {
			t.Fatalf("run() = %d, want 1", code)
		}
	})

	if !bytes.Contains([]byte(out), []byte("DEGRADED")) {
		t.Errorf("stdout = %q, want DEGRADED", out)
	}
}

func TestRun_DownQueryError(t *testing.T) {
	d := testDeps(func(ctx context.Context, db string) ([]BeadRecord, error) {
		return nil, errors.New("dolt database locked")
	})

	out := captureStdout(func() {
		code := run([]string{"--lookback", "24h"}, d)
		if code != 2 {
			t.Fatalf("run() = %d, want 2", code)
		}
	})

	if !bytes.Contains([]byte(out), []byte("DOWN: cannot query beads")) {
		t.Errorf("stdout = %q, want DOWN: cannot query beads", out)
	}
}

func TestRun_DownInvalidTimestampFormat(t *testing.T) {
	d := testDeps(func(ctx context.Context, db string) ([]BeadRecord, error) {
		return []BeadRecord{
			{
				ID:       "ce-bad-ts",
				Title:    "Corrupt timestamp",
				Status:   "closed",
				ClosedAt: "not-a-valid-timestamp",
			},
		}, nil
	})

	out := captureStdout(func() {
		code := run([]string{"--lookback", "24h"}, d)
		if code != 2 {
			t.Fatalf("run() = %d, want 2", code)
		}
	})

	if !bytes.Contains([]byte(out), []byte("DOWN: cannot parse closed_at")) {
		t.Errorf("stdout = %q, want DOWN: cannot parse closed_at", out)
	}
}

func TestRun_DownFutureClosureClockSkew(t *testing.T) {
	now := fixedTime()
	d := testDeps(func(ctx context.Context, db string) ([]BeadRecord, error) {
		return []BeadRecord{
			{
				ID:       "ce-future",
				Title:    "Future timestamp",
				Status:   "closed",
				ClosedAt: now.Add(10 * time.Minute).Format(time.RFC3339),
			},
		}, nil
	})

	out := captureStdout(func() {
		code := run([]string{"--lookback", "24h"}, d)
		if code != 2 {
			t.Fatalf("run() = %d, want 2", code)
		}
	})

	if !bytes.Contains([]byte(out), []byte("DOWN: latest bead ce-future closure time")) {
		t.Errorf("stdout = %q, want DOWN future message", out)
	}
}

func TestRun_UsageBadLookback(t *testing.T) {
	d := deps{
		now: fixedTime,
		queryLatestClosed: func(ctx context.Context, db string) ([]BeadRecord, error) {
			return nil, nil
		},
	}

	code := run([]string{"--lookback", "invalid"}, d)
	if code != 3 {
		t.Fatalf("run() = %d, want 3 for invalid lookback", code)
	}

	code = run([]string{"--lookback", "-5m"}, d)
	if code != 3 {
		t.Fatalf("run() = %d, want 3 for negative lookback", code)
	}
}

func TestRun_JSONReport(t *testing.T) {
	now := fixedTime()
	d := testDeps(func(ctx context.Context, db string) ([]BeadRecord, error) {
		return []BeadRecord{
			{
				ID:       "ce-json",
				Title:    "JSON report check",
				Status:   "closed",
				ClosedAt: now.Add(-30 * time.Minute).Format(time.RFC3339),
			},
		}, nil
	})

	var r Report
	out := captureStdout(func() {
		code := run([]string{"--lookback", "1h", "--json"}, d)
		if code != 0 {
			t.Fatalf("run() = %d, want 0", code)
		}
	})

	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("unmarshal json report: %v, raw output: %q", err, out)
	}

	if r.Status != "healthy" {
		t.Errorf("r.Status = %q, want healthy", r.Status)
	}
	if r.LatestID != "ce-json" {
		t.Errorf("r.LatestID = %q, want ce-json", r.LatestID)
	}
	if r.LatestTitle != "JSON report check" {
		t.Errorf("r.LatestTitle = %q, want JSON report check", r.LatestTitle)
	}
	if r.LatestClosedAge != "30m0s" {
		t.Errorf("r.LatestClosedAge = %q, want 30m0s", r.LatestClosedAge)
	}
	if r.Lookback != "1h" {
		t.Errorf("r.Lookback = %q, want 1h", r.Lookback)
	}
}

func TestRun_DownDBStatError(t *testing.T) {
	d := deps{
		now: fixedTime,
		statDB: func(path string) (os.FileInfo, error) {
			return nil, os.ErrNotExist
		},
	}

	out := captureStdout(func() {
		code := run([]string{"--db", "/path/not/found"}, d)
		if code != 2 {
			t.Fatalf("run() = %d, want 2", code)
		}
	})

	if !bytes.Contains([]byte(out), []byte("DOWN: cannot access beads database")) {
		t.Errorf("stdout = %q, want DOWN: cannot access beads database", out)
	}
}

func TestRun_HomeDirErrorOnTildePath(t *testing.T) {
	d := deps{
		now: fixedTime,
		userHomeDir: func() (string, error) {
			return "", errors.New("cannot determine home directory")
		},
	}

	code := run([]string{"--db", "~/test"}, d)
	if code != 3 {
		t.Fatalf("run() = %d, want 3", code)
	}
}

type mockFileInfo struct{}

func (mockFileInfo) Name() string       { return "file" }
func (mockFileInfo) Size() int64        { return 10 }
func (mockFileInfo) Mode() os.FileMode  { return 0644 }
func (mockFileInfo) ModTime() time.Time { return fixedTime() }
func (mockFileInfo) IsDir() bool        { return false }
func (mockFileInfo) Sys() any           { return nil }

func TestRun_DownDBNotADir(t *testing.T) {
	d := deps{
		now: fixedTime,
		statDB: func(path string) (os.FileInfo, error) {
			return mockFileInfo{}, nil
		},
	}

	out := captureStdout(func() {
		code := run([]string{"--db", "/path/to/file"}, d)
		if code != 2 {
			t.Fatalf("run() = %d, want 2", code)
		}
	})

	if !bytes.Contains([]byte(out), []byte("is not a directory")) {
		t.Errorf("stdout = %q, want is not a directory", out)
	}
}
