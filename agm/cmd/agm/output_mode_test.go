package main

import "testing"

// TestResolveOutputMode exercises the agent-mode precedence ladder:
//
//	--agent > --no-agent > AGM_AGENT=1 > non-TTY auto > human default.
func TestResolveOutputMode(t *testing.T) {
	tests := []struct {
		name         string
		forceAgent   bool
		forceNoAgent bool
		agentEnv     string // value for AGM_AGENT ("" means unset)
		tty          bool
		want         OutputMode
	}{
		{
			name:       "--agent forces agent even on a TTY",
			forceAgent: true,
			tty:        true,
			want:       ModeAgent,
		},
		{
			name:       "--agent wins over --no-agent",
			forceAgent: true,
			// both set: --agent has higher precedence
			forceNoAgent: true,
			tty:          true,
			want:         ModeAgent,
		},
		{
			name:         "--no-agent forces human even when non-TTY",
			forceNoAgent: true,
			tty:          false,
			want:         ModeHuman,
		},
		{
			name:         "--no-agent overrides AGM_AGENT=1",
			forceNoAgent: true,
			agentEnv:     "1",
			tty:          true,
			want:         ModeHuman,
		},
		{
			name:     "AGM_AGENT=1 forces agent on a TTY",
			agentEnv: "1",
			tty:      true,
			want:     ModeAgent,
		},
		{
			name:     "AGM_AGENT other-than-1 does not force agent",
			agentEnv: "true",
			tty:      true,
			want:     ModeHuman,
		},
		{
			name: "non-TTY auto-detects agent mode",
			tty:  false,
			want: ModeAgent,
		},
		{
			name: "TTY with no overrides is human",
			tty:  true,
			want: ModeHuman,
		},
	}

	oldTTY := stdoutIsTTY
	defer func() { stdoutIsTTY = oldTTY }()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdoutIsTTY = func() bool { return tt.tty }
			t.Setenv("AGM_AGENT", tt.agentEnv)

			got := resolveOutputMode(tt.forceAgent, tt.forceNoAgent)
			if got != tt.want {
				t.Errorf("resolveOutputMode(forceAgent=%v, forceNoAgent=%v, AGM_AGENT=%q, tty=%v) = %v, want %v",
					tt.forceAgent, tt.forceNoAgent, tt.agentEnv, tt.tty, got, tt.want)
			}
		})
	}
}
