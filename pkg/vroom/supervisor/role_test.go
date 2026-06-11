package supervisor

import "testing"

func TestRole_Valid(t *testing.T) {
	for _, r := range AllRoles() {
		if !r.Valid() {
			t.Errorf("AllRoles() returned %q which reports !Valid()", r)
		}
	}
	if Role("verifier").Valid() {
		t.Error("retired Verifier role still reports Valid() — supersession leak")
	}
	if Role("").Valid() {
		t.Error("empty role reports Valid()")
	}
}

func TestRole_AllRoles_IsThree(t *testing.T) {
	// The "three-supervisor" doctrine is structural. If this count changes,
	// CONTEXT.md and ADR-002 must change first.
	if got := len(AllRoles()); got != 3 {
		t.Fatalf("AllRoles() returned %d roles, want 3", got)
	}
}

func TestRole_Peers_TableMatchesContextMD(t *testing.T) {
	// Source of truth: CONTEXT.md § "The three supervisors". This test
	// encodes the table; if CONTEXT.md changes, this test must change with it.
	cases := []struct {
		role         Role
		wantVerifies Role
		wantUnsticks Role
	}{
		// Meta-O is Secondary FOR Orchestrator, Tertiary FOR Overseer.
		// Inverse: Meta-O is verified by Overseer, unstuck by Orchestrator —
		// but Peers() returns whom *Meta-O watches*, which is the inverse:
		// Meta-O verifies Orchestrator (Meta-O is Orchestrator's Secondary)
		// Meta-O unsticks  Overseer     (Meta-O is Overseer's Tertiary)
		{RoleMetaOrchestrator, RoleOrchestrator, RoleOverseer},
		{RoleOrchestrator, RoleOverseer, RoleMetaOrchestrator},
		{RoleOverseer, RoleMetaOrchestrator, RoleOrchestrator},
	}
	for _, tc := range cases {
		t.Run(string(tc.role), func(t *testing.T) {
			v, u, err := tc.role.Peers()
			if err != nil {
				t.Fatalf("Peers() err = %v", err)
			}
			if v != tc.wantVerifies {
				t.Errorf("verifies = %q, want %q", v, tc.wantVerifies)
			}
			if u != tc.wantUnsticks {
				t.Errorf("unsticks = %q, want %q", u, tc.wantUnsticks)
			}
		})
	}
}

func TestRole_Peers_CoversBothOtherSupervisors(t *testing.T) {
	// Invariant from CONTEXT.md: "the first action of every loop iteration
	// is to run the supervisor-check skill against the *other two*
	// supervisors". Peers() must return both other supervisors and never
	// the role itself.
	for _, r := range AllRoles() {
		v, u, err := r.Peers()
		if err != nil {
			t.Fatalf("Peers(%q) err = %v", r, err)
		}
		if v == r || u == r {
			t.Errorf("Peers(%q) = (%q,%q) includes self — supervisor would check itself", r, v, u)
		}
		if v == u {
			t.Errorf("Peers(%q) = (%q,%q) duplicates a peer — third supervisor unwatched", r, v, u)
		}
		// The union of {r, v, u} must equal AllRoles().
		seen := map[Role]bool{r: true, v: true, u: true}
		for _, want := range AllRoles() {
			if !seen[want] {
				t.Errorf("Peers(%q): role %q is neither self nor in peers — mesh has a gap", r, want)
			}
		}
	}
}

func TestRole_Peers_CycleClosesOnEachRelationship(t *testing.T) {
	// Cyclic invariant: for every (a, b) where a verifies b, the inverse
	// must also hold per the CONTEXT.md table — b is verified-by a. Same
	// for unsticks. If this breaks, the mesh has a one-way edge.
	verifies := map[Role]Role{} // a verifies b
	unsticks := map[Role]Role{}
	for _, r := range AllRoles() {
		v, u, err := r.Peers()
		if err != nil {
			t.Fatalf("Peers(%q) err = %v", r, err)
		}
		verifies[r] = v
		unsticks[r] = u
	}
	// Each role must appear exactly once on the right side of `verifies`
	// (everyone is verified by exactly one peer). Same for unsticks.
	verifiedBy := map[Role]int{}
	unstuckBy := map[Role]int{}
	for _, b := range verifies {
		verifiedBy[b]++
	}
	for _, b := range unsticks {
		unstuckBy[b]++
	}
	for _, r := range AllRoles() {
		if verifiedBy[r] != 1 {
			t.Errorf("role %q is verified by %d peers, want 1", r, verifiedBy[r])
		}
		if unstuckBy[r] != 1 {
			t.Errorf("role %q is unstuck by %d peers, want 1", r, unstuckBy[r])
		}
	}
}

func TestRole_Peers_InvalidRole(t *testing.T) {
	_, _, err := Role("verifier").Peers()
	if err == nil {
		t.Error("Peers() returned nil error for retired Verifier role")
	}
}

func TestRole_PeerList(t *testing.T) {
	for _, r := range AllRoles() {
		peers, err := r.PeerList()
		if err != nil {
			t.Fatalf("PeerList(%q) err = %v", r, err)
		}
		if len(peers) != 2 {
			t.Errorf("PeerList(%q) returned %d peers, want 2", r, len(peers))
		}
	}
}
