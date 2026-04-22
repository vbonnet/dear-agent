//go:build !linux

// Package overlayfs provides a native Linux OverlayFS sandbox implementation.
// On non-Linux platforms this package is a no-op stub; the provider is not
// registered because OverlayFS is Linux-specific.
package overlayfs
