package audit

import "testing"

func TestSeverityRankOrdering(t *testing.T) {
	cases := []struct {
		s    Severity
		want int
	}{
		{SeverityP0, 0},
		{SeverityP1, 1},
		{SeverityP2, 2},
		{SeverityP3, 3},
		{Severity("nope"), -1},
	}
	for _, tc := range cases {
		if got := tc.s.Rank(); got != tc.want {
			t.Errorf("Severity(%q).Rank() = %d, want %d", tc.s, got, tc.want)
		}
	}
}

func TestSeverityIsValid(t *testing.T) {
	if !SeverityP0.IsValid() {
		t.Error("P0 should be valid")
	}
	if Severity("p4").IsValid() {
		t.Error("p4 should be invalid")
	}
}

func TestCadenceIsValid(t *testing.T) {
	if !CadenceDaily.IsValid() {
		t.Error("daily should be valid")
	}
	if Cadence("hourly").IsValid() {
		t.Error("hourly is not in v1; should be invalid")
	}
}

func TestCheckMetaValidate(t *testing.T) {
	good := CheckMeta{ID: "build", Cadence: CadenceDaily, SeverityCeiling: SeverityP0}
	if err := good.Validate(); err != nil {
		t.Errorf("good meta failed validate: %v", err)
	}

	cases := []CheckMeta{
		{},                                    // empty id
		{ID: "x", Cadence: "invalid", SeverityCeiling: SeverityP0},
		{ID: "x", Cadence: CadenceDaily, SeverityCeiling: "P9"},
		{ID: "with\x07ctrl", Cadence: CadenceDaily, SeverityCeiling: SeverityP0},
	}
	for i, c := range cases {
		if err := c.Validate(); err == nil {
			t.Errorf("case %d should have failed: %+v", i, c)
		}
	}
}

func TestClampSeverity(t *testing.T) {
	cases := []struct {
		want, ceiling, expect Severity
	}{
		{SeverityP0, SeverityP1, SeverityP1}, // can't escalate above ceiling
		{SeverityP2, SeverityP1, SeverityP2}, // already below ceiling
		{SeverityP1, SeverityP1, SeverityP1}, // equal
		{Severity("bad"), SeverityP2, SeverityP2},
	}
	for _, tc := range cases {
		if got := ClampSeverity(tc.want, tc.ceiling); got != tc.expect {
			t.Errorf("ClampSeverity(%s, %s) = %s, want %s", tc.want, tc.ceiling, got, tc.expect)
		}
	}
}
