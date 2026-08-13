package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

const (
	processFixtureSetupTimeout = 30 * time.Second
	processFixtureStopTimeout  = 5 * time.Second
)

type processPIDs struct {
	leader int
	child  int
}

func TestParseProcessPIDs(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		want      processPIDs
		wantReady bool
		wantErr   bool
	}{
		{name: "empty"},
		{name: "one complete PID", raw: "123\n"},
		{name: "incomplete descendant", raw: "123\n456"},
		{name: "malformed leader", raw: "bad\n456\n", wantErr: true},
		{name: "non-positive descendant", raw: "123\n0\n", wantErr: true},
		{name: "extra field", raw: "123\n456\n789\n", wantErr: true},
		{
			name:      "complete leader and descendant",
			raw:       "123\n456\n",
			want:      processPIDs{leader: 123, child: 456},
			wantReady: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ready, err := parseProcessPIDs([]byte(tc.raw))
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseProcessPIDs(%q) error = %v, wantErr %v", tc.raw, err, tc.wantErr)
			}
			if ready != tc.wantReady {
				t.Fatalf("parseProcessPIDs(%q) ready = %v, want %v", tc.raw, ready, tc.wantReady)
			}
			if got != tc.want {
				t.Fatalf("parseProcessPIDs(%q) = %+v, want %+v", tc.raw, got, tc.want)
			}
		})
	}
}

func TestAwaitProcessFixtureWaitsForCompletePIDRecord(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "go-pids")
	if err := os.WriteFile(pidFile, []byte("123\n456"), 0o600); err != nil {
		t.Fatal(err)
	}

	type fixtureOutcome struct {
		pids processPIDs
		err  error
	}
	result := make(chan struct{})
	outcome := make(chan fixtureOutcome, 1)
	go func() {
		pids, err := awaitProcessFixture(
			pidFile,
			result,
			func() {},
			func(struct{}) string { return "unexpected completion" },
			5*time.Second,
		)
		outcome <- fixtureOutcome{pids: pids, err: err}
	}()

	select {
	case got := <-outcome:
		t.Fatalf("partial PID record established readiness: %+v", got)
	case <-time.After(50 * time.Millisecond):
	}
	if err := os.WriteFile(pidFile, []byte("123\n456\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-outcome:
		if got.err != nil {
			t.Fatal(got.err)
		}
		want := processPIDs{leader: 123, child: 456}
		if got.pids != want {
			t.Fatalf("process PIDs = %+v, want %+v", got.pids, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("complete PID record did not establish readiness")
	}
}

func TestAwaitProcessFixtureReportsEarlyCommandExit(t *testing.T) {
	binDir := t.TempDir()
	stub := "#!/bin/sh\necho early-exit-marker >&2\nexit 23\n"
	if err := os.WriteFile(filepath.Join(binDir, "go"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := t.TempDir()
	var stderr bytes.Buffer
	result := make(chan int, 1)
	result <- runGoTestCommand(
		context.Background(),
		time.Second,
		options{root: root},
		[]string{"example.com/m/a"},
		&bytes.Buffer{},
		&stderr,
	)

	_, err := awaitProcessFixture(
		filepath.Join(t.TempDir(), "never-created"),
		result,
		func() {},
		func(code int) string {
			return fmt.Sprintf("exit code %d: %s", code, stderr.String())
		},
		5*time.Second,
	)
	if err == nil {
		t.Fatal("early command exit established process readiness")
	}
	if got := err.Error(); !strings.Contains(got, "exit code 23") || !strings.Contains(got, "early-exit-marker") {
		t.Fatalf("early command diagnostic = %q, want exit code and stderr marker", got)
	}
	if strings.Contains(err.Error(), "timed out") {
		t.Fatalf("early command exit was misreported as readiness timeout: %v", err)
	}
}

func TestAwaitProcessFixtureTimeoutCancelsAndJoinsProcessGroup(t *testing.T) {
	binDir := t.TempDir()
	observedPIDFile := filepath.Join(t.TempDir(), "observed-go-pids")
	stub := "#!/bin/sh\nsleep 120 &\nchild=$!\nprintf '%s\\n%s\\n' \"$$\" \"$child\" > \"$TEST_AFFECTED_OBSERVED_PID_FILE\"\nwait \"$child\"\n"
	if err := os.WriteFile(filepath.Join(binDir, "go"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TEST_AFFECTED_OBSERVED_PID_FILE", observedPIDFile)

	ctx, cancel := context.WithCancelCause(context.Background())
	t.Cleanup(func() { cancel(context.Canceled) })
	root := t.TempDir()
	var stderr bytes.Buffer
	result := make(chan int, 1)
	go func() {
		result <- runGoTestCommand(
			ctx,
			time.Second,
			options{root: root},
			[]string{"example.com/m/a"},
			&bytes.Buffer{},
			&stderr,
		)
	}()
	describeResult := func(code int) string {
		return fmt.Sprintf("exit code %d: %s", code, stderr.String())
	}
	pids, err := awaitProcessFixture(
		observedPIDFile,
		result,
		func() { cancel(context.Canceled) },
		describeResult,
		processFixtureSetupTimeout,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-pids.leader, syscall.SIGKILL)
	})

	_, err = awaitProcessFixture(
		filepath.Join(t.TempDir(), "never-ready"),
		result,
		func() { cancel(context.Canceled) },
		describeResult,
		25*time.Millisecond,
	)
	if err == nil {
		t.Fatal("missing readiness file did not time out")
	}
	if got := err.Error(); !strings.Contains(got, "timed out after 25ms") || !strings.Contains(got, "command stopped with exit code") {
		t.Fatalf("setup timeout diagnostic = %q, want timeout and joined command result", got)
	}
	waitForProcessGone(t, pids.leader)
	waitForProcessGone(t, pids.child)
}

func awaitProcessFixture[T any](
	pidFile string,
	result <-chan T,
	cancel func(),
	describeResult func(T) string,
	setupTimeout time.Duration,
) (processPIDs, error) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(setupTimeout)
	defer timer.Stop()

	lastContents := "missing"
	for {
		select {
		case completed := <-result:
			return processPIDs{}, fmt.Errorf(
				"command exited before process fixture readiness: %s",
				describeResult(completed),
			)
		default:
		}

		raw, err := os.ReadFile(pidFile)
		if err == nil {
			lastContents = strconv.Quote(string(raw))
			pids, ready, parseErr := parseProcessPIDs(raw)
			if parseErr != nil {
				return processPIDs{}, stopProcessFixture(
					result,
					cancel,
					describeResult,
					fmt.Errorf("invalid process fixture readiness file %s: %w", pidFile, parseErr),
				)
			}
			if ready {
				select {
				case completed := <-result:
					return processPIDs{}, fmt.Errorf(
						"command exited before process fixture readiness: %s",
						describeResult(completed),
					)
				default:
					return pids, nil
				}
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return processPIDs{}, stopProcessFixture(
				result,
				cancel,
				describeResult,
				fmt.Errorf("read process fixture readiness file %s: %w", pidFile, err),
			)
		}

		select {
		case completed := <-result:
			return processPIDs{}, fmt.Errorf(
				"command exited before process fixture readiness: %s",
				describeResult(completed),
			)
		case <-ticker.C:
		case <-timer.C:
			return processPIDs{}, stopProcessFixture(
				result,
				cancel,
				describeResult,
				fmt.Errorf(
					"timed out after %s waiting for complete process fixture readiness file %s (last contents: %s)",
					setupTimeout,
					pidFile,
					lastContents,
				),
			)
		}
	}
}

func parseProcessPIDs(raw []byte) (processPIDs, bool, error) {
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		return processPIDs{}, false, nil
	}
	fields := strings.Fields(string(raw))
	if len(fields) < 2 {
		return processPIDs{}, false, nil
	}
	if len(fields) > 2 {
		return processPIDs{}, false, fmt.Errorf("got %d fields, want leader and descendant", len(fields))
	}
	leader, err := strconv.Atoi(fields[0])
	if err != nil || leader <= 0 {
		return processPIDs{}, false, fmt.Errorf("invalid leader process ID %q", fields[0])
	}
	child, err := strconv.Atoi(fields[1])
	if err != nil || child <= 0 {
		return processPIDs{}, false, fmt.Errorf("invalid descendant process ID %q", fields[1])
	}
	return processPIDs{leader: leader, child: child}, true, nil
}

func stopProcessFixture[T any](
	result <-chan T,
	cancel func(),
	describeResult func(T) string,
	reason error,
) error {
	cancel()
	timer := time.NewTimer(processFixtureStopTimeout)
	defer timer.Stop()
	select {
	case completed := <-result:
		return fmt.Errorf("%w; command stopped with %s", reason, describeResult(completed))
	case <-timer.C:
		return fmt.Errorf(
			"%w; command did not stop within %s after cancellation",
			reason,
			processFixtureStopTimeout,
		)
	}
}
