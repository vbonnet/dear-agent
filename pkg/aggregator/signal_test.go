package aggregator

import (
	"strings"
	"testing"
	"time"
)

func TestKindValidate(t *testing.T) {
	t.Parallel()
	for _, k := range AllKinds() {
		if err := k.Validate(); err != nil {
			t.Errorf("AllKinds()[%s] should be valid: %v", k, err)
		}
	}
	if err := Kind("").Validate(); err == nil {
		t.Error("empty kind should be invalid")
	}
	if err := Kind("nonsense").Validate(); err == nil {
		t.Error("unknown kind should be invalid")
	}
}

func TestSignalValidate(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC)
	good := Signal{
		ID:          "id1",
		Kind:        KindLintTrend,
		Subject:     "pkg/foo/bar.go",
		Value:       7,
		CollectedAt: at,
	}
	if err := good.Validate(); err != nil {
		t.Fatalf("good signal should validate: %v", err)
	}

	cases := []struct {
		name    string
		mut     func(s *Signal)
		wantSub string
	}{
		{"empty ID", func(s *Signal) { s.ID = "" }, "empty ID"},
		{"unknown kind", func(s *Signal) { s.Kind = "wat" }, "unknown signal kind"},
		{"empty subject", func(s *Signal) { s.Subject = "  " }, "empty subject"},
		{"zero time", func(s *Signal) { s.CollectedAt = time.Time{} }, "zero CollectedAt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := good
			tc.mut(&s)
			err := s.Validate()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("err = %q, want substring %q", err, tc.wantSub)
			}
		})
	}
}
