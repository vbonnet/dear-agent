package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/agm/internal/testcontext"
)

func TestTestEnvironmentCreateListDestroySharesOwnedRoot(t *testing.T) {
	name := "cli-" + testcontext.New().RunID
	tc, err := testcontext.NewNamed(name)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tc.Cleanup() })

	originalName, err := testEnvCreateCmd.Flags().GetString("name")
	require.NoError(t, err)
	originalAuthMode, err := testEnvCreateCmd.Flags().GetString("auth-mode")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, testEnvCreateCmd.Flags().Set("name", originalName))
		require.NoError(t, testEnvCreateCmd.Flags().Set("auth-mode", originalAuthMode))
	})
	require.NoError(t, testEnvCreateCmd.Flags().Set("name", name))
	require.NoError(t, testEnvCreateCmd.Flags().Set("auth-mode", "none"))

	createOutput := captureStdout(t, func() {
		require.NoError(t, testEnvCreateCmd.RunE(testEnvCreateCmd, nil))
	})
	require.Contains(t, createOutput, "AGM_TEST_ENV="+name)
	require.Contains(t, createOutput, tc.BaseDir)

	listOutput := captureStdout(t, func() {
		require.NoError(t, testEnvListCmd.RunE(testEnvListCmd, nil))
	})
	require.Contains(t, listOutput, name)
	require.Contains(t, listOutput, tc.BaseDir)

	destroyOutput := captureStdout(t, func() {
		require.NoError(t, testEnvDestroyCmd.RunE(testEnvDestroyCmd, []string{name}))
	})
	require.Contains(t, destroyOutput, "Destroyed test environment: "+name)
	_, err = os.Stat(tc.BaseDir)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestTestEnvironmentCommandsRejectTraversalNames(t *testing.T) {
	outsideRoot, err := os.MkdirTemp("/tmp", "agm-unowned-") //nolint:usetesting // Must be a direct sibling of the fixed test-environment root.
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(outsideRoot)) })
	sentinel := filepath.Join(outsideRoot, "preserve")
	require.NoError(t, os.WriteFile(sentinel, []byte("owned by test"), 0600))

	// Without validation, prefixing this name produces
	// /tmp/agm-test-x/../<outsideRoot>, which cleans the sibling directory.
	unsafeName := strings.Join([]string{"x", "..", filepath.Base(outsideRoot)}, string(os.PathSeparator))

	originalName, err := testEnvCreateCmd.Flags().GetString("name")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, testEnvCreateCmd.Flags().Set("name", originalName))
	})
	require.NoError(t, testEnvCreateCmd.Flags().Set("name", unsafeName))

	err = testEnvCreateCmd.RunE(testEnvCreateCmd, nil)
	require.ErrorContains(t, err, "must not contain path separators")

	err = testEnvDestroyCmd.RunE(testEnvDestroyCmd, []string{unsafeName})
	require.ErrorContains(t, err, "must not contain path separators")
	_, err = os.Stat(sentinel)
	require.NoError(t, err, "rejected destroy mutated a sibling outside the owned environment")
}

func TestTestEnvironmentDestroyRemovesRetiredRoot(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	retiredRoot := filepath.Clean(os.TempDir())
	require.NotEqual(t, "/tmp", retiredRoot)
	name := testcontext.New().RunID + strings.Repeat("l", 64)
	retiredBase := filepath.Join(retiredRoot, "agm-test-"+name)
	retiredSocket := filepath.Join(retiredRoot, "agm-test-"+name+".sock")
	require.NoError(t, os.MkdirAll(filepath.Join(retiredBase, "home"), 0700))
	require.NoError(t, os.WriteFile(retiredSocket, []byte("retired socket"), 0600))
	t.Cleanup(func() {
		require.NoError(t, os.RemoveAll(retiredBase))
		if err := os.Remove(retiredSocket); err != nil && !os.IsNotExist(err) {
			t.Errorf("remove retired socket: %v", err)
		}
	})

	destroyOutput := captureStdout(t, func() {
		require.NoError(t, testEnvDestroyCmd.RunE(testEnvDestroyCmd, []string{name}))
	})
	require.Contains(t, destroyOutput, "Destroyed test environment: "+name)
	for _, removed := range []string{retiredBase, retiredSocket} {
		_, err := os.Lstat(removed)
		require.ErrorIs(t, err, os.ErrNotExist)
	}
}
