package supervisor

import "testing"

func TestTopology_Table(t *testing.T) {
	tests := []struct {
		role        Role
		id          string
		alias       string
		primaryFor  string
		tertiaryFor string
	}{
		{RoleMetaOrchestrator, IDMetaOrchestrator, AliasMetaOrchestrator, IDOrchestrator, IDOverseer},
		{RoleOrchestrator, IDOrchestrator, AliasOrchestrator, IDOverseer, IDMetaOrchestrator},
		{RoleOverseer, IDOverseer, AliasOverseer, IDMetaOrchestrator, IDOrchestrator},
	}

	members := AllMembers()
	if len(members) != len(tests) {
		t.Fatalf("AllMembers() returned %d members, want %d", len(members), len(tests))
	}
	for i, tt := range tests {
		member := members[i]
		if member.Role != tt.role || member.ID != tt.id || member.Alias != tt.alias ||
			member.PrimaryFor != tt.primaryFor || member.TertiaryFor != tt.tertiaryFor {
			t.Errorf("member[%d] = %+v, want role=%q id=%q alias=%q primary=%q tertiary=%q",
				i, member, tt.role, tt.id, tt.alias, tt.primaryFor, tt.tertiaryFor)
		}
		if member.PrimaryFor == member.ID || member.TertiaryFor == member.ID {
			t.Errorf("member %q contains itself in its peer graph", member.ID)
		}
		if member.PrimaryFor == member.TertiaryFor {
			t.Errorf("member %q has the same primary and tertiary peer", member.ID)
		}
	}
}

func TestTopology_LookupAliases(t *testing.T) {
	for _, member := range AllMembers() {
		for _, identity := range []string{member.ID, member.Alias, string(member.Role), "  " + member.Alias + "  "} {
			got, ok := Lookup(identity)
			if !ok {
				t.Errorf("Lookup(%q) did not resolve", identity)
				continue
			}
			if got != member {
				t.Errorf("Lookup(%q) = %+v, want %+v", identity, got, member)
			}
		}
	}
	if _, ok := Lookup("verifier"); ok {
		t.Error("Lookup resolved retired verifier role")
	}
}

func TestTopology_PeerRelationshipsResolve(t *testing.T) {
	for _, member := range AllMembers() {
		primary, err := member.PrimaryPeer()
		if err != nil {
			t.Fatalf("PrimaryPeer(%q): %v", member.ID, err)
		}
		tertiary, err := member.TertiaryPeer()
		if err != nil {
			t.Fatalf("TertiaryPeer(%q): %v", member.ID, err)
		}
		if primary.ID != member.PrimaryFor {
			t.Errorf("PrimaryPeer(%q) = %q, want %q", member.ID, primary.ID, member.PrimaryFor)
		}
		if tertiary.ID != member.TertiaryFor {
			t.Errorf("TertiaryPeer(%q) = %q, want %q", member.ID, tertiary.ID, member.TertiaryFor)
		}
	}
}

func TestTopology_AllMembersReturnsCopy(t *testing.T) {
	members := AllMembers()
	members[0].ID = "mutated"
	got, ok := MemberForRole(RoleMetaOrchestrator)
	if !ok {
		t.Fatal("MemberForRole(meta-orchestrator) did not resolve")
	}
	if got.ID != IDMetaOrchestrator {
		t.Errorf("topology mutated through AllMembers: ID = %q", got.ID)
	}
}
