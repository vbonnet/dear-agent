package bus

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// DiscordPortalConfig is the on-disk config for the multi-bot portal,
// loaded from ~/.agm/discord-agents.yaml (chmod 600, gitignored). Secrets
// live here, never in the repo — same posture as DISCORD_BOT_TOKEN.
//
// Example:
//
//	channel: "1475530285490634865"
//	guild:   "1475530284911956030"      # optional defense-in-depth
//	author_allowlist: ["<your-user-id>"] # optional; empty = any human
//	agents:
//	  - name: claude
//	    token: "BOT_TOKEN_FOR_CLAUDE_APP"
//	    bus_session: "claude-portal"
//	  - name: codex
//	    token: "BOT_TOKEN_FOR_CODEX_APP"
//	    bus_session: "codex-portal"
type DiscordPortalConfig struct {
	Channel         string   `yaml:"channel"`
	Guild           string   `yaml:"guild"`
	AuthorAllowlist []string `yaml:"author_allowlist"`
	Agents          []struct {
		Name       string `yaml:"name"`
		Token      string `yaml:"token"`
		BusSession string `yaml:"bus_session"`
	} `yaml:"agents"`
}

// LoadDiscordAgentsConfig reads and validates the portal config. It returns a
// permission warning (non-fatal, via the returned bool) if the file is
// readable by group/other, since it holds bot tokens.
func LoadDiscordAgentsConfig(path string) (cfg *DiscordPortalConfig, loosePerms bool, err error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, false, fmt.Errorf("discord-multibot: stat config %s: %w", path, err)
	}
	loosePerms = fi.Mode().Perm()&0o077 != 0

	b, err := os.ReadFile(path)
	if err != nil {
		return nil, loosePerms, fmt.Errorf("discord-multibot: read config %s: %w", path, err)
	}
	var c DiscordPortalConfig
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, loosePerms, fmt.Errorf("discord-multibot: parse config %s: %w", path, err)
	}
	if c.Channel == "" {
		return nil, loosePerms, fmt.Errorf("discord-multibot: config %s: 'channel' is required", path)
	}
	if len(c.Agents) == 0 {
		return nil, loosePerms, fmt.Errorf("discord-multibot: config %s: no agents defined", path)
	}
	seen := make(map[string]bool, len(c.Agents))
	for i, a := range c.Agents {
		switch {
		case a.Name == "":
			return nil, loosePerms, fmt.Errorf("discord-multibot: agent[%d]: 'name' is required", i)
		case a.Token == "":
			return nil, loosePerms, fmt.Errorf("discord-multibot: agent %q: 'token' is required", a.Name)
		case a.BusSession == "":
			return nil, loosePerms, fmt.Errorf("discord-multibot: agent %q: 'bus_session' is required", a.Name)
		case seen[a.Name]:
			return nil, loosePerms, fmt.Errorf("discord-multibot: duplicate agent name %q", a.Name)
		}
		seen[a.Name] = true
	}
	return &c, loosePerms, nil
}

// ToAgents converts the parsed config into adapter DiscordAgent values.
func (c *DiscordPortalConfig) ToAgents() []*DiscordAgent {
	out := make([]*DiscordAgent, 0, len(c.Agents))
	for _, a := range c.Agents {
		out = append(out, &DiscordAgent{Name: a.Name, Token: a.Token, BusSession: a.BusSession})
	}
	return out
}
