//go:build !darwin && !linux

package sandboxonboarding

import "fmt"

func requireSupportedPlatform() error {
	return fmt.Errorf("sandbox onboarding installation is unsupported on this platform")
}

func installOutput(string, string, []byte, installHooks) error {
	return fmt.Errorf("sandbox onboarding installation is unsupported on this platform")
}
