//go:build !linux

// Package overlayfs provides a no-op stub on non-Linux platforms so binaries
// can compile without the Linux-only OverlayFS provider implementation.
package overlayfs
