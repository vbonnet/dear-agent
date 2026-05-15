package backlog

import (
	"reflect"
	"testing"
)

func TestParseStatus(t *testing.T) {
	cases := map[string]Status{
		"":                     StatusUnknown,
		"pending":              StatusPending,
		"`pending`":            StatusPending,
		"in-flight (claude/x)": StatusInFlight,
		"in flight":            StatusInFlight,
		"blocked (waiting)":    StatusBlocked,
		"done":                 StatusDone,
		"`done (#40)`":         StatusDone,
		"done — note":          StatusDone,
		"~~old~~ DONE":         StatusDone,
		"something unexpected": StatusUnknown,
	}
	for in, want := range cases {
		if got := parseStatus(in); got != want {
			t.Errorf("parseStatus(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParsePriority(t *testing.T) {
	cases := map[string]Priority{
		"HIGH": PriorityHigh, "P0": PriorityHigh, "P1": PriorityHigh,
		"MED": PriorityMed, "MEDIUM": PriorityMed, "P2": PriorityMed,
		"LOW": PriorityLow, "P3": PriorityLow,
		"": PriorityUnset, "M": PriorityUnset, "garbage": PriorityUnset,
	}
	for in, want := range cases {
		if got := parsePriority(in); got != want {
			t.Errorf("parsePriority(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseEffort(t *testing.T) {
	cases := map[string]Effort{
		"S": EffortSmall, "small": EffortSmall,
		"M": EffortMedium, "MEDIUM": EffortMedium,
		"L": EffortLarge, "large": EffortLarge,
		"": EffortUnknown, "XL": EffortUnknown,
	}
	for in, want := range cases {
		if got := parseEffort(in); got != want {
			t.Errorf("parseEffort(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseDeps(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"—", nil},
		{"-", nil},
		{"", nil},
		{"n/a", nil},
		{"none", nil},
		{"0.1", []string{"0.1"}},
		{"3.3, 1.6", []string{"3.3", "1.6"}},
		{"0.*", []string{"0.*"}},
		{"`0.1` `0.2`", []string{"0.1", "0.2"}},
		{"1.1;1.2", []string{"1.1", "1.2"}},
	}
	for _, c := range cases {
		if got := parseDeps(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("parseDeps(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParsePhase(t *testing.T) {
	cases := map[string]int{
		"0.1": 0, "1.2": 1, "6.3": 6, "10.4": 10,
		"X.1": -1, "DEAR-X.5": -1, "nodot": -1, "": -1,
	}
	for in, want := range cases {
		if got := parsePhase(in); got != want {
			t.Errorf("parsePhase(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestIsPhaseWildcard(t *testing.T) {
	cases := map[string]bool{
		"0.*": true, "12.*": true,
		"*": false, ".*": false, "1.1": false, "": false,
	}
	for in, want := range cases {
		if got := IsPhaseWildcard(in); got != want {
			t.Errorf("IsPhaseWildcard(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestCleanCell(t *testing.T) {
	cases := map[string]string{
		"`x`":          "x",
		"**bold**":     "bold",
		"~~struck~~ Z": "struck Z",
		"  spaced  ":   "spaced",
	}
	for in, want := range cases {
		if got := cleanCell(in); got != want {
			t.Errorf("cleanCell(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEnumStrings(t *testing.T) {
	if StatusInFlight.String() != "in-flight" || StatusDone.String() != "done" {
		t.Error("Status.String mismatch")
	}
	if PriorityHigh.String() != "HIGH" || PriorityUnset.String() != "—" {
		t.Error("Priority.String mismatch")
	}
	if EffortSmall.String() != "S" || EffortUnknown.String() != "?" {
		t.Error("Effort.String mismatch")
	}
}
