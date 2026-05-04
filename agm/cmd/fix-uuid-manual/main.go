package main

import (
	"fmt"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"os"
)

func main() {
	// Configure for oss workspace
	config := &dolt.Config{
		Workspace: "oss",
		Port:      "3307",
		Host:      "127.0.0.1",
		Database:  "oss",
		User:      "root",
		Password:  "",
	}

	adapter, err := dolt.New(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer adapter.Close()

	// Get the session
	sessionID := "1d24de72-41df-4ec8-8731-4230174e45d9"
	session, err := adapter.GetSession(sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get session: %v\n", err)
		adapter.Close()
		os.Exit(1) //nolint:gocritic // adapter.Close() called explicitly above
	}

	fmt.Printf("Current UUID: %s\n", session.Claude.UUID)
	fmt.Printf("Updating to: 50f0d309-6f71-460d-9877-fc615c8fd07f\n")

	// Update the UUID
	session.Claude.UUID = "50f0d309-6f71-460d-9877-fc615c8fd07f"

	err = adapter.UpdateSession(session)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to update session: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("✓ UUID updated successfully")
}
