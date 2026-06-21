package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeCreds writes a minimal credentials.json with the given access token and
// expiry, returning its path.
func writeCreds(t *testing.T, accessToken string, expiresAt time.Time) string {
	t.Helper()
	block := map[string]any{}
	if accessToken != "" {
		block["accessToken"] = accessToken
	}
	if !expiresAt.IsZero() {
		block["expiresAt"] = expiresAt.UnixMilli()
	}
	data, err := json.Marshal(map[string]any{"claudeAiOauth": block})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := filepath.Join(t.TempDir(), ".credentials.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func TestRun_ExitCodes(t *testing.T) {
	cases := []struct {
		name      string
		expiresIn time.Duration
		noToken   bool
		wantCode  int
	}{
		{name: "fresh", expiresIn: time.Hour, wantCode: 0},
		{name: "expiring", expiresIn: 5 * time.Minute, wantCode: 1},
		{name: "expired", expiresIn: -time.Minute, wantCode: 2},
		{name: "missing", noToken: true, wantCode: 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var path string
			if tc.noToken {
				path = writeCreds(t, "", time.Time{})
			} else {
				path = writeCreds(t, "access", time.Now().Add(tc.expiresIn))
			}
			var out, errb bytes.Buffer
			code, err := run([]string{"--credentials", path}, &out, &errb)
			if err != nil {
				t.Fatalf("run error: %v", err)
			}
			if code != tc.wantCode {
				t.Errorf("exit code = %d, want %d (out=%q)", code, tc.wantCode, out.String())
			}
		})
	}
}

// --trail appends exactly one supervisor.credential.stale record when stale,
// and none when fresh.
func TestRun_TrailAppendsOnlyWhenStale(t *testing.T) {
	t.Run("stale writes a record", func(t *testing.T) {
		creds := writeCreds(t, "access", time.Now().Add(2*time.Minute))
		trailPath := filepath.Join(t.TempDir(), "trail.jsonl")
		var out, errb bytes.Buffer

		code, err := run([]string{"--credentials", creds, "--trail", trailPath, "--json"}, &out, &errb)
		if err != nil {
			t.Fatalf("run error: %v", err)
		}
		if code != 1 {
			t.Errorf("exit code = %d, want 1", code)
		}

		data, err := os.ReadFile(trailPath)
		if err != nil {
			t.Fatalf("read trail: %v", err)
		}
		var rec struct {
			Role string `json:"role"`
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(bytes.TrimSpace(data), &rec); err != nil {
			t.Fatalf("unmarshal record: %v\nraw: %q", err, string(data))
		}
		if rec.Kind != "supervisor.credential.stale" {
			t.Errorf("kind = %q, want supervisor.credential.stale", rec.Kind)
		}
		if rec.Role != "overseer" {
			t.Errorf("role = %q, want overseer", rec.Role)
		}
	})

	t.Run("fresh writes no trail file", func(t *testing.T) {
		creds := writeCreds(t, "access", time.Now().Add(time.Hour))
		trailPath := filepath.Join(t.TempDir(), "trail.jsonl")
		var out, errb bytes.Buffer

		if _, err := run([]string{"--credentials", creds, "--trail", trailPath}, &out, &errb); err != nil {
			t.Fatalf("run error: %v", err)
		}
		if _, err := os.Stat(trailPath); !os.IsNotExist(err) {
			t.Errorf("fresh credential should not create a trail file, stat err = %v", err)
		}
	})
}

func TestRun_JSONOutput(t *testing.T) {
	creds := writeCreds(t, "access", time.Now().Add(-time.Minute))
	var out, errb bytes.Buffer
	code, err := run([]string{"--credentials", creds, "--json"}, &out, &errb)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	var got map[string]any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal json: %v\nraw: %q", err, out.String())
	}
	if got["state"] != "expired" {
		t.Errorf("state = %v, want expired", got["state"])
	}
	if got["stale"] != true {
		t.Errorf("stale = %v, want true", got["stale"])
	}
}

func TestRun_BadFlag(t *testing.T) {
	var out, errb bytes.Buffer
	code, err := run([]string{"--nonsense"}, &out, &errb)
	if err != nil {
		t.Fatalf("run should not return error for bad flag, got %v", err)
	}
	if code != exitUsage {
		t.Errorf("exit code = %d, want %d", code, exitUsage)
	}
}
