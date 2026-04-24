// Package bubblewrap provides bubblewrap functionality.
package bubblewrap

import "github.com/vbonnet/dear-agent/internal/sandbox"

func init() {
	// Register bubblewrap provider in the global registry
	sandbox.RegisterProvider("bubblewrap", func() sandbox.Provider {
		return NewProvider()
	})
}
