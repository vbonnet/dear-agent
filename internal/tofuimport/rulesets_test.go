package tofuimport

import (
	"strings"
	"testing"
)

func ruleset(id int, name string) Ruleset {
	return Ruleset{ID: id, Name: name}
}

const canonicalName = "main-zero-bypass"

func TestParseRulesetPagesFlattensAndProvesIdentity(t *testing.T) {
	rulesets, err := ParseRulesetPages([]byte(`[[{"id":1,"name":"a"}],[{"id":2,"name":"b"}]]`))
	if err != nil {
		t.Fatalf("ParseRulesetPages: %v", err)
	}
	if len(rulesets) != 2 || rulesets[1].Name != "b" {
		t.Fatalf("pages were not flattened in order: %+v", rulesets)
	}
}

func TestParseRulesetPagesRejectsEvidenceThatCannotProveAbsence(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{
			// [null] is an array that matches no name. Reading it as "no
			// ruleset exists" would let the next plan create a second active
			// ruleset beside the real one.
			name:    "a null entry is not proof of absence",
			raw:     `[[null]]`,
			wantErr: "without an id or a name",
		},
		{
			name:    "an entry without an id would import the literal string null",
			raw:     `[[{"name":"branch-protection"}]]`,
			wantErr: "without an id or a name",
		},
		{name: "an entry without a name cannot be matched", raw: `[[{"id":7}]]`, wantErr: "without an id or a name"},
		{name: "a non-positive id is not a provider object", raw: `[[{"id":0,"name":"x"}]]`, wantErr: "non-positive"},
		{name: "an empty name matches nothing", raw: `[[{"id":7,"name":""}]]`, wantErr: "empty name"},
		{
			name:    "an unpaginated array means the request shape changed",
			raw:     `[{"id":7,"name":"x"}]`,
			wantErr: "not a paginated array",
		},
		{name: "an error body is not a listing", raw: `{"message":"Bad credentials"}`, wantErr: "not a paginated array"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseRulesetPages([]byte(tt.raw))
			if err == nil {
				t.Fatal("ParseRulesetPages unexpectedly succeeded")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestSelectRulesetIDForCanonicalRepository(t *testing.T) {
	tests := []struct {
		name      string
		rulesets  []Ruleset
		wantID    int
		wantFound bool
		wantErr   string
	}{
		{
			name:      "the canonical id under its canonical name is selected",
			rulesets:  []Ruleset{ruleset(CanonicalRulesetID, canonicalName), ruleset(42, "unrelated")},
			wantID:    CanonicalRulesetID,
			wantFound: true,
		},
		{
			// The importer has to run before as well as after the rename.
			name:      "the canonical id under its pre-rename name is selected",
			rulesets:  []Ruleset{ruleset(CanonicalRulesetID, LegacyRulesetName)},
			wantID:    CanonicalRulesetID,
			wantFound: true,
		},
		{
			name:     "a second ruleset carrying the canonical name is ambiguous",
			rulesets: []Ruleset{ruleset(CanonicalRulesetID, canonicalName), ruleset(999, canonicalName)},
			wantErr:  "found 2",
		},
		{
			// A replacement ID means the ruleset was deleted and recreated.
			// Importing it would bind state to an object no one reviewed.
			name:     "a replacement id fails closed rather than importing the new object",
			rulesets: []Ruleset{ruleset(999, canonicalName)},
			wantErr:  "replacement ID 999",
		},
		{
			name:     "no matching ruleset at all is an error, not an absence",
			rulesets: []Ruleset{ruleset(42, "unrelated")},
			wantErr:  "found 0",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, found, err := SelectRulesetID("dear-agent", canonicalName, tt.rulesets)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("SelectRulesetID unexpectedly succeeded with %d", id)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error %q does not contain %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("SelectRulesetID: %v", err)
			}
			if id != tt.wantID || found != tt.wantFound {
				t.Fatalf("got (%d, %t), want (%d, %t)", id, found, tt.wantID, tt.wantFound)
			}
		})
	}
}

func TestSelectRulesetIDForFleetRepositories(t *testing.T) {
	t.Run("a single legacy-named ruleset is selected", func(t *testing.T) {
		id, found, err := SelectRulesetID("engram-research", canonicalName,
			[]Ruleset{ruleset(77, LegacyRulesetName), ruleset(1, "other")})
		if err != nil || !found || id != 77 {
			t.Fatalf("got (%d, %t, %v), want (77, true, nil)", id, found, err)
		}
	})

	t.Run("no ruleset is a provable absence, not an error", func(t *testing.T) {
		id, found, err := SelectRulesetID("engram-research", canonicalName, []Ruleset{ruleset(1, "other")})
		if err != nil || found || id != 0 {
			t.Fatalf("got (%d, %t, %v), want (0, false, nil)", id, found, err)
		}
	})

	t.Run("duplicates refuse an ambiguous import", func(t *testing.T) {
		_, _, err := SelectRulesetID("engram-research", canonicalName,
			[]Ruleset{ruleset(1, LegacyRulesetName), ruleset(2, LegacyRulesetName)})
		if err == nil || !strings.Contains(err.Error(), "refusing an ambiguous import") {
			t.Fatalf("expected an ambiguity error, got %v", err)
		}
	})

	t.Run("an empty canonical name is rejected", func(t *testing.T) {
		if _, _, err := SelectRulesetID("dear-agent", "", nil); err == nil {
			t.Fatal("SelectRulesetID unexpectedly accepted an empty canonical name")
		}
	})
}

func TestCanonicalRulesetName(t *testing.T) {
	name, err := CanonicalRulesetName([]byte(`{"name":"main-zero-bypass","target":"branch"}`))
	if err != nil || name != canonicalName {
		t.Fatalf("got (%q, %v), want (%q, nil)", name, err, canonicalName)
	}
	for _, raw := range []string{`{}`, `{"name":""}`, `not json`} {
		if _, err := CanonicalRulesetName([]byte(raw)); err == nil {
			t.Errorf("CanonicalRulesetName(%q) unexpectedly succeeded", raw)
		}
	}
}
