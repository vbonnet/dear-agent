//go:build freebsd

package harnessexec

import "os/exec"

// resolveExecutableInEnvironment keeps the private executor compile-complete
// on FreeBSD without treating compilation as runtime support.
func resolveExecutableInEnvironment(file string, _ []string) (string, error) {
	return "", &exec.Error{Name: file, Err: errFreeBSDPrivateHarnessExecution}
}
