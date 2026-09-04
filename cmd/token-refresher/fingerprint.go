package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/vbonnet/dear-agent/pkg/llm/auth"
)

// credentialsFingerprint returns a short, non-reversible fingerprint of the
// refresh token currently on disk, plus the file's modification time.
//
// This exists to identify the process that kills the OAuth token family
// (ce-77ip). The refresh token is single-use and rotating: whoever spends it
// gets a new one, and any OTHER client that later presents the spent token
// looks like a replay attack, so the server revokes the entire family and the
// operator has to run `claude /login` again.
//
// ~15 independent OAuth clients on this host share ~/.claude/.credentials.json
// (the Desktop-embedded claude-code runtime, CLI sessions, agm sandbox spawns).
// Only this binary takes the cross-process lock, so the logs could never say
// which client actually spent the token. Recording the fingerprint on every
// tick makes that inferable after the fact:
//
//   - fingerprint CHANGED between our ticks, and we did not refresh -> a third
//     party rotated the token and wrote the new one back. We were about to
//     present a stale token; that client is the rotator.
//   - fingerprint UNCHANGED right up to a token_family_dead outcome -> the
//     token was spent by a client that never wrote the result back to this
//     file (a different credential store, or another machine entirely).
//
// Returns empty strings when the credentials cannot be read; the caller treats
// the fingerprint as best-effort diagnostics and never fails a refresh over it.
func credentialsFingerprint(path string) (fp string, modTime string) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", ""
		}
		path = filepath.Join(home, ".claude", ".credentials.json")
	}

	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", ""
	}
	modTime = info.ModTime().UTC().Format("2006-01-02T15:04:05Z")

	data, err := os.ReadFile(path)
	if err != nil {
		return "", modTime
	}

	var creds struct {
		ClaudeAiOauth struct {
			RefreshToken string `json:"refreshToken"`
		} `json:"claudeAiOauth"`
	}
	if err := json.Unmarshal(data, &creds); err != nil {
		return "", modTime
	}
	// Shared with the quarantine record so audit lines and quarantine markers
	// live in the same fingerprint space and a post-mortem can match them up.
	return auth.RefreshTokenFingerprint(creds.ClaudeAiOauth.RefreshToken), modTime
}
