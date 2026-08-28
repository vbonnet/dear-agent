package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestListWorkflowsRejectsInvalidRunState(t *testing.T) {
	fixture := newFixture(t)
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
