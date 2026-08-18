//go:build windows

package main

import "fmt"

func installOperatorGrant([]byte, string) error {
	return fmt.Errorf("operator-owned override grants are not implemented on Windows")
}

func removeOperatorGrant(string) error {
	return fmt.Errorf("operator-owned override grants are not implemented on Windows")
}
