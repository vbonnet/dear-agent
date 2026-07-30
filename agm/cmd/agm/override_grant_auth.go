package main

import (
	"errors"
	"fmt"
)

const unixFreshAuthenticationCommand = "/usr/bin/true"

func freshUnixAuthenticationArgs(nonInteractive bool) []string {
	args := make([]string, 0, 2)
	if nonInteractive {
		args = append(args, "-n")
	}
	return append(args, unixFreshAuthenticationCommand)
}

func requireFreshAuthentication(
	invalidate func() error,
	passwordless func() (bool, error),
	prompt func() error,
) error {
	if err := invalidate(); err != nil {
		return err
	}
	available, err := passwordless()
	if err != nil {
		return err
	}
	if available {
		return errors.Join(
			errors.New("fresh operator authentication is unavailable: passwordless sudo cannot approve a dangerous override"),
			invalidate(),
		)
	}
	if err := prompt(); err != nil {
		return errors.Join(
			fmt.Errorf("fresh operator authentication failed: %w", err),
			invalidate(),
		)
	}
	return nil
}
