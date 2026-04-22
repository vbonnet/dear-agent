//go:build linux

// Package overlayfs provides overlayfs functionality.
package overlayfs

import "github.com/vbonnet/dear-agent/internal/sandbox"

func init() {
	// Register overlayfs provider in the global registry
	sandbox.RegisterProvider("overlayfs", func() sandbox.Provider {
		return NewProvider()
	})

	// Also register as "overlayfs-native" for backward compatibility
	sandbox.RegisterProvider("overlayfs-native", func() sandbox.Provider {
		return NewProvider()
	})
}
