// Command override-audit-launchdaemon-installer performs the privileged half
// of the macOS override-audit installation transaction.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"

	"github.com/vbonnet/dear-agent/agm/internal/launchdaudit"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) != 5 {
		fmt.Fprintln(os.Stderr, "override-audit LaunchDaemon installer requires 5 arguments")
		return 2
	}
	rootGID, err := strconv.Atoi(args[0])
	if err != nil || rootGID < 0 {
		fmt.Fprintln(os.Stderr, "override-audit LaunchDaemon installer received an invalid root group ID")
		return 2
	}
	config, err := launchdaudit.NewConfig(
		rootGID,
		args[1], args[2],
		args[3], args[4],
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "override-audit LaunchDaemon installer: %v\n", err)
		return 2
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGHUP, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	var signalCode atomic.Int32
	go func() {
		select {
		case received := <-signals:
			signalCode.Store(exitCodeForSignal(received))
			cancel(fmt.Errorf("received %s", received))
		case <-ctx.Done():
		}
	}()

	if err := launchdaudit.Install(ctx, config); err != nil {
		fmt.Fprintf(os.Stderr, "override-audit LaunchDaemon installer: %v\n", err)
		switch signalCode.Load() {
		case 129:
			return 129
		case 130:
			return 130
		case 143:
			return 143
		}
		return 1
	}
	return 0
}

func exitCodeForSignal(received os.Signal) int32 {
	switch received {
	case syscall.SIGHUP:
		return 129
	case os.Interrupt:
		return 130
	case syscall.SIGTERM:
		return 143
	default:
		return 1
	}
}
