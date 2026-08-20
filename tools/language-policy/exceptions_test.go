package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

var refNow = time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)

func TestLoadStoreRejectsMalformed(t *testing.T) {
	cases := []struct {
		name, in, wantErr string
	}{
		{"bad json", `{"rule":"r","path":"p"`, "invalid JSON"},
		{"missing path", `{"rule":"r","status":"active"}`, "required"},
		{"unknown status", `{"rule":"r","path":"p","status":"activ"}`, "unknown status"},
		{
			"duplicate entry",
			`{"rule":"r","path":"p","status":"active"}` + "\n" + `{"rule":"r","path":"./p","status":"active"}`,
			"duplicate",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadStore(strings.NewReader(tc.in))
			if err == nil {
				t.Fatalf("LoadStore(%q) succeeded, want error", tc.in)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

// A corrupt store must fail loudly. Degrading to "no waivers" would fail every
// waived script at once and read as a mass policy violation.
func TestLoadStoreDoesNotDegradeToEmpty(t *testing.T) {
	if _, err := LoadStore(strings.NewReader("{oops}\n")); err == nil {
		t.Fatal("corrupt store parsed without error")
	}
}

func TestActive(t *testing.T) {
	store, err := LoadStore(strings.NewReader(strings.Join([]string{
		`{"rule":"bash-20-line-limit","path":"a.sh","status":"active","sunset":null}`,
		`{"rule":"bash-20-line-limit","path":"b.sh","status":"active","sunset":"2099-01-01"}`,
		`{"rule":"bash-20-line-limit","path":"c.sh","status":"active","sunset":"2020-01-01"}`,
		`{"rule":"bash-20-line-limit","path":"d.sh","status":"revoked","sunset":null}`,
		`{"rule":"bash-20-line-limit","path":"e.sh","status":"active","sunset":"not-a-date"}`,
	}, "\n")))
	if err != nil {
		t.Fatalf("LoadStore: %v", err)
	}
	cases := []struct {
		path string
		want bool
		why  string
	}{
		{"a.sh", true, "no sunset means open-ended"},
		{"b.sh", true, "sunset in the future"},
		{"c.sh", false, "sunset already passed"},
		{"d.sh", false, "revoked"},
		{"e.sh", false, "unparseable sunset must fail closed"},
		{"missing.sh", false, "no entry"},
		{"./a.sh", true, "leading ./ is normalized"},
	}
	for _, tc := range cases {
		if got := store.Active("bash-20-line-limit", tc.path, refNow); got != tc.want {
			t.Errorf("Active(%q) = %v, want %v (%s)", tc.path, got, tc.want, tc.why)
		}
	}
	// A waiver is scoped to its rule.
	if store.Active("some-other-rule", "a.sh", refNow) {
		t.Error("waiver leaked across rules")
	}
}

func TestExpired(t *testing.T) {
	store, err := LoadStore(strings.NewReader(strings.Join([]string{
		`{"rule":"r","path":"a.sh","status":"active","sunset":"2020-01-01"}`,
		`{"rule":"r","path":"b.sh","status":"active","sunset":null}`,
		`{"rule":"r","path":"c.sh","status":"revoked","sunset":"2020-01-01"}`,
	}, "\n")))
	if err != nil {
		t.Fatalf("LoadStore: %v", err)
	}
	got := store.Expired(refNow)
	if len(got) != 1 || got[0].Path != "a.sh" {
		t.Fatalf("Expired = %+v, want exactly a.sh", got)
	}
}

func TestCheckSorted(t *testing.T) {
	sorted := []Exception{{Rule: "a", Path: "1.sh"}, {Rule: "a", Path: "2.sh"}, {Rule: "b", Path: "0.sh"}}
	if err := CheckSorted(sorted); err != nil {
		t.Errorf("CheckSorted(sorted) = %v, want nil", err)
	}
	unsorted := []Exception{{Rule: "b", Path: "0.sh"}, {Rule: "a", Path: "1.sh"}}
	if err := CheckSorted(unsorted); err == nil {
		t.Error("CheckSorted(unsorted) = nil, want error")
	}
}

// The on-disk form must round-trip exactly, so rewriting the store never
// produces spurious diff churn.
func TestWriteStoreRoundTrip(t *testing.T) {
	in := `{"rule":"bash-20-line-limit","path":"a.sh","status":"active","reason":"why","approver":"vbonnet","sunset":null,"added":"2026-04-24"}` + "\n"
	store, err := LoadStore(strings.NewReader(in))
	if err != nil {
		t.Fatalf("LoadStore: %v", err)
	}
	var buf bytes.Buffer
	if err := WriteStore(&buf, store.All); err != nil {
		t.Fatalf("WriteStore: %v", err)
	}
	if buf.String() != in {
		t.Errorf("round trip changed the bytes:\n got: %q\nwant: %q", buf.String(), in)
	}
}
