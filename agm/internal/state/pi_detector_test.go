package state

import (
	"testing"
	"time"
)

func TestPiManagedStateDrivesReadinessAndDelivery(t *testing.T) {
	detector := NewDetector()
	for _, test := range []struct {
		name     string
		content  string
		state    State
		delivery CanReceive
	}{
		{name: "ready", content: "Pi footer\nAGM plan/ready", state: StateReady, delivery: CanReceiveYes},
		{name: "working", content: "AGM plan/ready\nAGM auto/working", state: StateThinking, delivery: CanReceiveQueue},
		{name: "unmanaged", content: "Pi footer unknown", state: StateUnknown, delivery: CanReceiveQueue},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := detector.DetectState(test.content, time.Now()).State; got != test.state {
				t.Fatalf("state = %s, want %s", got, test.state)
			}
			if got := detector.CheckCanReceive(test.content); got != test.delivery {
				t.Fatalf("delivery = %s, want %s", got, test.delivery)
			}
		})
	}
}

func TestPiManagedPermissionPromptBlocksDelivery(t *testing.T) {
	detector := NewDetector()
	content := "AGM permission required\nAllow bash?\nAGM default/ready"
	if got := detector.DetectState(content, time.Now()).State; got != StateBlockedPermission {
		t.Fatalf("state = %s, want %s", got, StateBlockedPermission)
	}
	if got := detector.CheckCanReceive(content); got != CanReceiveNo {
		t.Fatalf("delivery = %s, want %s", got, CanReceiveNo)
	}
}
