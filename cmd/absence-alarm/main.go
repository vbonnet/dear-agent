// Command absence-alarm alarms on the ABSENCE of expected positive events.
//
// This is the domain slice of a two-PR stack: it ships the pulse model
// (declarative expected positive events with maximum silence windows, EARS
// AA-01..AA-06, AA-19), the alarm state machine (transition + backoff
// notification decisions, expiring snoozes, escalation journal, heartbeat;
// AA-09..AA-18) and their tests. The CLI wiring, launchd deployment, and
// default pulse set land in the slice stacked on top.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "absence-alarm: the CLI lands in the next slice of this stack")
	os.Exit(2)
}
