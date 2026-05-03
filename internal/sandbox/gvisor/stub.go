//go:build !linux

// Package gvisor provides a Linux-only gVisor (runsc) sandbox implementation.
// gVisor intercepts application syscalls in userspace via ptrace or KVM, both
// of which require Linux. On non-Linux platforms this package compiles to an
// empty stub so binaries (e.g. agm) can blank-import the package
// unconditionally for init-time provider registration without breaking
// cross-platform builds. The provider is intentionally not registered on
// non-Linux platforms; requesting "gvisor" there falls through to the
// registry's "provider not available" error, which is the correct behavior.
package gvisor
