// Package common provides common functionality.
package common

import "errors"

var (
	// ErrBaselineNotFound indicates the baseline file does not exist
	ErrBaselineNotFound = errors.New("baseline file not found")

	// ErrInvalidSchema indicates the baseline schema version is not supported
	ErrInvalidSchema = errors.New("unsupported baseline schema version")

	// ErrHighVariance indicates benchmark results have high coefficient of variation
	ErrHighVariance = errors.New("high variance detected (CV% > 20%)")

	// ErrTimeout indicates command execution exceeded timeout
	ErrTimeout = errors.New("command execution timeout")
)
