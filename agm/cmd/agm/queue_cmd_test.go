package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestRunQueueListRejectsInvalidStatusBeforeOpeningQueue(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	cmd := &cobra.Command{}
	cmd.Flags().String("status", "", "")
	cmd.Flags().Int("limit", 20, "")
	require.NoError(t, cmd.Flags().Set("status", "waiting"))

	err := runQueueList(cmd, nil)
	require.EqualError(t, err, `invalid status filter: invalid queue state "waiting": must be queued, delivered, or failed`)

	_, statErr := os.Stat(filepath.Join(home, ".config", "agm"))
	require.ErrorIs(t, statErr, os.ErrNotExist)
}
