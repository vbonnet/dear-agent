package main

import (
	"testing"
	"time"
)

func TestParseDuration_Days(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"1d", 24 * time.Hour},
		{"7d", 7 * 24 * time.Hour},
		{"30d", 30 * 24 * time.Hour},
	}
	for _, tc := range cases {
		got, err := parseDuration(tc.in)
		if err != nil {
			t.Errorf("parseDuration(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseDuration(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParseDuration_Standard(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"1h", time.Hour},
		{"30m", 30 * time.Minute},
		{"1h30m", 90 * time.Minute},
	}
	for _, tc := range cases {
		got, err := parseDuration(tc.in)
		if err != nil {
			t.Errorf("parseDuration(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseDuration(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParseDuration_Invalid(t *testing.T) {
	t.Parallel()
	_, err := parseDuration("notaduration")
	if err == nil {
		t.Error("parseDuration(\"notaduration\"): expected error, got nil")
	}
}
