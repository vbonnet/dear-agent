//go:build windows

package codexhooks

import "fmt"

func validateTrustedExecutableSearchPath(string) error {
	return fmt.Errorf("attested Codex command hooks require a POSIX runtime")
}
