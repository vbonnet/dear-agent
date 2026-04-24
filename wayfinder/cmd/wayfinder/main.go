package main

import (
	"fmt"
	"os"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
