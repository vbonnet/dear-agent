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

// TestListWorkflowsRejectsMalformedQuery pins the review finding that a
// malformed query string bypassed the run-state filter entirely.
//
// Go's URL.Query() discards pairs it cannot parse and reports no error, so
// "?state=running;state=typo" yielded an empty slice. ParseRunStateFilterValues
// treats empty as the any-state filter, so the handler answered 200 with EVERY
// run instead of refusing the request. A validation gate a typo can silently
// switch off is not a gate.
func TestListWorkflowsRejectsMalformedQuery(t *testing.T) {
	for _, query := range []string{
		"?state=running;state=typo",
		"?state=running;limit=10",
		"?state=%ZZ",
	} {
		t.Run(query, func(t *testing.T) {
			fixture := newFixture(t)
			response, err := http.Get(fixture.ts.URL + "/workflows" + query)
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusBadRequest {
				t.Errorf("status = %d, want 400: a query the server cannot parse must be "+
					"refused, not silently treated as no filter", response.StatusCode)
			}
		})
	}
}
