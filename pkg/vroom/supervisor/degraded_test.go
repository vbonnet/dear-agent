package supervisor

import "testing"

func TestModeForAlivePeers(t *testing.T) {
	cases := []struct {
		alive int
		want  MeshMode
	}{
		{2, ModeNormal},
		{3, ModeNormal}, // clamp
		{1, ModeDegraded},
		{0, ModeEmergency},
		{-1, ModeEmergency},
	}
	for _, c := range cases {
		if got := ModeForAlivePeers(c.alive); got != c.want {
			t.Errorf("ModeForAlivePeers(%d) = %v, want %v", c.alive, got, c.want)
		}
	}
}

func TestMeshModeString(t *testing.T) {
	cases := map[MeshMode]string{
		ModeNormal:    "3/3 normal",
		ModeDegraded:  "2/3 degraded",
		ModeEmergency: "1/3 emergency",
	}
	for m, want := range cases {
		if got := m.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", int(m), got, want)
		}
	}
}

func TestCapabilitiesFor_Normal(t *testing.T) {
	// With roadmap authority (the Meta-O itself, or an assumed holder).
	c := CapabilitiesFor(ModeNormal, true)
	if !c.AddRoadmapItems {
		t.Error("normal+authority: want AddRoadmapItems true")
	}
	if !c.SpawnWorkers || c.SpawnRateLimit != 1.0 {
		t.Errorf("normal: want full spawn, got SpawnWorkers=%v rate=%v", c.SpawnWorkers, c.SpawnRateLimit)
	}
	if c.ElevatedMonitoring || c.SelfTerminate || c.NotifyHuman {
		t.Error("normal: want no elevated monitoring / terminate / notify")
	}

	// Without authority (a non-Meta-O supervisor in normal mode cannot admit
	// roadmap work).
	if CapabilitiesFor(ModeNormal, false).AddRoadmapItems {
		t.Error("normal without authority: want AddRoadmapItems false")
	}
}

func TestCapabilitiesFor_Degraded(t *testing.T) {
	c := CapabilitiesFor(ModeDegraded, true)
	if !c.AddRoadmapItems {
		t.Error("degraded+authority: want AddRoadmapItems true (retro R.5 delegation)")
	}
	if !c.SpawnWorkers {
		t.Error("degraded: want SpawnWorkers true")
	}
	if c.SpawnRateLimit != 0.5 {
		t.Errorf("degraded: want reduced spawn rate 0.5, got %v", c.SpawnRateLimit)
	}
	if !c.ElevatedMonitoring {
		t.Error("degraded: want ElevatedMonitoring true")
	}
	if c.SelfTerminate || c.NotifyHuman {
		t.Error("degraded: want no terminate / notify")
	}
	if CapabilitiesFor(ModeDegraded, false).AddRoadmapItems {
		t.Error("degraded without authority: want AddRoadmapItems false")
	}
}

func TestCapabilitiesFor_Emergency(t *testing.T) {
	// Authority flag is irrelevant in emergency mode — no new work ever.
	for _, auth := range []bool{true, false} {
		c := CapabilitiesFor(ModeEmergency, auth)
		if c.AddRoadmapItems {
			t.Errorf("emergency(auth=%v): want AddRoadmapItems false", auth)
		}
		if c.SpawnWorkers || c.SpawnRateLimit != 0 {
			t.Errorf("emergency(auth=%v): want no spawn", auth)
		}
		if !c.SelfTerminate {
			t.Errorf("emergency(auth=%v): want SelfTerminate true (retro R.3/R.4)", auth)
		}
		if !c.NotifyHuman {
			t.Errorf("emergency(auth=%v): want NotifyHuman true", auth)
		}
		if !c.ElevatedMonitoring {
			t.Errorf("emergency(auth=%v): want ElevatedMonitoring true", auth)
		}
	}
}
