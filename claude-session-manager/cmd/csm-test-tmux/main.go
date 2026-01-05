package main

import (
	"fmt"
	"os"
)

var Version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// TODO: Initialize Cobra root command
	fmt.Println("csm-test-tmux", Version)
	return nil
}
