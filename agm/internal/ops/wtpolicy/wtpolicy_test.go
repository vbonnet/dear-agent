package wtpolicy

import (
	"errors"
	"testing"
)

type fakeProbe struct {
	dirty    map[string]bool
	dirtyErr map[string]error
	anc      map[string]bool
	ancErr   map[string]error
	prState  map[string]string
	prKnown  map[string]bool
}

func (f *fakeProbe) IsDirty(p string) (bool, error) {
	if e := f.dirtyErr[p]; e != nil {
		return true, e
	}
	return f.dirty[p], nil
}
func (f *fakeProbe) IsAncestor(_, branch, _ string) (bool, error) {
	if e := f.ancErr[branch]; e != nil {
		return false, e
	}
	return f.anc[branch], nil
}
func (f *fakeProbe) PRState(_, branch string) (string, bool) {
	return f.prState[branch], f.prKnown[branch]
}

func TestDirty(t *testing.T) {
	f := &fakeProbe{
		dirty:    map[string]bool{"/clean": false, "/messy": true},
		dirtyErr: map[string]error{"/err": errors.New("x")},
	}
	if d, known := Dirty(f, "/clean"); d || !known {
		t.Fatalf("clean = (%v,%v), want (false,true)", d, known)
	}
	if d, known := Dirty(f, "/messy"); !d || !known {
		t.Fatalf("messy = (%v,%v), want (true,true)", d, known)
	}
	// Probe failure must be (true,false): unprovable ⇒ not safe, distinctly.
	if d, known := Dirty(f, "/err"); !d || known {
		t.Fatalf("err = (%v,%v), want (true,false)", d, known)
	}
}

func TestProvablyMerged(t *testing.T) {
	tests := []struct {
		name        string
		branch      string
		checkPR     bool
		setup       func(*fakeProbe)
		wantMerged  bool
		wantReason  string
		wantPRState string
		wantAncErr  bool
	}{
		{
			name: "ancestor of base ⇒ merged (squash-safe FF path)", branch: "b",
			setup:       func(f *fakeProbe) { f.anc = map[string]bool{"b": true} },
			wantMerged:  true,
			wantReason:  "ancestor-of-base",
			wantPRState: "(ancestor)",
		},
		{
			name: "squash: not ancestor but PR MERGED ⇒ merged", branch: "b", checkPR: true,
			setup: func(f *fakeProbe) {
				f.prState = map[string]string{"b": "MERGED"}
				f.prKnown = map[string]bool{"b": true}
			},
			wantMerged:  true,
			wantReason:  "pr-merged",
			wantPRState: "MERGED",
		},
		{
			name: "open PR ⇒ not merged", branch: "b", checkPR: true,
			setup: func(f *fakeProbe) {
				f.prState = map[string]string{"b": "OPEN"}
				f.prKnown = map[string]bool{"b": true}
			},
			wantPRState: "OPEN",
		},
		{
			name: "pr-check off ⇒ squash kept conservatively", branch: "b",
			wantPRState: "(pr-check-off)",
		},
		{
			name: "pr unknown ⇒ not merged", branch: "b", checkPR: true,
			wantPRState: "UNKNOWN",
		},
		{
			name: "ancestor probe error ⇒ AncestorErr, not merged", branch: "b", checkPR: true,
			setup:       func(f *fakeProbe) { f.ancErr = map[string]error{"b": errors.New("boom")} },
			wantAncErr:  true,
			wantPRState: "UNKNOWN",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeProbe{}
			if tc.setup != nil {
				tc.setup(f)
			}
			mv := ProvablyMerged(f, "/repo", tc.branch, "origin/main", tc.checkPR)
			if mv.Merged != tc.wantMerged || mv.Reason != tc.wantReason || mv.PRState != tc.wantPRState {
				t.Fatalf("got merged=%v reason=%q pr=%q, want merged=%v reason=%q pr=%q",
					mv.Merged, mv.Reason, mv.PRState, tc.wantMerged, tc.wantReason, tc.wantPRState)
			}
			if (mv.AncestorErr != nil) != tc.wantAncErr {
				t.Fatalf("AncestorErr = %v, wantErr=%v", mv.AncestorErr, tc.wantAncErr)
			}
			if mv.Merged && !mv.Pushed {
				t.Fatal("a proven merge implies the branch reached the remote")
			}
		})
	}
}
