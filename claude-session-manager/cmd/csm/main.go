package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func main() {
	// Print deprecation notice to stderr
	fmt.Fprintln(os.Stderr, "⚠️  csm has been renamed to agm, use that instead")
	fmt.Fprintln(os.Stderr, "")

	// Find agm binary in PATH
	agmPath, err := exec.LookPath("agm")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: agm binary not found in PATH\n")
		fmt.Fprintf(os.Stderr, "Please install agm and ensure it's in your PATH\n")
		os.Exit(1)
	}

	// Forward all arguments to agm using syscall.Exec
	// This replaces the current process with agm, preserving exit codes
	args := append([]string{"agm"}, os.Args[1:]...)
	env := os.Environ()

	err = syscall.Exec(agmPath, args, env)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error executing agm: %v\n", err)
		os.Exit(1)
	}
}
