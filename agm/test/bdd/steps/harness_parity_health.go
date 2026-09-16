package steps

import (
	"os"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
)

// checkHarnessHealthForScenario preserves the BDD glue's ambient-HOME setup
// without retaining that compatibility seam in the production agent package.
func checkHarnessHealthForScenario(harness string) agent.HarnessHealth {
	home, _ := os.UserHomeDir()
	return agent.CheckHarnessHealthAtHome(harness, home)
}
