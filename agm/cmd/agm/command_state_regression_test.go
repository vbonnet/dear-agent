package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCobraCommandValidationIsOrderIndependent(t *testing.T) {
	tests := []struct {
		name       string
		newCommand func() *cobra.Command
		args       []string
		wantError  string
	}{
		{
			name:       "invalid harness",
			newCommand: newInstallHarnessCommand,
			args:       []string{"invalid-harness"},
			wantError:  "unsupported harness",
		},
		{
			name:       "tag without session",
			newCommand: newSessionTagCommand,
			args:       []string{},
			wantError:  "accepts between 1 and 2 arg(s), received 0",
		},
		{
			name:       "tag without mutation",
			newCommand: newSessionTagCommand,
			args:       []string{"some-session"},
			wantError:  "provide a tag to add",
		},
	}

	orders := []struct {
		name    string
		indexes []int
	}{
		{name: "forward", indexes: []int{0, 1, 2}},
		{name: "reverse", indexes: []int{2, 1, 0}},
	}

	for _, order := range orders {
		t.Run(order.name, func(t *testing.T) {
			for repeat := range 3 {
				for _, index := range order.indexes {
					test := tests[index]
					err := executeFreshCommandForTest(t, test.newCommand, test.args)
					if err == nil || !strings.Contains(err.Error(), test.wantError) {
						t.Fatalf("iteration %d, %s: error = %v, want substring %q", repeat, test.name, err, test.wantError)
					}
				}
			}
		})
	}
}

func TestCobraCommandFactoriesIsolateFlagValues(t *testing.T) {
	tests := []struct {
		name       string
		newCommand func() *cobra.Command
		flag       string
		set        string
		want       string
	}{
		{name: "install JSON", newCommand: newInstallHarnessCommand, flag: "json", set: "false", want: "true"},
		{name: "tag removal", newCommand: newSessionTagCommand, flag: "remove", set: "role:worker", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			first := test.newCommand()
			second := test.newCommand()
			if err := first.Flags().Set(test.flag, test.set); err != nil {
				t.Fatal(err)
			}
			secondFlag := second.Flags().Lookup(test.flag)
			if got := secondFlag.Value.String(); got != test.want {
				t.Errorf("fresh --%s = %q, want default %q", test.flag, got, test.want)
			}
			if secondFlag.Changed {
				t.Errorf("fresh --%s should not be marked changed", test.flag)
			}
		})
	}
}

func TestRestoreCommandTreeFlagsForTestPreservesNilStringSlice(t *testing.T) {
	var fields []string
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().StringSliceVar(&fields, "fields", nil, "field mask")

	t.Run("mutate and restore", func(t *testing.T) {
		restoreCommandTreeFlagsForTest(t, cmd)
		if err := cmd.Flags().Set("fields", "id,name"); err != nil {
			t.Fatal(err)
		}
	})

	flag := cmd.Flags().Lookup("fields")
	if fields != nil {
		t.Errorf("restored fields = %#v, want nil", fields)
	}
	if flag.Changed {
		t.Error("restored --fields should not be marked changed")
	}
}

func TestRestoreCommandTreeFlagsForTestPreservesStringSliceParseState(t *testing.T) {
	defaults := []string{"worktrees", "sandboxes"}
	var targets []string
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().StringSliceVar(&targets, "targets", defaults, "cleanup targets")

	t.Run("mutate and restore", func(t *testing.T) {
		restoreCommandTreeFlagsForTest(t, cmd)
		if err := cmd.Flags().Set("targets", "sessions"); err != nil {
			t.Fatal(err)
		}
	})

	flag := cmd.Flags().Lookup("targets")
	if flag.Changed {
		t.Error("restored --targets should not be marked changed")
	}
	if err := cmd.Flags().Set("targets", "processes"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"processes"}; !slices.Equal(targets, want) {
		t.Errorf("first parse after restore = %v, want replacement %v", targets, want)
	}
}
