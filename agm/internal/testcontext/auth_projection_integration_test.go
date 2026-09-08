package testcontext_test

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vbonnet/dear-agent/agm/internal/agent"
	"github.com/vbonnet/dear-agent/agm/internal/testcontext"
)

func TestForwardAuthRoutesSelectedHomeMutations(t *testing.T) {
	hostHome := t.TempDir()
	hostCredential := filepath.Join(hostHome, ".codex", "auth.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(hostCredential), 0700))
	require.NoError(t, os.WriteFile(hostCredential, []byte("synthetic-auth"), 0600))
	hostConfig := filepath.Join(hostHome, ".codex", "config.toml")
	require.NoError(t, os.WriteFile(hostConfig, []byte("host-trust-sentinel"), 0600))
	hostOnboarding := filepath.Join(hostHome, ".claude.json")
	require.NoError(t, os.WriteFile(hostOnboarding, []byte("host-onboarding-sentinel"), 0600))

	tc := testcontext.New()
	require.NoError(t, tc.EnsureDirs())
	t.Cleanup(func() {
		require.NoError(t, tc.Cleanup())
	})
	require.NoError(t, tc.ForwardAuth(hostHome, testcontext.AuthModeInherit))

	t.Setenv("HOME", tc.HomeDir)
	t.Setenv("CODEX_HOME", "")
	workDir := t.TempDir()
	require.NoError(t, agent.EnsureCodexWorkdirTrusted(workDir))
	var entropy [16]byte
	_, err := rand.Read(entropy[:])
	require.NoError(t, err)
	token := hex.EncodeToString(entropy[:])
	markerName := ".agm-synthetic-onboarding-" + token
	require.NoError(t, os.WriteFile(filepath.Join(tc.HomeDir, markerName), []byte(token), 0600))
	onboarding := exec.Command(os.Args[0], "-test.run=^TestSyntheticProviderOnboardingProcess$")
	onboarding.Env = replaceTestEnvironment(os.Environ(), [][2]string{
		{"HOME", tc.HomeDir},
		{"CODEX_HOME", ""},
		{"AGM_TEST_SYNTHETIC_ONBOARDING_TOKEN", token},
		{"AGM_TEST_SYNTHETIC_ONBOARDING_HOME", tc.HomeDir},
		{"AGM_TEST_SYNTHETIC_ONBOARDING_MARKER", markerName},
	})
	output, err := onboarding.CombinedOutput()
	require.NoError(t, err, string(output))

	selectedConfig, err := os.ReadFile(filepath.Join(tc.HomeDir, ".codex", "config.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(selectedConfig), strings.ReplaceAll(workDir, `\`, `\\`))
	selectedOnboarding, err := os.ReadFile(filepath.Join(tc.HomeDir, ".claude.json"))
	require.NoError(t, err)
	assert.Equal(t, "selected-onboarding", string(selectedOnboarding))

	hostConfigData, err := os.ReadFile(hostConfig)
	require.NoError(t, err)
	assert.Equal(t, "host-trust-sentinel", string(hostConfigData))
	hostOnboardingData, err := os.ReadFile(hostOnboarding)
	require.NoError(t, err)
	assert.Equal(t, "host-onboarding-sentinel", string(hostOnboardingData))
}

func TestSyntheticProviderOnboardingProcess(t *testing.T) {
	token := os.Getenv("AGM_TEST_SYNTHETIC_ONBOARDING_TOKEN")
	if token == "" {
		t.Skip("helper process")
	}
	expectedHome := os.Getenv("AGM_TEST_SYNTHETIC_ONBOARDING_HOME")
	markerName := os.Getenv("AGM_TEST_SYNTHETIC_ONBOARDING_MARKER")
	require.NotEmpty(t, expectedHome)
	require.True(t, filepath.IsAbs(expectedHome))
	require.Equal(t, filepath.Clean(expectedHome), expectedHome)
	require.Equal(t, filepath.Base(markerName), markerName)
	require.True(t, strings.HasPrefix(markerName, ".agm-synthetic-onboarding-"))

	selectedHome, err := os.UserHomeDir()
	require.NoError(t, err)
	require.Equal(t, expectedHome, selectedHome)
	homeInfo, err := os.Lstat(selectedHome)
	require.NoError(t, err)
	require.True(t, homeInfo.IsDir())
	require.Equal(t, os.FileMode(0700), homeInfo.Mode().Perm())
	homeStat, ok := homeInfo.Sys().(*syscall.Stat_t)
	require.True(t, ok)
	// #nosec G115 -- effective Unix user IDs are non-negative and Stat_t.Uid is uint32.
	require.Equal(t, uint32(os.Geteuid()), homeStat.Uid)
	markerPath := filepath.Join(selectedHome, markerName)
	markerInfo, err := os.Lstat(markerPath)
	require.NoError(t, err)
	require.True(t, markerInfo.Mode().IsRegular())
	require.Equal(t, os.FileMode(0600), markerInfo.Mode().Perm())
	markerStat, ok := markerInfo.Sys().(*syscall.Stat_t)
	require.True(t, ok)
	// #nosec G115 -- effective Unix user IDs are non-negative and Stat_t.Uid is uint32.
	require.Equal(t, uint32(os.Geteuid()), markerStat.Uid)
	markerData, err := os.ReadFile(markerPath)
	require.NoError(t, err)
	require.Equal(t, token, string(markerData))
	require.NoError(t, os.WriteFile(
		filepath.Join(selectedHome, ".claude.json"),
		[]byte("selected-onboarding"),
		0600,
	))
}

func TestForwardAuthProjectionRejectsAmbientOnboardingActivation(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.Chmod(home, 0700))
	sentinel := filepath.Join(home, ".claude.json")
	require.NoError(t, os.WriteFile(sentinel, []byte("preserve"), 0600))

	helper := exec.Command(os.Args[0], "-test.run=^TestSyntheticProviderOnboardingProcess$")
	helper.Env = replaceTestEnvironment(os.Environ(), [][2]string{
		{"HOME", home},
		{"AGM_TEST_SYNTHETIC_ONBOARDING_TOKEN", "ambient-only"},
		{"AGM_TEST_SYNTHETIC_ONBOARDING_HOME", ""},
		{"AGM_TEST_SYNTHETIC_ONBOARDING_MARKER", ""},
	})
	output, err := helper.CombinedOutput()
	require.Error(t, err, string(output))
	data, readErr := os.ReadFile(sentinel)
	require.NoError(t, readErr)
	assert.Equal(t, "preserve", string(data))
}

func replaceTestEnvironment(base []string, replacements [][2]string) []string {
	replaced := make(map[string]struct{}, len(replacements))
	for _, replacement := range replacements {
		replaced[replacement[0]] = struct{}{}
	}

	result := make([]string, 0, len(base)+len(replacements))
	for _, entry := range base {
		key, _, found := strings.Cut(entry, "=")
		if _, replace := replaced[key]; found && replace {
			continue
		}
		result = append(result, entry)
	}
	for _, replacement := range replacements {
		result = append(result, replacement[0]+"="+replacement[1])
	}
	return result
}
