//go:build darwin

package override

// macOS exposes /etc as a symlink to the canonical, root-owned /private/etc.
// Use the canonical path so the grant validator can continue rejecting
// attacker-controlled symlinks without rejecting the operating-system layout.
const operatorGrantDir = "/private/etc"
