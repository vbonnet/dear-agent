package daemon

import (
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockExecutor records command executions for testing.
type MockExecutor struct {
	mu       sync.Mutex
	calls    []MockCall
	failNext bool
	failMsg  string
}

type MockCall struct {
	Name string
	Args []string
}

func (m *MockExecutor) Execute(name string, args ...string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, MockCall{Name: name, Args: args})
	// Return valid safe JSON for safety check calls (never fail these)
	if len(args) > 0 && args[0] == "safety" {
		return []byte(`{"safe":true}`), nil
	}
	if m.failNext {
		m.failNext = false
		return []byte(m.failMsg), fmt.Errorf("command failed")
	}
	return []byte("ok"), nil
}

// nonSafetyCalls returns calls excluding safety check calls (for test assertions).
func (m *MockExecutor) nonSafetyCalls() []MockCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []MockCall
	for _, c := range m.calls {
		if len(c.Args) > 0 && c.Args[0] == "safety" {
			continue
		}
		result = append(result, c)
	}
	return result
}

func (m *MockExecutor) GetCalls() []MockCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]MockCall, len(m.calls))
	copy(result, m.calls)
	return result
}

func newTestPipeline(executor *MockExecutor) *EscalationPipeline {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return NewEscalationPipeline(executor, logger, "agm", 5)
}

func TestSafetyClassifier_Safe(t *testing.T) {
	sc := NewSafetyClassifier()

	safeCommands := []string{
		"git status",
		"git log --oneline",
		"git diff HEAD",
		"cat README.md",
		"ls -la",
		"go test ./...",
		"go build .",
		"go vet ./...",
		"head -n 10 file.txt",
		"tail -f log.txt",
		"grep -r pattern .",
		"rg pattern",
		"find . -name '*.go'",
		"wc -l file.txt",
		"git show HEAD",
		"git branch -a",
		"npm test",
		"make test",
		"mkdir -p dir",
		"touch file.txt",
	}

	for _, cmd := range safeCommands {
		t.Run(cmd, func(t *testing.T) {
			assert.Equal(t, ClassificationSafe, sc.Classify(cmd), "expected safe: %s", cmd)
		})
	}
}

func TestSafetyClassifier_Dangerous(t *testing.T) {
	sc := NewSafetyClassifier()

	dangerousCommands := []string{
		"git push origin main --force",
		"git push -f origin main",
		"git clean -fd",
		"git reset --hard HEAD~1",
		"git checkout .",
		"git restore .",
		"rm -rf /tmp/dir",
		"rm -r somedir",
		"chmod 777 script.sh",
		"chown root:root file",
		"cat .env",
		"cat credentials",
	}

	for _, cmd := range dangerousCommands {
		t.Run(cmd, func(t *testing.T) {
			assert.Equal(t, ClassificationDangerous, sc.Classify(cmd), "expected dangerous: %s", cmd)
		})
	}
}

func TestSafetyClassifier_Unknown(t *testing.T) {
	sc := NewSafetyClassifier()

	unknownCommands := []string{
		"npm install express",
		"python script.py",
		"curl https://example.com",
		"docker run nginx",
		"rustc main.rs",
	}

	for _, cmd := range unknownCommands {
		t.Run(cmd, func(t *testing.T) {
			assert.Equal(t, ClassificationUnknown, sc.Classify(cmd), "expected unknown: %s", cmd)
		})
	}
}

func TestEscalate_PermissionPrompt_SafeCommand(t *testing.T) {
	executor := &MockExecutor{}
	pipeline := newTestPipeline(executor)

	result, err := pipeline.Escalate("test-session", "stuck_permission_prompt", "git status")
	require.NoError(t, err)
	assert.Equal(t, ActionApprove, result.Action)
	assert.True(t, result.Success)

	calls := executor.nonSafetyCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "agm", calls[0].Name)
	assert.Equal(t, []string{"send", "approve", "test-session"}, calls[0].Args)
}

func TestEscalate_PermissionPrompt_DangerousCommand(t *testing.T) {
	executor := &MockExecutor{}
	pipeline := newTestPipeline(executor)

	result, err := pipeline.Escalate("test-session", "stuck_permission_prompt", "git push --force origin main")
	require.NoError(t, err)
	// MVP: dangerous commands notify instead of auto-reject
	assert.Equal(t, ActionNotify, result.Action)
	assert.True(t, result.Success)

	calls := executor.nonSafetyCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "agm", calls[0].Name)
	assert.Equal(t, "send", calls[0].Args[0])
	assert.Equal(t, "msg", calls[0].Args[1])
	assert.Equal(t, "test-session", calls[0].Args[2])
}

func TestEscalate_PermissionPrompt_UnknownCommand(t *testing.T) {
	executor := &MockExecutor{}
	pipeline := newTestPipeline(executor)

	result, err := pipeline.Escalate("test-session", "stuck_permission_prompt", "npm install express")
	require.NoError(t, err)
	assert.Equal(t, ActionNotify, result.Action)
	assert.True(t, result.Success)

	calls := executor.nonSafetyCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "agm", calls[0].Name)
	assert.Equal(t, "send", calls[0].Args[0])
	assert.Equal(t, "msg", calls[0].Args[1])
	assert.Equal(t, "test-session", calls[0].Args[2])
	assert.Equal(t, "--sender", calls[0].Args[3])
	assert.Equal(t, "astrocyte", calls[0].Args[4])
	assert.Equal(t, "--prompt", calls[0].Args[5])
}

func TestEscalate_StuckMustering(t *testing.T) {
	executor := &MockExecutor{}
	pipeline := newTestPipeline(executor)

	result, err := pipeline.Escalate("test-session", "stuck_mustering", "")
	require.NoError(t, err)
	assert.Equal(t, ActionKillResume, result.Action)
	assert.True(t, result.Success)

	calls := executor.nonSafetyCalls()
	require.Len(t, calls, 2)

	// First call: kill
	assert.Equal(t, "agm", calls[0].Name)
	assert.Equal(t, []string{"session", "kill", "test-session", "--force"}, calls[0].Args)

	// Second call: resume
	assert.Equal(t, "agm", calls[1].Name)
	assert.Equal(t, []string{"session", "resume", "test-session", "--detached"}, calls[1].Args)
}

func TestEscalate_StuckWaiting(t *testing.T) {
	executor := &MockExecutor{}
	pipeline := newTestPipeline(executor)

	result, err := pipeline.Escalate("test-session", "stuck_waiting", "")
	require.NoError(t, err)
	assert.Equal(t, ActionNotify, result.Action)
	assert.True(t, result.Success)

	// Only notification, no kill/resume (stuck_waiting goes directly to notify, no safety check)
	calls := executor.nonSafetyCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "send", calls[0].Args[0])
	assert.Equal(t, "msg", calls[0].Args[1])
}

func TestRateLimit(t *testing.T) {
	executor := &MockExecutor{}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	pipeline := NewEscalationPipeline(executor, logger, "agm", 2)

	// First two should approve
	result1, err := pipeline.Escalate("test-session", "stuck_permission_prompt", "git status")
	require.NoError(t, err)
	assert.Equal(t, ActionApprove, result1.Action)

	result2, err := pipeline.Escalate("test-session", "stuck_permission_prompt", "git diff")
	require.NoError(t, err)
	assert.Equal(t, ActionApprove, result2.Action)

	// Third should fall back to notification (rate limited)
	result3, err := pipeline.Escalate("test-session", "stuck_permission_prompt", "git log")
	require.NoError(t, err)
	assert.Equal(t, ActionNotify, result3.Action)
	assert.Contains(t, result3.Message, "Rate limit reached")
}

func TestEscalate_KillFailure(t *testing.T) {
	// failNext only applies to non-safety calls, so the safety check succeeds
	// and the kill call (first non-safety call) fails
	executor := &MockExecutor{failNext: true, failMsg: "session not found"}
	pipeline := newTestPipeline(executor)

	result, err := pipeline.Escalate("test-session", "stuck_mustering", "")
	require.NoError(t, err)
	assert.Equal(t, ActionKillResume, result.Action)
	assert.False(t, result.Success)
	assert.Contains(t, result.Message, "kill failed")
}

func TestConcurrentEscalation(t *testing.T) {
	executor := &MockExecutor{}
	pipeline := newTestPipeline(executor)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sessionName := fmt.Sprintf("session-%d", idx)
			_, err := pipeline.Escalate(sessionName, "stuck_permission_prompt", "git status")
			assert.NoError(t, err)
		}(i)
	}
	wg.Wait()

	// All 10 sessions should have been processed (each has its own rate limit)
	calls := executor.nonSafetyCalls()
	assert.Len(t, calls, 10)
}

// HumanDetectedExecutor returns unsafe JSON for safety check calls.
type HumanDetectedExecutor struct {
	MockExecutor
}

func (h *HumanDetectedExecutor) Execute(name string, args ...string) ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.calls = append(h.calls, MockCall{Name: name, Args: args})
	if len(args) > 0 && args[0] == "safety" {
		// Return violation JSON with exit code 1
		return []byte(`{"safe":false,"violations":[{"guard":"human_attached","message":"1 client attached"}]}`),
			fmt.Errorf("safety check failed")
	}
	return []byte("ok"), nil
}

func TestEscalate_SafetyGuardBlocks_PermissionPrompt(t *testing.T) {
	executor := &HumanDetectedExecutor{}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	pipeline := NewEscalationPipeline(executor, logger, "agm", 5)

	// With human detected, safe command should be downgraded to notify instead of approve
	result, err := pipeline.Escalate("test-session", "stuck_permission_prompt", "git status")
	require.NoError(t, err)
	assert.Equal(t, ActionNotify, result.Action)
	assert.Contains(t, result.Message, "Human detected")
}

func TestEscalate_SafetyGuardBlocks_StuckSession(t *testing.T) {
	executor := &HumanDetectedExecutor{}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	pipeline := NewEscalationPipeline(executor, logger, "agm", 5)

	// With human detected, stuck session should be downgraded to notify instead of kill/resume
	result, err := pipeline.Escalate("test-session", "stuck_mustering", "")
	require.NoError(t, err)
	assert.Equal(t, ActionNotify, result.Action)
	assert.Contains(t, result.Message, "Human detected")

	// Should NOT have made any kill/resume calls
	nonSafety := executor.nonSafetyCalls()
	// Only the notify call should be present
	require.Len(t, nonSafety, 1)
	assert.Equal(t, "send", nonSafety[0].Args[0])
	assert.Equal(t, "msg", nonSafety[0].Args[1])
}
