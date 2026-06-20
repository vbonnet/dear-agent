package supervisor

import "testing"

func TestSecondaryFor(t *testing.T) {
	cases := map[Role]Role{
		RoleMetaOrchestrator: RoleOverseer,
		RoleOrchestrator:     RoleMetaOrchestrator,
		RoleOverseer:         RoleOrchestrator,
	}
	for target, want := range cases {
		got, err := SecondaryFor(target)
		if err != nil {
			t.Fatalf("SecondaryFor(%s): %v", target, err)
		}
		if got != want {
			t.Errorf("SecondaryFor(%s) = %s, want %s", target, got, want)
		}
	}
}

func TestTertiaryFor(t *testing.T) {
	cases := map[Role]Role{
		RoleMetaOrchestrator: RoleOrchestrator,
		RoleOrchestrator:     RoleOverseer,
		RoleOverseer:         RoleMetaOrchestrator,
	}
	for target, want := range cases {
		got, err := TertiaryFor(target)
		if err != nil {
			t.Fatalf("TertiaryFor(%s): %v", target, err)
		}
		if got != want {
			t.Errorf("TertiaryFor(%s) = %s, want %s", target, got, want)
		}
	}
}

// TestInverseConsistency verifies SecondaryFor/TertiaryFor are the exact
// inverse of Peers: SecondaryFor(target) verifies target, TertiaryFor(target)
// unsticks target.
func TestInverseConsistency(t *testing.T) {
	for _, target := range AllRoles() {
		sec, err := SecondaryFor(target)
		if err != nil {
			t.Fatal(err)
		}
		v, _, err := sec.Peers()
		if err != nil {
			t.Fatal(err)
		}
		if v != target {
			t.Errorf("SecondaryFor(%s)=%s but %s verifies %s, not %s", target, sec, sec, v, target)
		}

		ter, err := TertiaryFor(target)
		if err != nil {
			t.Fatal(err)
		}
		_, u, err := ter.Peers()
		if err != nil {
			t.Fatal(err)
		}
		if u != target {
			t.Errorf("TertiaryFor(%s)=%s but %s unsticks %s, not %s", target, ter, ter, u, target)
		}

		// Secondary and Tertiary for a target must be distinct.
		if sec == ter {
			t.Errorf("SecondaryFor(%s)==TertiaryFor(%s)==%s", target, target, sec)
		}
	}
}

func TestSecondaryTertiaryFor_InvalidRole(t *testing.T) {
	if _, err := SecondaryFor("bogus"); err == nil {
		t.Error("SecondaryFor(bogus): want error")
	}
	if _, err := TertiaryFor("bogus"); err == nil {
		t.Error("TertiaryFor(bogus): want error")
	}
}
