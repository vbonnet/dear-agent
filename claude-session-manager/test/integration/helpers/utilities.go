//go:build integration

package helpers

import (
	"crypto/rand"
	"encoding/hex"
	"os/exec"
)

// RandomString generates a random string of specified length
func RandomString(length int) string {
	bytes := make([]byte, length/2+1)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)[:length]
}

// IsTmuxAvailable checks if tmux is installed and available
func IsTmuxAvailable() bool {
	cmd := exec.Command("tmux", "-V")
	return cmd.Run() == nil
}
