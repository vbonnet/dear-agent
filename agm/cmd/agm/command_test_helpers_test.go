package main

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func executeFreshCommandForTest(t *testing.T, newCommand func() *cobra.Command, args []string) error {
	t.Helper()

	cmd := newCommand()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(append(make([]string, 0, len(args)), args...))
	t.Cleanup(func() {
		cmd.SetArgs(nil)
		cmd.SetOut(nil)
		cmd.SetErr(nil)
		cmd.SetContext(context.Background())
	})

	return cmd.Execute()
}

func restoreCommandFlagForTest(t *testing.T, cmd *cobra.Command, name string) {
	t.Helper()

	flag := cmd.Flags().Lookup(name)
	if flag == nil {
		t.Fatalf("command %q has no --%s flag", cmd.CommandPath(), name)
	}
	value := flag.Value.String()
	changed := flag.Changed
	t.Cleanup(func() {
		if err := flag.Value.Set(value); err != nil {
			t.Errorf("restore --%s: %v", name, err)
		}
		flag.Changed = changed
	})
}

func restoreCommandTreeFlagsForTest(t *testing.T, root *cobra.Command) {
	t.Helper()

	type flagState struct {
		flag    *pflag.Flag
		value   string
		slice   []string
		changed bool
	}
	seen := make(map[*pflag.Flag]bool)
	var states []flagState
	var visit func(*cobra.Command)
	visit = func(cmd *cobra.Command) {
		cmd.Flags().VisitAll(func(flag *pflag.Flag) {
			if seen[flag] {
				return
			}
			seen[flag] = true
			state := flagState{flag: flag, value: flag.Value.String(), changed: flag.Changed}
			if sliceValue, ok := flag.Value.(pflag.SliceValue); ok {
				state.slice = append([]string(nil), sliceValue.GetSlice()...)
			}
			states = append(states, state)
		})
		for _, child := range cmd.Commands() {
			visit(child)
		}
	}
	visit(root)

	t.Cleanup(func() {
		for i := len(states) - 1; i >= 0; i-- {
			state := states[i]
			var err error
			if sliceValue, ok := state.flag.Value.(pflag.SliceValue); ok {
				err = sliceValue.Replace(state.slice)
			} else {
				err = state.flag.Value.Set(state.value)
			}
			if err != nil {
				t.Errorf("restore --%s: %v", state.flag.Name, err)
			}
			state.flag.Changed = state.changed
		}
	})
}
