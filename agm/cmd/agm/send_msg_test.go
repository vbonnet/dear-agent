package main

import "testing"

func TestIsAutonomousRole(t *testing.T) {
	cases := []struct {
		tags []string
		want bool
	}{
		{[]string{"role:worker"}, true},
		{[]string{"role:orchestrator"}, true},
		{[]string{"role:overseer"}, true},
		{[]string{"role:meta-orchestrator"}, true},
		{[]string{"role:human"}, false},
		{[]string{}, false},
		{nil, false},
		{[]string{"other-tag", "role:worker"}, true},
		{[]string{"cap:web-search"}, false},
	}
	for _, c := range cases {
		got := isAutonomousRole(c.tags)
		if got != c.want {
			t.Errorf("isAutonomousRole(%v) = %v, want %v", c.tags, got, c.want)
		}
	}
}
