package supervisor

import (
	"context"
	"testing"
	"time"
)

// claimStoreContract exercises the ClaimStore behaviour both implementations
// must satisfy.
func claimStoreContract(t *testing.T, store ClaimStore) {
	t.Helper()
	ctx := context.Background()
	base := time.Unix(7000, 0).UTC()

	// Empty store.
	if _, ok, err := store.Get(ctx, RoleMetaOrchestrator); err != nil || ok {
		t.Fatalf("empty Get: ok=%v err=%v", ok, err)
	}

	// Put two competing claims; earlier one wins.
	if err := store.Put(ctx, Claim{Target: RoleMetaOrchestrator, By: RoleOverseer, Reason: "second", Timestamp: base.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ctx, Claim{Target: RoleMetaOrchestrator, By: RoleOrchestrator, Reason: "first", Timestamp: base}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.Get(ctx, RoleMetaOrchestrator)
	if err != nil || !ok {
		t.Fatalf("Get after puts: ok=%v err=%v", ok, err)
	}
	if got.By != RoleOrchestrator {
		t.Errorf("winner = %s, want orchestrator (earlier claim)", got.By)
	}
	if got.Reason != "first" {
		t.Errorf("winner reason = %q, want first", got.Reason)
	}

	// Delete the winner → the other claim now wins.
	if err := store.Delete(ctx, RoleMetaOrchestrator, RoleOrchestrator); err != nil {
		t.Fatal(err)
	}
	got, ok, err = store.Get(ctx, RoleMetaOrchestrator)
	if err != nil || !ok {
		t.Fatalf("Get after delete: ok=%v err=%v", ok, err)
	}
	if got.By != RoleOverseer {
		t.Errorf("after deleting orchestrator, winner = %s, want overseer", got.By)
	}

	// Delete remaining → empty again.
	if err := store.Delete(ctx, RoleMetaOrchestrator, RoleOverseer); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := store.Get(ctx, RoleMetaOrchestrator); ok {
		t.Error("store should be empty after deleting all claims")
	}

	// Deleting a non-existent claim is not an error.
	if err := store.Delete(ctx, RoleOverseer, RoleMetaOrchestrator); err != nil {
		t.Errorf("delete non-existent: %v", err)
	}
}

func TestInMemoryClaimStore_Contract(t *testing.T) {
	claimStoreContract(t, NewInMemoryClaimStore())
}

func TestFileClaimStore_Contract(t *testing.T) {
	claimStoreContract(t, NewFileClaimStore(t.TempDir()))
}

func TestFileClaimStore_PersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()
	s1 := NewFileClaimStore(dir)
	c := Claim{Target: RoleMetaOrchestrator, By: RoleOverseer, Reason: "dark", Timestamp: time.Unix(8000, 0).UTC()}
	if err := s1.Put(ctx, c); err != nil {
		t.Fatal(err)
	}
	// A fresh store pointed at the same dir sees the claim (cross-session).
	s2 := NewFileClaimStore(dir)
	got, ok, err := s2.Get(ctx, RoleMetaOrchestrator)
	if err != nil || !ok {
		t.Fatalf("cross-instance Get: ok=%v err=%v", ok, err)
	}
	if got.By != RoleOverseer || !got.Timestamp.Equal(c.Timestamp) {
		t.Errorf("round-trip mismatch: got %+v want %+v", got, c)
	}
}

func TestClaimStore_RejectsInvalidClaim(t *testing.T) {
	ctx := context.Background()
	for _, store := range []ClaimStore{NewInMemoryClaimStore(), NewFileClaimStore(t.TempDir())} {
		if err := store.Put(ctx, Claim{Target: "bogus", By: RoleOverseer}); err == nil {
			t.Errorf("%T: want error on invalid target", store)
		}
	}
}
