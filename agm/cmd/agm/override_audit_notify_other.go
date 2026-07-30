//go:build !darwin

package main

import "errors"

func sendOverrideAuditNotification(string) error {
	return errors.New("override audit notifications are only implemented for the macOS launchd deployment")
}
