package main

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func executeFreshCommandForTest(t *testing.T, newCommand func() *cobra.Command, args []string) error {
	t.Helper()

	cmd := newCommand()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs(slices.Clone(args))
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
		clone   pflag.Value
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
				state.slice = slices.Clone(sliceValue.GetSlice())
				state.clone = cloneFlagValueForTest(t, flag.Value)
			}
			states = append(states, state)
		})
		for _, child := range cmd.Commands() {
			visit(child)
		}
	}
	visit(root)

	t.Cleanup(func() {
		for _, state := range slices.Backward(states) {
			var err error
			if sliceValue, ok := state.flag.Value.(pflag.SliceValue); ok {
				err = sliceValue.Replace(state.slice)
				state.flag.Value = state.clone
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

// cloneFlagValueForTest preserves private parse state held by pflag slice
// values. Restoring only Flag.Changed is insufficient: a slice value also
// remembers whether its first Set should replace or append to defaults.
func cloneFlagValueForTest(t *testing.T, value pflag.Value) pflag.Value {
	t.Helper()

	original := reflect.ValueOf(value)
	if original.Kind() != reflect.Pointer || original.IsNil() {
		t.Fatalf("clone pflag value %T: expected non-nil pointer", value)
	}
	clone := reflect.New(original.Elem().Type())
	clone.Elem().Set(original.Elem())
	cloned, ok := clone.Interface().(pflag.Value)
	if !ok {
		t.Fatalf("clone pflag value %T: clone does not implement pflag.Value", value)
	}
	return cloned
}
