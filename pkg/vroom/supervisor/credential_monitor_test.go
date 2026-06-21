package supervisor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/pkg/vroom/decisiontrail"
)

// refTime is a fixed clock so expiry math is deterministic.
var refTime = time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)

func fixedNow() time.Time { return refTime }

// writeCreds writes a credentials.json with the given access/refresh tokens and
// expiry, and returns its path. An empty accessToken omits the token; a zero
// expiresAt omits the expiry field.
func writeCreds(t *testing.T, accessToken, refreshToken string, expiresAt time.Time) string {
	t.Helper()
	block := map[string]any{}
	if accessToken != "" {
		block["accessToken"] = accessToken
	}
	if refreshToken != "" {
		block["refreshToken"] = refreshToken
	}
	if !expiresAt.IsZero() {
		block["expiresAt"] = expiresAt.UnixMilli()
	}
	doc := map[string]any{"claudeAiOauth": block}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal creds: %v", err)
	}
	path := filepath.Join(t.TempDir(), ".credentials.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write creds: %v", err)
	}
	return path
}

func TestCheckCredentialFreshness_States(t *testing.T) {
	cases := []struct {
		name      string
		expiresIn time.Duration // relative to refTime; ignored if noToken
		noToken   bool
		noExpiry  bool
		want      CredentialState
	}{
		{name: "fresh, far from expiry", expiresIn: time.Hour, want: CredentialFresh},
		{name: "fresh, just outside window", expiresIn: 11 * time.Minute, want: CredentialFresh},
		{name: "expiring, inside window", expiresIn: 5 * time.Minute, want: CredentialExpiring},
		{name: "expiring, at window edge", expiresIn: 9 * time.Minute, want: CredentialExpiring},
		{name: "expired, just past", expiresIn: -time.Second, want: CredentialExpired},
		{name: "expired, long past", expiresIn: -time.Hour, want: CredentialExpired},
		{name: "missing token", noToken: true, want: CredentialMissing},
		{name: "token without expiry treated fresh", noExpiry: true, want: CredentialFresh},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var path string
			switch {
			case tc.noToken:
				path = writeCreds(t, "", "refresh-xyz", time.Time{})
			case tc.noExpiry:
				path = writeCreds(t, "access-abc", "refresh-xyz", time.Time{})
			default:
				path = writeCreds(t, "access-abc", "refresh-xyz", refTime.Add(tc.expiresIn))
			}

			got := CheckCredentialFreshness(path, fixedNow, DefaultCredentialStaleWindow)
			if got.State != tc.want {
				t.Errorf("State = %v, want %v", got.State, tc.want)
			}
		})
	}
}

// The core DoD assertion: a token expiring within 10 minutes (and one already
// expired) is flagged stale, while a comfortably-fresh token is not.
func TestCheckCredentialFreshness_DetectsStale(t *testing.T) {
	expiring := CheckCredentialFreshness(
		writeCreds(t, "access", "refresh", refTime.Add(3*time.Minute)), fixedNow, 0)
	if !expiring.State.Stale() {
		t.Errorf("token expiring in 3m should be stale, got %v", expiring.State)
	}
	if expiring.State != CredentialExpiring {
		t.Errorf("want CredentialExpiring, got %v", expiring.State)
	}

	expired := CheckCredentialFreshness(
		writeCreds(t, "access", "refresh", refTime.Add(-time.Minute)), fixedNow, 0)
	if !expired.State.Stale() {
		t.Errorf("expired token should be stale, got %v", expired.State)
	}
}

func TestCheckCredentialFreshness_Fresh(t *testing.T) {
	fresh := CheckCredentialFreshness(
		writeCreds(t, "access", "refresh", refTime.Add(2*time.Hour)), fixedNow, 0)
	if fresh.State.Stale() {
		t.Errorf("token valid for 2h should be fresh, got %v", fresh.State)
	}
	if fresh.State != CredentialFresh {
		t.Errorf("want CredentialFresh, got %v", fresh.State)
	}
}

func TestCheckCredentialFreshness_MissingFile(t *testing.T) {
	got := CheckCredentialFreshness(
		filepath.Join(t.TempDir(), "does-not-exist.json"), fixedNow, 0)
	if got.State != CredentialMissing {
		t.Errorf("absent file should be Missing, got %v", got.State)
	}
}

func TestCheckCredentialFreshness_CustomWindow(t *testing.T) {
	// With a 30-minute window, a token 20 minutes out is now "expiring".
	path := writeCreds(t, "access", "refresh", refTime.Add(20*time.Minute))
	if got := CheckCredentialFreshness(path, fixedNow, 30*time.Minute); got.State != CredentialExpiring {
		t.Errorf("20m token under 30m window: State = %v, want expiring", got.State)
	}
	// Same token under the default 10-minute window is fresh.
	if got := CheckCredentialFreshness(path, fixedNow, DefaultCredentialStaleWindow); got.State != CredentialFresh {
		t.Errorf("20m token under 10m window: State = %v, want fresh", got.State)
	}
}

func TestCredentialState_ExitCode(t *testing.T) {
	want := map[CredentialState]int{
		CredentialFresh:    0,
		CredentialExpiring: 1,
		CredentialExpired:  2,
		CredentialMissing:  3,
	}
	for state, code := range want {
		if got := state.ExitCode(); got != code {
			t.Errorf("%v.ExitCode() = %d, want %d", state, got, code)
		}
	}
}

// EmitCredentialStale writes exactly one record (with the canonical kind and
// role) when stale, and nothing when fresh.
func TestEmitCredentialStale(t *testing.T) {
	t.Run("stale emits one record", func(t *testing.T) {
		path := writeCreds(t, "access", "", refTime.Add(2*time.Minute))
		f := CheckCredentialFreshness(path, fixedNow, 0)

		var buf bytes.Buffer
		trail := decisiontrail.NewJSONLTrail(&nopWriteCloser{&buf})
		if err := EmitCredentialStale(context.Background(), trail, f); err != nil {
			t.Fatalf("EmitCredentialStale: %v", err)
		}

		var rec struct {
			Role    string         `json:"role"`
			Kind    string         `json:"kind"`
			Payload map[string]any `json:"payload"`
		}
		if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
			t.Fatalf("unmarshal record: %v\nraw: %q", err, buf.String())
		}
		if rec.Kind != CredentialStaleKind {
			t.Errorf("kind = %q, want %q", rec.Kind, CredentialStaleKind)
		}
		if rec.Role != string(RoleOverseer) {
			t.Errorf("role = %q, want %q", rec.Role, RoleOverseer)
		}
		if rec.Payload["state"] != "expiring" {
			t.Errorf("payload.state = %v, want expiring", rec.Payload["state"])
		}
		// No refresh token was written → operator hint should reflect that.
		if rec.Payload["has_refresh_token"] != false {
			t.Errorf("payload.has_refresh_token = %v, want false", rec.Payload["has_refresh_token"])
		}
	})

	t.Run("fresh emits nothing", func(t *testing.T) {
		path := writeCreds(t, "access", "refresh", refTime.Add(time.Hour))
		f := CheckCredentialFreshness(path, fixedNow, 0)

		var buf bytes.Buffer
		trail := decisiontrail.NewJSONLTrail(&nopWriteCloser{&buf})
		if err := EmitCredentialStale(context.Background(), trail, f); err != nil {
			t.Fatalf("EmitCredentialStale: %v", err)
		}
		if buf.Len() != 0 {
			t.Errorf("fresh credential wrote a record: %q", buf.String())
		}
	})

	t.Run("nil trail is a no-op", func(t *testing.T) {
		path := writeCreds(t, "access", "refresh", refTime.Add(-time.Hour))
		f := CheckCredentialFreshness(path, fixedNow, 0)
		if err := EmitCredentialStale(context.Background(), nil, f); err != nil {
			t.Errorf("nil trail should be a no-op, got %v", err)
		}
	})
}

// Sanity: String never panics and is non-empty for known + unknown states.
func TestCredentialState_String(t *testing.T) {
	for _, s := range []CredentialState{CredentialFresh, CredentialExpiring, CredentialExpired, CredentialMissing, CredentialState(99)} {
		if s.String() == "" {
			t.Errorf("String() empty for %d", int(s))
		}
	}
	_ = fmt.Sprint(CredentialFresh)
}
