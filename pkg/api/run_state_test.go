package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestListWorkflowsRejectsInvalidRunState(t *testing.T) {
	fixture := newFixture(t)
	if err := fixture.state.Close(); err != nil {
		t.Fatalf("close test database: %v", err)
	}

	response, err := http.Get(fixture.ts.URL + "/workflows?state=typo")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", response.StatusCode)
	}
	var body map[string]string
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["code"] != "invalid_state" {
		t.Errorf("code = %q, want invalid_state", body["code"])
	}
}

func TestListWorkflowsPreservesEmptyRunStateFilter(t *testing.T) {
	fixture := newFixture(t)
	response, err := http.Get(fixture.ts.URL + "/workflows?state=")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.StatusCode)
	}
}

// Reading only the first "state" value made validation order-dependent: an
// unknown spelling after a valid one was silently ignored, while the reverse
// order returned 400. A repeated filter is refused either way.
func TestListWorkflowsRejectsRepeatedRunState(t *testing.T) {
	for _, query := range []string{
		"?state=running&state=typo",
		"?state=typo&state=running",
		"?state=running&state=failed",
	} {
		t.Run(query, func(t *testing.T) {
			fixture := newFixture(t)
			response, err := http.Get(fixture.ts.URL + "/workflows" + query)
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d for %s, want 400", response.StatusCode, query)
			}
		})
	}
}
