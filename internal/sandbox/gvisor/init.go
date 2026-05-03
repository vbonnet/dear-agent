//go:build linux

// Package gvisor provides gVisor (runsc) sandbox functionality.
package gvisor

import "github.com/vbonnet/dear-agent/internal/sandbox"

func init() {
	sandbox.RegisterProvider("gvisor", func() sandbox.Provider {
		return NewProvider()
	})
}
