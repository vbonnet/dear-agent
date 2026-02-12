// Package astrocyte provides utilities for parsing Astrocyte diagnosis files.
//
// This package is part of the AGM session monitoring infrastructure and provides
// tools for reading and analyzing Astrocyte daemon diagnosis logs.
//
// Astrocyte is the AGM daemon that detects and auto-recovers stuck sessions
// (zero-token galloping, permission prompts, API stalls, etc.). It writes
// diagnosis files to ~/.agm/astrocyte/diagnoses/ in markdown format.
//
// Key features:
//   - Parse individual diagnosis files
//   - Parse entire diagnosis directories
//   - Extract hang type, recovery status, session ID, timestamp
//   - Filter diagnoses by session or time range
//   - Handle malformed files gracefully
//
// Example usage:
//
//	// Parse a single diagnosis file
//	diag, err := astrocyte.ParseDiagnosisFile("~/.agm/astrocyte/diagnoses/session-2026-01-01.md")
//	if err != nil {
//	    return err
//	}
//	fmt.Printf("Session: %s, Type: %s, Recovery: %v\n",
//	    diag.SessionID, diag.Type, diag.RecoverySuccess)
//
//	// Parse all diagnosis files in a directory
//	diagnoses, err := astrocyte.ParseDiagnosisDirectory("~/.agm/astrocyte/diagnoses")
//	if err != nil {
//	    log.Printf("Some files failed to parse: %v", err)
//	}
//
//	// Filter by session
//	sessionDiags := astrocyte.FilterBySession(diagnoses, "my-session")
//
// The parser handles various diagnosis file formats and extracts structured
// information for use in session monitoring and health dashboards.
package astrocyte
