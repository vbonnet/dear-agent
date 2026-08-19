package dispatchstate

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestResolveRelayTargetPrefersLiveFileOverFallback(t *testing.T) {
	home := t.TempDir()
	result, err := SetRelayTarget(home, "dispatch-live")
	if err != nil {
		t.Fatalf("SetRelayTarget() error: %v", err)
	}
	if result.Target != "dispatch-live" {
		t.Fatalf("SetRelayTarget target = %q, want dispatch-live", result.Target)
	}
	got := ResolveRelayTarget(home, "vroom-orchestrator", func(string) string { return "" })
	if got.Target != "dispatch-live" {
		t.Fatalf("ResolveRelayTarget target = %q, want live file target", got.Target)
	}
	if got.Source != "file" {
		t.Fatalf("ResolveRelayTarget source = %q, want file", got.Source)
	}
}

// The state file must outrank the environment variable. If it did not,
// then on any host exporting AGM_COMPLETION_RELAY_TARGET the setter would
// report success while the running watcher kept relaying to the old
// session, which is the silent misrouting this package exists to prevent.
func TestResolveRelayTargetFileOverridesEnv(t *testing.T) {
	home := t.TempDir()
	if _, err := SetRelayTarget(home, "dispatch-file"); err != nil {
		t.Fatalf("SetRelayTarget() error: %v", err)
	}
	got := ResolveRelayTarget(home, "fallback", envReturning("dispatch-env"))
	if got.Target != "dispatch-file" {
		t.Fatalf("ResolveRelayTarget target = %q, want the live file target", got.Target)
	}
	if got.Source != "file" {
		t.Fatalf("ResolveRelayTarget source = %q, want file", got.Source)
	}
}

func TestResolveRelayTargetUsesEnvWhenNoFile(t *testing.T) {
	got := ResolveRelayTarget(t.TempDir(), "fallback", envReturning("dispatch-env"))
	if got.Target != "dispatch-env" {
		t.Fatalf("ResolveRelayTarget target = %q, want env target", got.Target)
	}
	if got.Source != "env:"+relayTargetEnv {
		t.Fatalf("ResolveRelayTarget source = %q, want env source", got.Source)
	}
	if got.Reason != "" {
		t.Fatalf("ResolveRelayTarget reason = %q, want none for an absent file", got.Reason)
	}
}

// An unreadable state file must be distinguishable from no override being
// set: both fall through, but only one means the live target was lost.
func TestResolveRelayTargetReportsUnreadableState(t *testing.T) {
	home := t.TempDir()
	path := RelayTargetPath(home)
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	got := ResolveRelayTarget(home, "fallback", envReturning(""))
	if got.Target != "fallback" {
		t.Fatalf("ResolveRelayTarget target = %q, want fallback", got.Target)
	}
	if got.Reason == "" {
		t.Fatal("ResolveRelayTarget reason is empty, want the read failure reported")
	}
}

func TestResolveRelayTargetReportsEmptyState(t *testing.T) {
	home := t.TempDir()
	path := RelayTargetPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	if err := os.WriteFile(path, []byte("   \n"), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	got := ResolveRelayTarget(home, "fallback", envReturning(""))
	if got.Target != "fallback" {
		t.Fatalf("ResolveRelayTarget target = %q, want fallback", got.Target)
	}
	if got.Reason == "" {
		t.Fatal("ResolveRelayTarget reason is empty, want the empty state reported")
	}
}

// SPEC: modes are enforced on paths that already exist with broader ones.
func TestSetRelayTargetEnforcesModesOnExistingPaths(t *testing.T) {
	home := t.TempDir()
	path := RelayTargetPath(home)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o777); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatalf("Chmod(dir) error: %v", err)
	}
	if err := os.WriteFile(path, []byte("stale\n"), 0o666); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	if err := os.Chmod(path, 0o666); err != nil {
		t.Fatalf("Chmod(file) error: %v", err)
	}
	if _, err := SetRelayTarget(home, "dispatch-live"); err != nil {
		t.Fatalf("SetRelayTarget() error: %v", err)
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat(dir) error: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != relayDirPerm {
		t.Fatalf("dir mode = %#o, want %#o", got, relayDirPerm)
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(file) error: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != relayFilePerm {
		t.Fatalf("file mode = %#o, want %#o", got, relayFilePerm)
	}
}

// The write must be atomic: a watcher resolving a completion reads this
// file at arbitrary moments, and a truncating write would let it observe an
// empty target and route that completion to the stale fallback. Run under
// -race; the assertion that matters is that no reader ever sees a value
// that was never set.
func TestSetRelayTargetIsAtomicUnderConcurrentReads(t *testing.T) {
	home := t.TempDir()
	if _, err := SetRelayTarget(home, "dispatch-a"); err != nil {
		t.Fatalf("SetRelayTarget() error: %v", err)
	}
	valid := map[string]bool{"dispatch-a": true, "dispatch-bbbbbbbbbbbb": true}

	var wg sync.WaitGroup
	stop := make(chan struct{})
	errCh := make(chan string, 64)

	for range 4 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				got := ResolveRelayTarget(home, "sentinel-fallback", envReturning(""))
				if !valid[got.Target] {
					select {
					case errCh <- fmt.Sprintf("target=%q source=%q reason=%q", got.Target, got.Source, got.Reason):
					default:
					}
					return
				}
			}
		})
	}

	for i := range 300 {
		target := "dispatch-a"
		if i%2 == 0 {
			target = "dispatch-bbbbbbbbbbbb"
		}
		if _, err := SetRelayTarget(home, target); err != nil {
			t.Errorf("SetRelayTarget() error: %v", err)
			break
		}
	}
	close(stop)
	wg.Wait()
	close(errCh)

	for msg := range errCh {
		t.Fatalf("reader observed a target that was never set: %s", msg)
	}
}

// A crash or concurrent write must never leave a temp file behind that a
// later reader could mistake for state.
func TestSetRelayTargetLeavesNoTempFiles(t *testing.T) {
	home := t.TempDir()
	for range 20 {
		if _, err := SetRelayTarget(home, "dispatch-live"); err != nil {
			t.Fatalf("SetRelayTarget() error: %v", err)
		}
	}
	entries, err := os.ReadDir(filepath.Dir(RelayTargetPath(home)))
	if err != nil {
		t.Fatalf("ReadDir() error: %v", err)
	}
	for _, entry := range entries {
		if entry.Name() != relayTargetFile {
			t.Fatalf("unexpected leftover file %q in relay state dir", entry.Name())
		}
	}
}

func envReturning(value string) func(string) string {
	return func(key string) string {
		if key == relayTargetEnv {
			return value
		}
		return ""
	}
}

func TestReadQuotaStatusWarnsOnThrottleAndLowRemaining(t *testing.T) {
	home := t.TempDir()
	path := QuotaPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	data := `{"updated_at":"2026-08-12T12:00:00Z","remaining_percent":19,"throttled":true}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	got := ReadQuotaStatus(home, "codex", time.Date(2026, 8, 12, 12, 5, 0, 0, time.UTC))
	if !got.Available {
		t.Fatalf("quota available = false, reason %q", got.Reason)
	}
	if !got.Warning {
		t.Fatalf("quota warning = false, want true")
	}
	if got.Stale {
		t.Fatalf("quota stale = true, want false")
	}
}

func TestReadQuotaStatusSelectsCodexBarProviderRecord(t *testing.T) {
	home := t.TempDir()
	path := QuotaPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	data := `{
		"generatedAt":"2026-08-12T12:00:00Z",
		"providers":[
			{"family":"anthropic","sourceId":"claude","remainingPercent":81,"breakerState":"closed"},
			{"family":"openai","sourceId":"codex","remainingPercent":46,"breakerState":"throttled","overspending":true}
		]
	}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	got := ReadQuotaStatus(home, "codex", time.Date(2026, 8, 12, 12, 5, 0, 0, time.UTC))
	if !got.Available {
		t.Fatalf("quota available = false, reason %q", got.Reason)
	}
	if !got.Warning {
		t.Fatalf("quota warning = false, want throttled provider warning")
	}
	if got.Data["sourceId"] != "codex" {
		t.Fatalf("selected provider = %v, want codex record", got.Data["sourceId"])
	}
}

func TestReadQuotaStatusUnavailableWhenStateMissing(t *testing.T) {
	got := ReadQuotaStatus(t.TempDir(), "codex", time.Now())
	if got.Available {
		t.Fatalf("quota available = true, want false")
	}
	if got.Reason != "quota state not found" {
		t.Fatalf("quota reason = %q, want quota state not found", got.Reason)
	}
}

// A provider absent from a structured provider collection must not degrade
// into "here is the whole file": that reports the missing provider as
// available and suppresses its pacing warnings.
func TestReadQuotaStatusRejectsProviderMissingFromCollection(t *testing.T) {
	home := writeQuota(t, `{
		"generatedAt":"2026-08-12T12:00:00Z",
		"providers":[{"family":"anthropic","sourceId":"claude","remainingPercent":81}]
	}`)
	got := ReadQuotaStatus(home, "codex", time.Date(2026, 8, 12, 12, 5, 0, 0, time.UTC))
	if got.Available {
		t.Fatalf("quota available = true for an absent provider, want false (data %v)", got.Data)
	}
	if got.Reason == "" {
		t.Fatal("quota reason is empty, want the missing provider named")
	}
}

func TestReadQuotaStatusSelectsNestedProviderCaseInsensitively(t *testing.T) {
	home := writeQuota(t, `{"Codex":{"updated_at":"2026-08-12T12:00:00Z","remaining_percent":15}}`)
	got := ReadQuotaStatus(home, "codex", time.Date(2026, 8, 12, 12, 5, 0, 0, time.UTC))
	if !got.Available {
		t.Fatalf("quota available = false, reason %q", got.Reason)
	}
	if !got.Warning {
		t.Fatal("quota warning = false, want true from low remaining quota")
	}
	if got.Stale {
		t.Fatal("quota stale = true, want false")
	}
}

// An unknown capture time must read as stale. Treating it as current lets
// Dispatch pace work from an arbitrarily old snapshot forever.
func TestReadQuotaStatusTreatsUnknownTimestampAsStale(t *testing.T) {
	for name, payload := range map[string]string{
		"missing": `{"remaining_percent":90}`,
		"invalid": `{"updated_at":"12 Aug 2026 12:00","remaining_percent":90}`,
	} {
		t.Run(name, func(t *testing.T) {
			home := writeQuota(t, payload)
			got := ReadQuotaStatus(home, "codex", time.Date(2026, 8, 12, 12, 5, 0, 0, time.UTC))
			if !got.Stale {
				t.Fatal("quota stale = false, want true for an unknown capture time")
			}
			if !got.Warning {
				t.Fatal("quota warning = false, want true because stale implies warning")
			}
			if got.Reason == "" {
				t.Fatal("quota reason is empty, want the unknown capture time explained")
			}
		})
	}
}

// A per-provider capture time beats the envelope's: in a multi-provider
// snapshot the envelope says nothing about how fresh one provider's
// numbers are.
func TestReadQuotaStatusPrefersProviderCaptureTime(t *testing.T) {
	home := writeQuota(t, `{
		"generatedAt":"2026-08-12T12:00:00Z",
		"providers":[{"sourceId":"codex","updated_at":"2026-08-12T10:00:00Z","remaining_percent":90}]
	}`)
	got := ReadQuotaStatus(home, "codex", time.Date(2026, 8, 12, 12, 5, 0, 0, time.UTC))
	if !got.Available {
		t.Fatalf("quota available = false, reason %q", got.Reason)
	}
	if !got.Stale {
		t.Fatal("quota stale = false, want true from the provider's own 2h-old capture time")
	}
}

func TestReadQuotaStatusWarnsOnStringThrottleFlag(t *testing.T) {
	home := writeQuota(t, `{"updated_at":"2026-08-12T12:00:00Z","remaining_percent":90,"warning":"true"}`)
	got := ReadQuotaStatus(home, "", time.Date(2026, 8, 12, 12, 5, 0, 0, time.UTC))
	if !got.Warning {
		t.Fatal("quota warning = false, want true for a string-encoded warning flag")
	}
}

func writeQuota(t *testing.T, payload string) string {
	t.Helper()
	home := t.TempDir()
	path := QuotaPath(home)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() error: %v", err)
	}
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}
	return home
}
