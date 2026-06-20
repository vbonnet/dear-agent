package supervisor

import "testing"

func TestCountOpenNonDraft(t *testing.T) {
	cases := []struct {
		name string
		json string
		want int
	}{
		{"empty", `[]`, 0},
		{"all ready", `[{"number":1,"isDraft":false},{"number":2,"isDraft":false}]`, 2},
		{"drafts excluded", `[{"number":1,"isDraft":false},{"number":2,"isDraft":true},{"number":3,"isDraft":false}]`, 2},
		{"all drafts", `[{"number":1,"isDraft":true}]`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := countOpenNonDraft([]byte(tc.json))
			if err != nil {
				t.Fatalf("countOpenNonDraft: %v", err)
			}
			if got != tc.want {
				t.Errorf("count = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestCountOpenNonDraft_BadJSON(t *testing.T) {
	if _, err := countOpenNonDraft([]byte(`not json`)); err == nil {
		t.Error("expected error on malformed JSON")
	}
}
