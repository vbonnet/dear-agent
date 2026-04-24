package main

import (
	"fmt"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	if testing.Short() {
		fmt.Println("Skipping: requires infrastructure not available in CI")
		os.Exit(0)
	}
	os.Exit(m.Run())
}
