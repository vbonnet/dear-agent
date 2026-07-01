package main

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/vbonnet/dear-agent/pkg/vroom/supervisor"
)

func TestVROOMAGMDispatchArgsParseAgainstSessionNewFlags(t *testing.T) {
	restoreFlags := preserveFlagSet(t, newCmd.Flags())
	defer restoreFlags()

	args := supervisor.AGMDispatchArgs("ce-93lw.4", "sonnet-200k", "worker")
	if len(args) < 2 || args[0] != "session" || args[1] != "new" {
		t.Fatalf("expected agm session new args, got %v", args)
	}

	detachedFlag := newCmd.Flags().Lookup("detached")
	if detachedFlag == nil {
		t.Fatal("agm session new does not register --detached")
	}
	if obsolete := newCmd.Flags().Lookup("detach"); obsolete != nil {
		t.Fatal("agm session new unexpectedly registers obsolete --detach alias")
	}

	if err := newCmd.Flags().Parse(args[2:]); err != nil {
		t.Fatalf("VROOM dispatch args do not parse against agm session new flags: %v\nargs: %v", err, args)
	}
	if got, err := newCmd.Flags().GetBool("detached"); err != nil || !got {
		t.Fatalf("parsed --detached = %v, %v; want true", got, err)
	}
}

func preserveFlagSet(t *testing.T, flags *pflag.FlagSet) func() {
	t.Helper()

	type flagState struct {
		value   string
		changed bool
	}
	states := make(map[string]flagState)
	flags.VisitAll(func(flag *pflag.Flag) {
		states[flag.Name] = flagState{
			value:   flag.Value.String(),
			changed: flag.Changed,
		}
	})

	return func() {
		flags.VisitAll(func(flag *pflag.Flag) {
			state := states[flag.Name]
			if err := flag.Value.Set(state.value); err != nil {
				t.Errorf("restore flag %s: %v", flag.Name, err)
			}
			flag.Changed = state.changed
		})
	}
}
