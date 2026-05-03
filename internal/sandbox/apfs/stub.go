//go:build !darwin

// Package apfs provides a macOS APFS sandbox implementation.
// On non-Darwin platforms this package compiles to a no-op stub so that
// binaries (e.g. agm) can blank-import the package unconditionally for
// init-time provider registration without breaking cross-platform builds.
// The provider is intentionally not registered on non-Darwin platforms;
// requesting "apfs" there falls through to the registry's
// "provider not available" error, which is the correct behavior.
package apfs
