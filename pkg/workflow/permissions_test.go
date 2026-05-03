package workflow

import (
	"errors"
	"testing"
)

func TestPermissionEnforcerNilIsPermissive(t *testing.T) {
	e := DefaultPermissionEnforcer{}
	if err := e.CheckPath(nil, "/anywhere", AccessRead); err != nil {
		t.Errorf("nil perm should allow: %v", err)
	}
	if err := e.CheckHost(nil, "evil.example.com"); err != nil {
		t.Errorf("nil perm should allow: %v", err)
	}
	if err := e.CheckTool(nil, "Anything"); err != nil {
		t.Errorf("nil perm should allow: %v", err)
	}
}

func TestPermissionEnforcerEmptyAllowlistIsPermissive(t *testing.T) {
	// Empty FSRead with non-empty FSWrite still allows reads.
	e := DefaultPermissionEnforcer{}
	p := &Permissions{FSWrite: []string{"foo/**"}}
	if err := e.CheckPath(p, "/wherever", AccessRead); err != nil {
		t.Errorf("empty FSRead should allow: %v", err)
	}
}

func TestPermissionEnforcerPathAllow(t *testing.T) {
	e := DefaultPermissionEnforcer{}
	p := &Permissions{FSWrite: []string{"notes/**"}}
	if err := e.CheckPath(p, "notes/run-1/report.md", AccessWrite); err != nil {
		t.Errorf("expected allow, got %v", err)
	}
	if err := e.CheckPath(p, "secrets/.env", AccessWrite); !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("expected ErrPermissionDenied, got %v", err)
	}
}

func TestPermissionEnforcerPathReadVsWrite(t *testing.T) {
	// A path on the read list is NOT writable.
	e := DefaultPermissionEnforcer{}
	p := &Permissions{FSRead: []string{"src/**"}, FSWrite: []string{"build/**"}}
	if err := e.CheckPath(p, "src/main.go", AccessRead); err != nil {
		t.Errorf("read should pass: %v", err)
	}
	if err := e.CheckPath(p, "src/main.go", AccessWrite); !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("expected write deny, got %v", err)
	}
}

func TestPermissionEnforcerHostAllowExact(t *testing.T) {
	e := DefaultPermissionEnforcer{}
	p := &Permissions{Network: []string{"anthropic.com"}}
	if err := e.CheckHost(p, "anthropic.com"); err != nil {
		t.Errorf("exact match: %v", err)
	}
	if err := e.CheckHost(p, "api.anthropic.com"); err != nil {
		t.Errorf("subdomain bare match: %v", err)
	}
}

func TestPermissionEnforcerHostAllowStar(t *testing.T) {
	e := DefaultPermissionEnforcer{}
	p := &Permissions{Network: []string{"*.example.com"}}
	if err := e.CheckHost(p, "x.example.com"); err != nil {
		t.Errorf("subdomain match: %v", err)
	}
	if err := e.CheckHost(p, "evil.com"); !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("expected deny, got %v", err)
	}
}

func TestPermissionEnforcerHostAllowAll(t *testing.T) {
	e := DefaultPermissionEnforcer{}
	p := &Permissions{Network: []string{"*"}}
	if err := e.CheckHost(p, "anything.example.com"); err != nil {
		t.Errorf("wildcard: %v", err)
	}
}

func TestPermissionEnforcerHostStripsPort(t *testing.T) {
	e := DefaultPermissionEnforcer{}
	p := &Permissions{Network: []string{"anthropic.com"}}
	if err := e.CheckHost(p, "anthropic.com:443"); err != nil {
		t.Errorf("with port: %v", err)
	}
	if err := e.CheckHost(p, "https://anthropic.com:443/v1/messages"); err != nil {
		t.Errorf("URL form: %v", err)
	}
}

func TestPermissionEnforcerToolAllow(t *testing.T) {
	e := DefaultPermissionEnforcer{}
	p := &Permissions{Tools: []string{"Read", "FetchSource"}}
	if err := e.CheckTool(p, "Read"); err != nil {
		t.Errorf("Read: %v", err)
	}
	if err := e.CheckTool(p, "Bash"); !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("expected deny for Bash, got %v", err)
	}
}

func TestMatchGlobDoubleStar(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"notes/**", "notes/run-1/report.md", true},
		{"notes/**", "notes/", true},
		{"notes/**", "src/foo", false},
		{"**/report.md", "notes/run-1/report.md", true},
		{"**/.env", "secrets/.env", true},
		// Plain glob path through filepath.Match.
		{"*.md", "report.md", true},
		{"*.md", "report.txt", false},
	}
	for _, tc := range cases {
		if got := matchGlob(tc.pattern, tc.path); got != tc.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
		}
	}
}
