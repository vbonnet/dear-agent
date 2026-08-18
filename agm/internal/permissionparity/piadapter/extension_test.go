package piadapter

import (
	"strings"
	"testing"
)

func TestAuthorizationExtensionPublishesLaunchCorrelatedStatus(t *testing.T) {
	source := string(piAuthorizationExtension)
	for _, want := range []string{
		`process.env.AGM_PI_LAUNCH_ID`,
		`AGM ${mode}/${state}${launchID ? `,
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("embedded Pi extension omits launch readiness token %q", want)
		}
	}
}
