package temporal

import (
	"fmt"
	"strings"
)

// Package temporal provides Temporal-based session management as an alternative to tmux.
// This package mirrors the functionality of internal/tmux but uses Temporal workflows
// to manage session state and execution across distributed environments.

// Helper functions for working with temporal sessions

// ValidateSessionName checks if a session name is valid
// Session names should be non-empty and not contain special characters
func ValidateSessionName(name string) error {
	if name == "" {
		return fmt.Errorf("session name cannot be empty")
	}
	if strings.ContainsAny(name, " \t\n\r") {
		return fmt.Errorf("session name cannot contain whitespace")
	}
	return nil
}

// ValidateWorkdir checks if a working directory path is valid
// Stub implementation: just checks for empty string
func ValidateWorkdir(workdir string) error {
	if workdir == "" {
		return fmt.Errorf("working directory cannot be empty")
	}
	return nil
}

// FormatSessionInfo formats a SessionInfo struct as a human-readable string
func FormatSessionInfo(info SessionInfo) string {
	return fmt.Sprintf("Session: %s, Clients: %d, Attached: %s",
		info.Name, info.AttachedClients, info.AttachedList)
}

// FormatClientInfo formats a ClientInfo struct as a human-readable string
func FormatClientInfo(info ClientInfo) string {
	return fmt.Sprintf("Client: %s@%s (PID: %d)",
		info.SessionName, info.TTY, info.PID)
}

// SessionExists is a convenience function that wraps HasSession
// It returns true if the session exists, false otherwise
func SessionExists(client TemporalInterface, name string) bool {
	exists, err := client.HasSession(name)
	if err != nil {
		return false
	}
	return exists
}

// GetSessionCount returns the total number of active sessions
func GetSessionCount(client TemporalInterface) (int, error) {
	sessions, err := client.ListSessions()
	if err != nil {
		return 0, err
	}
	return len(sessions), nil
}

// GetClientCount returns the total number of clients for a session
func GetClientCount(client TemporalInterface, sessionName string) (int, error) {
	clients, err := client.ListClients(sessionName)
	if err != nil {
		return 0, err
	}
	return len(clients), nil
}

// FindSessionByName searches for a session by exact name match
func FindSessionByName(client TemporalInterface, name string) (*SessionInfo, error) {
	sessions, err := client.ListSessionsWithInfo()
	if err != nil {
		return nil, err
	}

	for _, session := range sessions {
		if session.Name == name {
			return &session, nil
		}
	}

	return nil, fmt.Errorf("session not found: %s", name)
}

// GetAllSessionInfo retrieves detailed information for all sessions
// This is a convenience wrapper around ListSessionsWithInfo
func GetAllSessionInfo(client TemporalInterface) ([]SessionInfo, error) {
	return client.ListSessionsWithInfo()
}
