package harnessexec

import (
	"os"
)

// init intercepts the detached expiry helper before either an application main
// or a Go test main can run. The current executable may be AGM, a linked
// companion, or a package test binary; every caller that can prepare a handoff
// necessarily links this package, so the marker provides one consistent early
// process boundary without recursively executing package tests.
func init() {
	if os.Getenv(expiryHelperEnv) != "1" {
		return
	}
	if len(os.Args) < 2 || os.Args[1] != ExpiryProtocol {
		os.Exit(2)
	}
	if err := runExpiry(os.Args[2:]); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
