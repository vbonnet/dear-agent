package main

import (
	"testing"
	"time"
)

func TestParseDuration_Days(t *testing.T) {
	d, err := parseDuration("7d")
	if err != nil {
		t.Fatalf("parseDuration(7d): %v", err)
	}
	if d != 7*24*time.Hour {
		t.Errorf("parseDuration(7d) = %v, want 168h", d)
	}
}

func TestParseDuration_SingleDay(t *testing.T) {
	d, err := parseDuration("1d")
	if err != nil {
		t.Fatalf("parseDuration(1d): %v", err)
	}
	if d != 24*time.Hour {
		t.Errorf("parseDuration(1d) = %v, want 24h", d)
	}
}

func TestParseDuration_Hours(t *testing.T) {
	d, err := parseDuration("3h")
	if err != nil {
		t.Fatalf("parseDuration(3h): %v", err)
	}
	if d != 3*time.Hour {
		t.Errorf("parseDuration(3h) = %v, want 3h", d)
	}
}

func TestParseDuration_Invalid(t *testing.T) {
	if _, err := parseDuration("invalid"); err == nil {
		t.Error("parseDuration(invalid) should return error")
	}
}
