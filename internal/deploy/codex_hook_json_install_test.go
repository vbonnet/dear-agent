package deploy

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestCodexHookJSONHelperUsesDigestBoundOperatorInstall(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	makefileBytes, err := os.ReadFile(filepath.Join(repoRoot, "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	makefile := string(makefileBytes)
	start := strings.Index(makefile, "install-codex-hook-json: build-codex-hook-json")
	end := strings.Index(makefile, "\n# Enforces Definition of Done")
	if start < 0 || end <= start {
		t.Fatal("Makefile does not retain a bounded Codex hook JSON helper install target")
	}
	install := makefile[start:end]
	for _, required := range []string{
		"test -t 0",
		`expected_hash="$$(/usr/bin/openssl dgst -sha256 -r "$$artifact")"`,
		`root_installer="$$(/bin/cat "$$root_installer_path")"`,
		`expected_installer_hash="$$(printf '%s' "$$root_installer" | /usr/bin/openssl dgst -sha256 -r)"`,
		"IFS= read -r confirmed_hash",
		"IFS= read -r confirmed_installer_hash",
		`printf 'PROBE\n' | /usr/bin/sudo -k -n /bin/sh -c "$$root_installer"`,
		`printf 'INSTALL\n' | /usr/bin/sudo -k /bin/sh -c "$$root_installer"`,
		"/usr/local/libexec/dear-agent-codex-hook-json",
	} {
		if !strings.Contains(install, required) {
			t.Errorf("Codex hook JSON helper installer does not retain %q", required)
		}
	}
	if strings.Contains(install, "bin/codex-hook-json /usr/local/libexec/dear-agent-codex-hook-json") {
		t.Fatal("Codex hook JSON helper installer copies mutable build output directly to the privileged path")
	}
	if got := strings.Count(install, "/usr/bin/sudo"); got != 2 {
		t.Fatalf("Codex hook JSON helper installer uses %d sudo calls, want one probe and one transaction", got)
	}
	for _, forbidden := range []string{
		"/usr/bin/sudo /usr/bin/true",
		"/usr/bin/sudo -n /usr/bin/true",
		"/usr/bin/sudo /usr/bin/install",
		"/usr/bin/sudo /bin/mv",
	} {
		if strings.Contains(install, forbidden) {
			t.Errorf("Codex hook JSON helper installer retains reusable sudo flow %q", forbidden)
		}
	}

	rootInstallerBytes, err := os.ReadFile(filepath.Join(repoRoot, "scripts", "install-root-artifact.sh"))
	if err != nil {
		t.Fatal(err)
	}
	rootInstaller := string(rootInstallerBytes)
	for _, required := range []string{
		`test "$mode" != PROBE || exit 42`,
		`trusted "$dir"`,
		`trusted /usr/local/libexec`,
		`trusted_file "$destination"`,
		`test "$((0$mode_bits & 0001))" -ne 0`,
		`test "$((0$mode_bits))" -eq "$((0755))"`,
		`staging=$(/usr/bin/mktemp /usr/local/libexec/.dear-agent-root-artifact.XXXXXX)`,
		`/usr/bin/install -o root -g "$root_gid" -m 0755 "$artifact" "$staging"`,
		`test "$staged_hash" = "$expected_hash"`,
		`staged_identity=$(file_identity "$staging")`,
		`/bin/mv -f "$staging" "$destination"`,
		`test "$(file_identity "$destination")" = "$staged_identity"`,
		`test "$activated_hash" = "$expected_hash"`,
	} {
		if !strings.Contains(rootInstaller, required) {
			t.Errorf("fixed root artifact installer lacks %q", required)
		}
	}
	staging := strings.LastIndex(rootInstaller, `staging=$(/usr/bin/mktemp /usr/local/libexec/.dear-agent-root-artifact.XXXXXX)`)
	activation := strings.Index(rootInstaller, `/bin/mv -f "$staging" "$destination"`)
	trustedDestination := strings.Index(rootInstaller, `trusted /usr/local/libexec`)
	trustedLeaf := strings.Index(rootInstaller, `trusted_file "$destination"`)
	verifiedLeaf := strings.LastIndex(rootInstaller, `trusted_file "$destination"`)
	activatedHash := strings.Index(rootInstaller, `test "$activated_hash" = "$expected_hash"`)
	if trustedDestination < 0 || trustedLeaf <= trustedDestination || staging <= trustedLeaf || activation <= staging || verifiedLeaf <= activation || activatedHash <= verifiedLeaf {
		t.Fatal("fixed root artifact installer must verify the destination directory, stage inside it, then atomically rename")
	}
	if strings.Contains(rootInstaller, `trusted_parent=/private/var/root`) || strings.Contains(rootInstaller, `trusted_parent=/root`) {
		t.Fatal("fixed root artifact installer still stages on a potentially different filesystem")
	}

	for _, name := range []string{
		"pretool-bead-close-guard",
		"pretool-beads-dir-block",
		"pretool-bypass-guard",
		"pretool-pr-guard",
		"pretool-spawn-routing",
	} {
		hookBytes, readErr := os.ReadFile(filepath.Join(repoRoot, ".codex", "hooks", name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		hook := string(hookBytes)
		if !strings.Contains(hook, `hook_json() { /usr/local/libexec/dear-agent-codex-hook-json "$@"; }`) {
			t.Errorf("%s does not select the operator-owned JSON helper for bypassed hooks", name)
		}
		if !strings.Contains(hook, `if [ -n "${AGM_CODEX_HOOK_ROOT:-}" ]`) ||
			!strings.Contains(hook, "command -v jq") ||
			!strings.Contains(hook, `hook_json() { jq "$@"; }`) {
			t.Errorf("%s does not preserve ordinary reviewed-session jq lookup", name)
		}
	}
}

func TestRootArtifactInstallerPublishesSPECContentAddressWithoutClobber(t *testing.T) {
	repoRoot := filepath.Join("..", "..")
	data, err := os.ReadFile(filepath.Join(repoRoot, "scripts", "install-root-artifact.sh"))
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	for _, required := range []string{
		`pinned_destination="$destination.$expected_hash"`,
		`reuse_pinned() { trusted_file "$pinned_destination" || exit 2`,
		`trusted_file "$pinned_destination" || exit 2`,
		`pinned_hash=$(/usr/bin/openssl dgst -sha256 -r "$pinned_destination")`,
		`test "$pinned_hash" = "$expected_hash"`,
		`/bin/ln "$pinned_destination" "$staging"`,
		`elif /bin/ln "$staging" "$pinned_destination"; then :; else reuse_pinned; fi`,
		`test "$(file_identity "$pinned_destination")" = "$staged_identity"`,
		`test "$(file_identity "$destination")" = "$staged_identity"; then /bin/rm -f "$staging"`,
		`elif /bin/mv -f "$staging" "$destination"; then if test -e "$staging" || test -L "$staging"; then /bin/rm -f "$staging"; fi`,
		`else trusted_file "$destination" || exit 2; test "$(file_identity "$destination")" = "$staged_identity" || exit 2; /bin/rm -f "$staging"; fi`,
	} {
		if !strings.Contains(script, required) {
			t.Errorf("fixed root artifact installer lacks content-addressed invariant %q", required)
		}
	}
	existing := strings.Index(script, `if test -e "$pinned_destination" || test -L "$pinned_destination"`)
	newLink := strings.Index(script, `elif /bin/ln "$staging" "$pinned_destination"; then :; else reuse_pinned; fi`)
	activation := strings.Index(script, `/bin/mv -f "$staging" "$destination"`)
	if existing < 0 || newLink <= existing || activation <= newLink {
		t.Fatal("SPEC installer must reject a mismatched existing digest leaf or no-clobber link a new leaf before stable activation")
	}
	if strings.Contains(script, `/bin/mv -f "$staging" "$pinned_destination"`) ||
		strings.Contains(script, `/bin/ln -f`) || strings.Contains(script, `/bin/rm -f "$pinned_destination"`) {
		t.Fatal("SPEC installer must never replace or clean up a published content-addressed helper")
	}
}

func TestContentAddressedPublicationRaceReusesExactWinner(t *testing.T) {
	directory := t.TempDir()
	pinned := filepath.Join(directory, "helper.digest")
	body := []byte("reviewed helper\n")
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	ready := sync.WaitGroup{}
	ready.Add(2)
	release := make(chan struct{})
	type result struct {
		reused bool
		err    error
	}
	results := make(chan result, 2)
	for index := range 2 {
		go func() {
			staging := filepath.Join(directory, fmt.Sprintf("staging-%d", index))
			if err := os.WriteFile(staging, body, 0o755); err != nil {
				ready.Done()
				results <- result{err: err}
				return
			}
			if _, err := os.Lstat(pinned); !errors.Is(err, os.ErrNotExist) {
				ready.Done()
				results <- result{err: fmt.Errorf("initial pinned state: %w", err)}
				return
			}
			ready.Done()
			<-release
			reused, err := publishPinnedModel(staging, pinned, digest)
			results <- result{reused: reused, err: err}
		}()
	}
	ready.Wait()
	close(release)
	reused := 0
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.reused {
			reused++
		}
	}
	if reused != 1 {
		t.Fatalf("race reused winner %d times, want exactly one losing publisher", reused)
	}
}

func publishPinnedModel(staging, pinned, expectedDigest string) (bool, error) {
	if err := os.Link(staging, pinned); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrExist) {
		return false, err
	}
	body, err := os.ReadFile(pinned)
	if err != nil {
		return false, err
	}
	if info, err := os.Lstat(pinned); err != nil {
		return false, err
	} else if !info.Mode().IsRegular() || info.Mode().Perm() != 0o755 ||
		fmt.Sprintf("%x", sha256.Sum256(body)) != expectedDigest {
		return false, fmt.Errorf("winning content-addressed leaf is not trusted and exact")
	}
	if err := os.Remove(staging); err != nil {
		return false, err
	}
	if err := os.Link(pinned, staging); err != nil {
		return false, err
	}
	pinnedInfo, err := os.Stat(pinned)
	if err != nil {
		return false, err
	}
	stagingInfo, err := os.Stat(staging)
	if err != nil {
		return false, err
	}
	if !os.SameFile(pinnedInfo, stagingInfo) {
		return false, fmt.Errorf("losing publisher did not reuse winning identity")
	}
	return true, nil
}

func TestStableActivationRaceAcceptsExactWinnerAndRemovesAliases(t *testing.T) {
	directory := t.TempDir()
	pinned := filepath.Join(directory, "helper.digest")
	stable := filepath.Join(directory, "helper")
	if err := os.WriteFile(pinned, []byte("reviewed helper\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stable, []byte("prior helper\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	pinnedInfo, err := os.Stat(pinned)
	if err != nil {
		t.Fatal(err)
	}
	ready := sync.WaitGroup{}
	ready.Add(2)
	release := make(chan struct{})
	results := make(chan error, 2)
	stagingPaths := make([]string, 2)
	for index := range 2 {
		staging := filepath.Join(directory, fmt.Sprintf("activation-%d", index))
		stagingPaths[index] = staging
		if err := os.Link(pinned, staging); err != nil {
			t.Fatal(err)
		}
		go func(staging string) {
			stableBefore, err := os.Stat(stable)
			if err != nil {
				ready.Done()
				results <- fmt.Errorf("inspect initial stable identity: %w", err)
				return
			}
			if os.SameFile(stableBefore, pinnedInfo) {
				ready.Done()
				results <- errors.New("initial stable identity unexpectedly matched published helper")
				return
			}
			ready.Done()
			<-release
			results <- activateStableModel(staging, stable, pinnedInfo)
		}(staging)
	}
	ready.Wait()
	close(release)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	stableInfo, err := os.Stat(stable)
	if err != nil || !os.SameFile(stableInfo, pinnedInfo) {
		t.Fatalf("stable identity was not the exact published winner: %v", err)
	}
	for _, staging := range stagingPaths {
		if _, err := os.Lstat(staging); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("residual staging alias %s: %v", staging, err)
		}
	}
}

func activateStableModel(staging, stable string, expected os.FileInfo) error {
	if err := os.Rename(staging, stable); err != nil {
		stableInfo, statErr := os.Stat(stable)
		if statErr != nil || !os.SameFile(stableInfo, expected) {
			return fmt.Errorf("activate stable helper: %w", err)
		}
	}
	if _, err := os.Lstat(staging); err == nil {
		if err := os.Remove(staging); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	stableInfo, err := os.Stat(stable)
	if err != nil {
		return err
	}
	if !os.SameFile(stableInfo, expected) {
		return fmt.Errorf("stable helper does not match the exact published identity")
	}
	return nil
}
