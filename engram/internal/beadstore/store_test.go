package beadstore

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeCall records one bd invocation and scripts its result.
type fakeCall struct {
	args   []string
	stdout string
	stderr string
	err    error
}

// fakeRunner returns a Runner that replays scripted results in order and
// records the args of every invocation.
func fakeRunner(t *testing.T, calls *[]fakeCall) Runner {
	t.Helper()
	i := 0
	return func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
		t.Helper()
		if i >= len(*calls) {
			t.Fatalf("unexpected bd invocation #%d: %s %v", i+1, name, args)
		}
		c := &(*calls)[i]
		c.args = args
		i++
		return []byte(c.stdout), []byte(c.stderr), c.err
	}
}

const createdJSON = `{
  "id": "ce-abc",
  "title": "test bead",
  "description": "desc",
  "status": "open",
  "priority": 1,
  "labels": ["foo"]
}`

const shownJSON = `[
  {
    "id": "ce-abc",
    "title": "test bead",
    "description": "desc",
    "status": "open",
    "priority": 1,
    "labels": ["foo"]
  }
]`

// bd --json show <missing> exits 0 and prints an error OBJECT, not an array.
// Verification must parse the payload; exit codes cannot be trusted.
const showMissingJSON = `{
  "error": "no issues found matching the provided IDs",
  "schema_version": 1
}`

func testReq() CreateRequest {
	return CreateRequest{
		Title:            "test bead",
		Description:      "desc",
		Priority:         1,
		Labels:           []string{"foo"},
		EstimatedMinutes: 30,
	}
}

// TestVerifiedCreate_AcknowledgedButNotLanded reproduces ce-ctsi: the create
// command acknowledges the write (prints a bead with an ID, exit 0) but the
// row never lands in the store the ce database reads. VerifiedCreate must
// surface this as a hard error, never as success.
func TestVerifiedCreate_AcknowledgedButNotLanded(t *testing.T) {
	calls := []fakeCall{
		{stdout: createdJSON},     // bd create: acknowledged
		{stdout: showMissingJSON}, // bd show: row is NOT in the store
	}
	s := &Store{DBPath: "/tmp/db/.beads", Run: fakeRunner(t, &calls)}

	bead, err := s.VerifiedCreate(context.Background(), testReq())
	if err == nil {
		t.Fatalf("want hard error for acknowledged-but-not-landed write, got success: %+v", bead)
	}
	if !strings.Contains(err.Error(), "ce-abc") || !strings.Contains(err.Error(), "/tmp/db/.beads") {
		t.Errorf("error should identify the bead ID and store path, got: %v", err)
	}
}

func TestVerifiedCreate_Success(t *testing.T) {
	calls := []fakeCall{
		{stdout: createdJSON},
		{stdout: shownJSON},
	}
	s := &Store{DBPath: "/tmp/db/.beads", Run: fakeRunner(t, &calls)}

	bead, err := s.VerifiedCreate(context.Background(), testReq())
	if err != nil {
		t.Fatalf("VerifiedCreate: %v", err)
	}
	if bead.ID != "ce-abc" {
		t.Errorf("ID = %q, want ce-abc", bead.ID)
	}

	// Both the write and the verification read must target the SAME store
	// via an explicit --db — never bd's auto-discovery (the wrong-database
	// foot-gun that caused ce-ctsi).
	for i, c := range calls {
		joined := strings.Join(c.args, " ")
		if !strings.Contains(joined, "--db /tmp/db/.beads") {
			t.Errorf("call %d missing explicit --db: %v", i, c.args)
		}
	}
	if !strings.Contains(strings.Join(calls[1].args, " "), "show ce-abc") {
		t.Errorf("verification read should be `show ce-abc`, got: %v", calls[1].args)
	}
}

func TestVerifiedCreate_CreateFailureIsHardError(t *testing.T) {
	calls := []fakeCall{
		{stderr: "dolt: connection refused", err: errors.New("exit status 1")},
	}
	s := &Store{DBPath: "/tmp/db/.beads", Run: fakeRunner(t, &calls)}

	_, err := s.VerifiedCreate(context.Background(), testReq())
	if err == nil {
		t.Fatal("want error when bd create fails")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("stderr should be surfaced to the caller, got: %v", err)
	}
}

func TestVerifiedCreate_UnparseableAckIsHardError(t *testing.T) {
	calls := []fakeCall{
		{stdout: `{"schema_version": 1}`}, // no id
	}
	s := &Store{DBPath: "/tmp/db/.beads", Run: fakeRunner(t, &calls)}

	if _, err := s.VerifiedCreate(context.Background(), testReq()); err == nil {
		t.Fatal("want error when create output has no bead ID")
	}
}

// TestVerifiedCreate_UnconfiguredStoreIsHardError: the legacy tool silently
// defaulted to ~/.beads/issues.jsonl — a store nothing reads. An unconfigured
// store must be a hard error, never a fallback write.
func TestVerifiedCreate_UnconfiguredStoreIsHardError(t *testing.T) {
	calls := []fakeCall{}
	s := &Store{Run: fakeRunner(t, &calls)}

	_, err := s.VerifiedCreate(context.Background(), testReq())
	if !errors.Is(err, ErrStoreNotConfigured) {
		t.Fatalf("want ErrStoreNotConfigured, got: %v", err)
	}
	if len(calls) != 0 {
		t.Error("no bd invocation may happen without a configured store")
	}
}

func TestVerifiedCreate_ValidatesInput(t *testing.T) {
	s := &Store{DBPath: "/tmp/db/.beads", Run: fakeRunner(t, &[]fakeCall{})}
	ctx := context.Background()

	cases := []struct {
		name string
		mut  func(*CreateRequest)
	}{
		{"empty title", func(r *CreateRequest) { r.Title = "" }},
		{"empty description", func(r *CreateRequest) { r.Description = "" }},
		{"priority too high", func(r *CreateRequest) { r.Priority = 5 }},
		{"priority negative", func(r *CreateRequest) { r.Priority = -1 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := testReq()
			tc.mut(&req)
			if _, err := s.VerifiedCreate(ctx, req); err == nil {
				t.Error("want validation error")
			}
		})
	}
}

func TestShow_NotFound(t *testing.T) {
	calls := []fakeCall{{stdout: showMissingJSON}}
	s := &Store{DBPath: "/tmp/db/.beads", Run: fakeRunner(t, &calls)}

	_, err := s.Show(context.Background(), "ce-nope")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got: %v", err)
	}
}

func TestShow_Found(t *testing.T) {
	calls := []fakeCall{{stdout: shownJSON}}
	s := &Store{DBPath: "/tmp/db/.beads", Run: fakeRunner(t, &calls)}

	bead, err := s.Show(context.Background(), "ce-abc")
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if bead.ID != "ce-abc" {
		t.Errorf("ID = %q, want ce-abc", bead.ID)
	}
}

func TestList_PassesFlags(t *testing.T) {
	calls := []fakeCall{{stdout: `[]`}}
	s := &Store{DBPath: "/tmp/db/.beads", Run: fakeRunner(t, &calls)}

	if _, err := s.List(context.Background(), true); err != nil {
		t.Fatalf("List: %v", err)
	}
	joined := strings.Join(calls[0].args, " ")
	for _, want := range []string{"--db /tmp/db/.beads", "--json", "list", "--all", "-n 0"} {
		if !strings.Contains(joined, want) {
			t.Errorf("List args missing %q: %v", want, calls[0].args)
		}
	}
}
