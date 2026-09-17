package testcontext_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	llmauth "github.com/vbonnet/dear-agent/pkg/llm/auth"
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

func TestForwardAuthProjectionClaudeRefreshUsesCanonicalHostLeaf(t *testing.T) {
	hostHome := t.TempDir()
	hostClaudeDir := filepath.Join(hostHome, ".claude")
	require.NoError(t, os.MkdirAll(hostClaudeDir, 0700))
	hostCredential := filepath.Join(hostClaudeDir, ".credentials.json")
	initialCredential := `{
  "claudeAiOauth": {
    "accessToken": "synthetic-access-before",
    "expiresAt": 1,
    "refreshToken": "synthetic-refresh-before",
    "scopes": ["user:inference"]
  }
}`
	require.NoError(t, os.WriteFile(hostCredential, []byte(initialCredential), 0600))
	hostBefore, err := os.Lstat(hostCredential)
	require.NoError(t, err)

	tc := testcontext.New()
	require.NoError(t, tc.EnsureDirs())
	t.Cleanup(func() {
		require.NoError(t, tc.Cleanup())
	})
	require.NoError(t, tc.ForwardAuth(hostHome, testcontext.AuthModeInherit))

	selectedCredential := filepath.Join(tc.HomeDir, ".claude", ".credentials.json")
	selectedBefore, err := os.Lstat(selectedCredential)
	require.NoError(t, err)
	require.NotZero(t, selectedBefore.Mode()&os.ModeSymlink)
	targetBefore, err := os.Readlink(selectedCredential)
	require.NoError(t, err)
	assert.Equal(t, hostCredential, targetBefore)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "cannot parse synthetic refresh request", http.StatusBadRequest)
			return
		}
		if got := r.FormValue("refresh_token"); got != "synthetic-refresh-before" {
			http.Error(w, "unexpected synthetic refresh token", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "synthetic-access-after",
			"expires_in":    3600,
			"refresh_token": "synthetic-refresh-after",
			"token_type":    "Bearer",
		}); err != nil {
			t.Errorf("encode synthetic refresh response: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	t.Setenv("HOME", tc.HomeDir)
	resolver := llmauth.OAuthResolver{
		HTTPClient:    server.Client(),
		TokenEndpoint: server.URL,
	}
	token, err := resolver.Refresh(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "synthetic-access-after", token)

	selectedAfter, err := os.Lstat(selectedCredential)
	require.NoError(t, err)
	require.NotZero(t, selectedAfter.Mode()&os.ModeSymlink)
	assert.True(t, os.SameFile(selectedBefore, selectedAfter), "selected credential link identity changed")
	targetAfter, err := os.Readlink(selectedCredential)
	require.NoError(t, err)
	assert.Equal(t, targetBefore, targetAfter)

	var persisted struct {
		ClaudeAIOAuth struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
		} `json:"claudeAiOauth"`
	}
	hostData, err := os.ReadFile(hostCredential)
	require.NoError(t, err)
	hostAfter, err := os.Lstat(hostCredential)
	require.NoError(t, err)
	assert.False(t, os.SameFile(hostBefore, hostAfter), "host credential was not atomically replaced")
	require.NoError(t, json.Unmarshal(hostData, &persisted))
	assert.Equal(t, "synthetic-access-after", persisted.ClaudeAIOAuth.AccessToken)
	assert.Equal(t, "synthetic-refresh-after", persisted.ClaudeAIOAuth.RefreshToken)
	selectedData, err := os.ReadFile(selectedCredential)
	require.NoError(t, err)
	assert.Equal(t, hostData, selectedData)

	backupData, err := os.ReadFile(hostCredential + ".bak")
	require.NoError(t, err)
	assert.JSONEq(t, initialCredential, string(backupData))
	_, err = os.Stat(filepath.Join(hostClaudeDir, ".credentials.lock"))
	require.NoError(t, err)
	for _, selectedSibling := range []string{
		filepath.Join(tc.HomeDir, ".claude", ".credentials.lock"),
		selectedCredential + ".bak",
	} {
		_, err = os.Lstat(selectedSibling)
		require.ErrorIs(t, err, os.ErrNotExist)
	}
	selectedEntries, err := os.ReadDir(filepath.Join(tc.HomeDir, ".claude"))
	require.NoError(t, err)
	require.Len(t, selectedEntries, 1)
	assert.Equal(t, ".credentials.json", selectedEntries[0].Name())
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
