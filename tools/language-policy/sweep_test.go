package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGitHubAnnotationEscapesDataAndProperties(t *testing.T) {
	got := githubAnnotation("warning", "dir/a:b,c%\r\n.sh", "message: 100%\r\nnext,still-data")
	want := "::warning file=dir/a%3Ab%2Cc%25%0D%0A.sh::message: 100%25%0D%0Anext,still-data"
	if got != want {
		t.Fatalf("githubAnnotation = %q, want %q", got, want)
	}

	got = githubAnnotation("warning", "", "line one\nline two")
	want = "::warning::line one%0Aline two"
	if got != want {
		t.Fatalf("githubAnnotation without properties = %q, want %q", got, want)
	}
}

func TestFormatExpiringWarning(t *testing.T) {
	sunset := "2026-09-14"
	e := Exception{
		Rule:   "bash-20-line-limit",
		Path:   "scripts/deepsec-incremental.sh",
		Sunset: &sunset,
	}

	plain := formatExpiringWarning(e, 30, false)
	wantPlain := "expiring waiver: scripts/deepsec-incremental.sh (rule bash-20-line-limit, sunset 2026-09-14, within 30 days); remove it if no longer needed, otherwise shorten the script, add a test under tests/bats/, or renew it with explicit owner approval before expiry"
	if plain != wantPlain {
		t.Errorf("plain warning = %q, want %q", plain, wantPlain)
	}

	github := formatExpiringWarning(e, 30, true)
	wantGitHub := "::warning file=scripts/deepsec-incremental.sh::bash-20-line-limit waiver expires on 2026-09-14 (within 30 days); remove it if no longer needed, otherwise shorten the script, add a test under tests/bats/, or renew it with explicit owner approval before expiry"
	if github != wantGitHub {
		t.Errorf("GitHub warning = %q, want %q", github, wantGitHub)
	}
}

func TestRunSweepAtReportsUpcomingWithoutFailing(t *testing.T) {
	store := strings.Join([]string{
		`{"rule":"bash-20-line-limit","path":"expired.sh","status":"active","reason":"r","approver":"v","sunset":"2026-08-18","added":"2026-08-01"}`,
		`{"rule":"bash-20-line-limit","path":"waived.sh","status":"active","reason":"r","approver":"v","sunset":"2026-09-14","added":"2026-08-01"}`,
	}, "\n") + "\n"
	root := newFixtureRepo(t, store, nil)
	for _, path := range []string{"expired.sh", "waived.sh"} {
		if err := os.WriteFile(filepath.Join(root, path), []byte("#!/bin/bash\necho present\n"), 0o755); err != nil {
			t.Fatalf("WriteFile %s: %v", path, err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "short.sh"), []byte("#!/bin/bash\necho short\n"), 0o755); err != nil {
		t.Fatalf("WriteFile short.sh: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "directory.sh"), 0o755); err != nil {
		t.Fatalf("MkdirAll directory.sh: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "vendor"), 0o755); err != nil {
		t.Fatalf("MkdirAll vendor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "vendor", "ignored.sh"), []byte("#!/bin/bash\necho ignored\n"), 0o755); err != nil {
		t.Fatalf("WriteFile vendor/ignored.sh: %v", err)
	}

	var runErr error
	out := captureStdout(t, func() {
		runErr = runSweepAt([]string{
			"-repo", root, "--github",
			"short.sh", "missing.sh", "directory.sh", "vendor/ignored.sh",
		}, time.Date(2026, 8, 19, 23, 59, 0, 0, time.UTC))
	})
	if runErr != nil {
		t.Fatalf("runSweepAt returned %v for warning-only state", runErr)
	}

	expired := "::warning file=expired.sh::waiver for expired.sh expired on 2026-08-18"
	upcoming := "::warning file=waived.sh::bash-20-line-limit waiver expires on 2026-09-14 (within 30 days)"
	if !strings.Contains(out, expired) {
		t.Errorf("sweep output missing expired warning %q:\n%s", expired, out)
	}
	if !strings.Contains(out, upcoming) {
		t.Errorf("sweep output missing upcoming warning prefix %q:\n%s", upcoming, out)
	}
	if strings.Index(out, expired) >= strings.Index(out, upcoming) {
		t.Errorf("expired warning must precede upcoming warning:\n%s", out)
	}
	wantCensus := "sweep: 2 waiver(s) total, 1 expired, 1 expiring within 30 days, 1 script(s) scanned"
	if !strings.Contains(out, wantCensus) {
		t.Errorf("sweep output missing census %q:\n%s", wantCensus, out)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = old })

	fn()
	if err := w.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	os.Stdout = old
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read captured stdout: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close stdout reader: %v", err)
	}
	return string(b)
}

func TestRunSweepAtReportsCensusWhenScanFails(t *testing.T) {
	store := strings.Join([]string{
		`{"rule":"bash-20-line-limit","path":"expired.sh","status":"active","reason":"r","approver":"v","sunset":"2026-08-18","added":"2026-08-01"}`,
		`{"rule":"bash-20-line-limit","path":"waived.sh","status":"active","reason":"r","approver":"v","sunset":"2026-09-14","added":"2026-08-01"}`,
	}, "\n") + "\n"
	root := newFixtureRepo(t, store, nil)

	// A long, untested, unwaived script makes report fail the sweep. The
	// census must survive that error return: LANGPOLICY-CMD-29 keeps the
	// backlog visible precisely when the verdict is bad.
	long := "#!/bin/bash\n"
	for i := 0; i < lineLimit+10; i++ {
		long += fmt.Sprintf("echo line %d\n", i)
	}
	if err := os.WriteFile(filepath.Join(root, "long.sh"), []byte(long), 0o755); err != nil {
		t.Fatalf("WriteFile long.sh: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "short.sh"), []byte("#!/bin/bash\necho short\n"), 0o755); err != nil {
		t.Fatalf("WriteFile short.sh: %v", err)
	}

	var runErr error
	out := captureStdout(t, func() {
		runErr = runSweepAt([]string{
			"-repo", root, "--github", "long.sh", "short.sh",
		}, time.Date(2026, 8, 19, 23, 59, 0, 0, time.UTC))
	})
	if runErr == nil {
		t.Fatalf("runSweepAt returned nil for a violating script; output:\n%s", out)
	}
	if !strings.Contains(runErr.Error(), "exceed the 20-line limit") {
		t.Errorf("runSweepAt error = %v, want the line-limit verdict", runErr)
	}

	wantCensus := "sweep: 2 waiver(s) total, 1 expired, 1 expiring within 30 days, 2 script(s) scanned"
	if !strings.Contains(out, wantCensus) {
		t.Errorf("failing sweep output missing census %q:\n%s", wantCensus, out)
	}
	if !strings.Contains(out, "::error file=long.sh::") {
		t.Errorf("failing sweep output missing the violation annotation:\n%s", out)
	}
}
