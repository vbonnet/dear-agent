package tmux

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/debug"
)

// AGM drives tmux as a text protocol, so bytes have to survive in both
// directions. Under a non-UTF-8 locale they do not, and the two directions
// fail for different reasons and need different remedies.
//
// Direction 1: what tmux writes back. Format results are transliterated to
// ASCII unless the *client* is in UTF-8 mode, which tmux decides from the
// first of LC_ALL, LC_CTYPE, LANG that is set (tmux(1)). With none set,
// `display-message -p '#{pane_current_path}'` returns every non-ASCII and
// control byte as "_", so EnsureSessionWorkDir compares the real workdir
// against a mangled pane path, declares a false mismatch, and pastes a
// corrective `cd` that ValidatePastedText then rejects outright. The remedy is
// tmux's own -u flag on the reads that carry arbitrary bytes: it forces UTF-8
// output for that one invocation, needs no locale to exist, and works even
// against a server that was started without one.
//
// AGM deliberately does NOT export a locale into its own environment to get
// this. Every subprocess AGM runs would inherit it, and AGM parses
// English-language output from some of them, so a process-wide locale could
// change what those parsers see.
//
// Direction 2: what the pane's own shell can handle. Panes inherit the tmux
// *server's* environment, and tmux's update-environment does not carry LC_*
// across from the client, so a server started without a locale keeps that for
// its whole life. A pane shell running under LC_CTYPE=C drops the multi-byte
// characters of a literal send-keys command line, so a harness launch command
// carrying box-drawing glyphs never execs and readiness reports WRONG_HARNESS.
// The remedy is to pin the locale into the *session* environment at creation
// with `new-session -e`, which overrides what the session would inherit.
//
// Only LC_CTYPE is pinned, never LC_MESSAGES, so a pane's programs keep
// whatever language they would otherwise use. LC_ALL is pinned to the empty
// string alongside it, because a non-empty LC_ALL inherited from the server
// would outrank LC_CTYPE and silently undo the pin; libc treats an empty
// LC_ALL as unset.
//
// Neither failure is visible on a developer laptop, where LANG is normally
// already UTF-8. They appear wherever AGM runs without an exported locale:
// launchd, cron, CI, a container, a bare agent shell.

// localeEnvVars is tmux's precedence order for deciding UTF-8 support.
var localeEnvVars = []string{"LC_ALL", "LC_CTYPE", "LANG"}

// localeIsUTF8 reports whether an environment would be treated as UTF-8. It
// mirrors tmux's rule exactly: only the first variable that is set is
// consulted, so a UTF-8 LC_CTYPE does not rescue a non-UTF-8 LC_ALL.
func localeIsUTF8(lookup func(string) (string, bool)) bool {
	for _, name := range localeEnvVars {
		value, ok := lookup(name)
		if !ok {
			continue
		}
		return localeNameIsUTF8(value)
	}
	return false
}

// localeIsUTF8ForShell reports whether a *program running in a pane* would use
// UTF-8. It differs from localeIsUTF8 on exactly one case, and that case is
// the whole reason the session pin works: POSIX says LC_ALL overrides the
// other variables only when it is set to a NON-EMPTY string, so an empty
// LC_ALL falls through to LC_CTYPE. tmux's own client-side check is a plain
// "is it set" string test and does not make that distinction.
//
// Pinning `LC_ALL=` alongside `LC_CTYPE=<utf8>` therefore neutralises an
// LC_ALL the session would otherwise inherit from an old server, without
// having to pick a value for it and without touching LC_MESSAGES.
func localeIsUTF8ForShell(lookup func(string) (string, bool)) bool {
	for _, name := range localeEnvVars {
		value, ok := lookup(name)
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		return localeNameIsUTF8(value)
	}
	return false
}

// localeNameIsUTF8 reports whether a locale name selects the UTF-8 codeset.
// The codeset sits between the territory and an optional @modifier, so
// "sr_RS.UTF-8@latin" is a UTF-8 locale even though the name does not end in
// the codeset.
func localeNameIsUTF8(name string) bool {
	value, _, _ := strings.Cut(strings.TrimSpace(name), "@")
	_, codeset, found := strings.Cut(value, ".")
	if !found {
		return false
	}
	return strings.ToUpper(strings.ReplaceAll(codeset, "-", "")) == "UTF8"
}

// pickUTF8Locale chooses a UTF-8 locale from the names the host actually has
// installed. Only installed names are eligible: exporting a locale the C
// library cannot load makes tools such as bash and perl print "cannot change
// locale" into the pane, which is exactly the output corruption this code
// exists to prevent. That is also why an inherited-but-unverified name is
// never forwarded; AGM always pins a name it has seen in the installed set.
//
// C.UTF-8 is preferred because it is locale-data-free and behaves identically
// everywhere; en_US.UTF-8 is the usual fallback on hosts (including macOS)
// that do not ship C.UTF-8. Any other installed UTF-8 locale is still
// correct for AGM's purpose, because only LC_CTYPE is ever pinned.
func pickUTF8Locale(available []string) string {
	installed := make(map[string]string, len(available))
	for _, name := range available {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		installed[strings.ToUpper(trimmed)] = trimmed
	}
	for _, preferred := range []string{"C.UTF-8", "C.utf8", "en_US.UTF-8", "en_US.utf8"} {
		if name, ok := installed[strings.ToUpper(preferred)]; ok {
			return name
		}
	}
	for _, name := range available {
		trimmed := strings.TrimSpace(name)
		if localeNameIsUTF8(trimmed) {
			return trimmed
		}
	}
	return ""
}

// installedLocales lists the locales the host can load. The second result
// says whether enumeration itself worked, which is NOT the same question as
// whether any UTF-8 locale exists.
//
// When enumeration is impossible AGM pins nothing. Guessing a name here would
// defeat the point: a locale the C library cannot load makes shells and tools
// print "cannot change locale" into the pane, which is precisely the output
// corruption this code exists to prevent, and it would break the promise that
// only installed locales are pinned. Skipping the pin merely leaves the pane
// as it already was, so the two failure modes are not symmetric.
func installedLocales() ([]string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "locale", "-a").Output()
	if err != nil {
		return nil, false
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n"), true
}

// resolveSessionLocale picks the locale to pin into new sessions, or "" when
// the host cannot prove one is installed.
func resolveSessionLocale(enumerate func() ([]string, bool)) string {
	available, enumerated := enumerate()
	if !enumerated {
		return ""
	}
	return pickUTF8Locale(available)
}

// minSessionEnvVersion is the first tmux release whose new-session accepts
// `-e`; 3.1 and earlier reject the flag and would fail session creation
// outright. AGM's documented floor is older than that, so the pin is gated
// and simply does not happen on an older server.
var minSessionEnvVersion = tmuxVersion{major: 3, minor: 2}

type tmuxVersion struct{ major, minor int }

func (v tmuxVersion) atLeast(other tmuxVersion) bool {
	if v.major != other.major {
		return v.major > other.major
	}
	return v.minor >= other.minor
}

// parseTmuxVersion reads the numeric prefix of a `tmux -V` line. Releases
// carry a letter suffix ("tmux 3.7c") and development builds report
// "tmux next-3.4", both of which resolve to their numeric release.
func parseTmuxVersion(raw string) (tmuxVersion, bool) {
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) < 2 {
		return tmuxVersion{}, false
	}
	value := strings.TrimPrefix(fields[1], "next-")
	major, rest, found := strings.Cut(value, ".")
	if !found {
		return tmuxVersion{}, false
	}
	majorNum, err := strconv.Atoi(major)
	if err != nil {
		return tmuxVersion{}, false
	}
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end == 0 {
		return tmuxVersion{}, false
	}
	minorNum, err := strconv.Atoi(rest[:end])
	if err != nil {
		return tmuxVersion{}, false
	}
	return tmuxVersion{major: majorNum, minor: minorNum}, true
}

// installedLocaleCache memoizes the pinnable locale. Only a successful
// resolution latches: a transient `locale -a` timeout used to be cached for
// the lifetime of a long-running AGM process, so every later session skipped
// an available pin long after the host recovered. Enumeration shells out, so
// a negative result is still rate-limited rather than retried on every call.
var installedLocaleCache struct {
	mu       sync.Mutex
	value    string
	resolved bool
	lastTry  time.Time
}

// installedLocaleRetryAfter bounds how often a failed enumeration is retried.
const installedLocaleRetryAfter = time.Minute

// localeEnumerator is the enumeration seam, so a test can state that the host
// cannot enumerate rather than depending on one that cannot.
var localeEnumerator = installedLocales

// pinnableLocale is the installed UTF-8 locale AGM would pin, cached because
// enumerating locales shells out. Empty means the host cannot prove one.
func pinnableLocale() string {
	installedLocaleCache.mu.Lock()
	defer installedLocaleCache.mu.Unlock()
	if installedLocaleCache.resolved {
		return installedLocaleCache.value
	}
	if !installedLocaleCache.lastTry.IsZero() &&
		time.Since(installedLocaleCache.lastTry) < installedLocaleRetryAfter {
		return ""
	}
	installedLocaleCache.lastTry = time.Now()
	if value := resolveSessionLocale(localeEnumerator); value != "" {
		installedLocaleCache.value = value
		installedLocaleCache.resolved = true
	}
	return installedLocaleCache.value
}

// serverTmuxVersion reports the version of the tmux server actually listening
// on socketPath. This deliberately does not use `tmux -V`, which is socketless
// and reports the CLIENT binary: after an upgrade from 3.1 to 3.2+ the
// long-lived server keeps running the old binary, and trusting the client
// there would send `-e` to a server that rejects it and fail every new
// session. The second result is false when no server is reachable, which is
// the normal case just before AGM starts one.
func serverTmuxVersion(ctx context.Context, socketPath string) (tmuxVersion, bool) {
	out, err := RunWithTimeout(ctx, globalTimeout, "tmux", "-S", socketPath,
		"display-message", "-p", "#{version}")
	if err != nil {
		return tmuxVersion{}, false
	}
	// #{version} is bare ("3.7c"); parseTmuxVersion expects `tmux -V` shape.
	return parseTmuxVersion("tmux " + strings.TrimSpace(string(out)))
}

// serverLocaleUsableUTF8 reports whether the server's own environment already
// gives panes a usable UTF-8 locale, and whether that could be established at
// all. The second result matters: a failed probe is not evidence of ASCII, and
// treating it as such would blank a perfectly good LC_ALL.
func serverLocaleUsableUTF8(ctx context.Context, socketPath string) (utf8, known bool) {
	out, err := RunWithTimeout(ctx, globalTimeout, "tmux", "-S", socketPath, "show-environment", "-g")
	if err != nil {
		return false, false
	}
	block := string(out)
	return localeIsUsableUTF8(func(key string) (string, bool) {
		for line := range strings.SplitSeq(block, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "-"+key {
				return "", false
			}
			if name, value, found := strings.Cut(trimmed, "="); found && name == key {
				return value, true
			}
		}
		return "", false
	}), true
}

// localeIsUsableUTF8 is localeIsUTF8ForShell plus the installation check. A
// name can be syntactically UTF-8 and still unloadable: LANG=en_US.UTF-8 on an
// image that only ships C.utf8 selects nothing, so a pane inheriting it gets
// setlocale warnings and ASCII behavior. Treating that as "already fine" would
// suppress the pin precisely where it is needed, so an uninstalled name counts
// as not UTF-8 and lets the proven locale be pinned instead.
func localeIsUsableUTF8(lookup func(string) (string, bool)) bool {
	return localeIsUsableUTF8With(lookup, installedLocales)
}

// localeIsUsableUTF8With is the enumeration seam, matching resolveSessionLocale
// so a test can state what the host installed instead of depending on it.
func localeIsUsableUTF8With(lookup func(string) (string, bool), enumerate func() ([]string, bool)) bool {
	if !localeIsUTF8ForShell(lookup) {
		return false
	}
	name := effectiveLocaleName(lookup)
	available, enumerated := enumerate()
	if !enumerated {
		// Cannot verify, and cannot pin either (pinnableLocale is empty in
		// this case), so report it as usable and change nothing.
		return true
	}
	for _, candidate := range available {
		// Exact match, not EqualFold. setlocale is case-sensitive about the
		// names locale(1) enumerates, so an inherited LC_ALL=c.utf8 on a host
		// whose locale -a reports C.utf8 is NOT usable: setlocale rejects it
		// and falls back to ASCII with warnings. Folding case there marked it
		// installed, suppressed the pin, and left exactly the byte corruption
		// this file exists to prevent.
		//
		// Erring the other way is cheap. A spelling this check fails to
		// recognize is answered by pinning a locale the host has proven,
		// which is the outcome the whole feature is built around.
		if strings.TrimSpace(candidate) == name {
			return true
		}
	}
	return false
}

// effectiveLocaleName returns the value of the first locale variable that is
// set to a non-empty string, which is the one POSIX actually uses.
func effectiveLocaleName(lookup func(string) (string, bool)) string {
	for _, name := range localeEnvVars {
		// Only an exactly empty value counts as unset, which is what libc
		// does. A whitespace-only LC_ALL is SET, and it names no installed
		// locale, so treating it as unset skipped past it to a usable
		// LC_CTYPE and declared the environment fine while libc had already
		// fallen back to ASCII.
		//
		// The value is returned untrimmed for the same reason: libc reads the
		// environment byte for byte. Normalizing it turned a padded spelling
		// into the installed one and suppressed the pin.
		if value, ok := lookup(name); ok && value != "" {
			return value
		}
	}
	return ""
}

// serverProbe is what AGM could establish about the tmux server on the
// configured socket. "bound but unidentified" is deliberately distinct from
// "absent": the first must not be pinned (TMUX-54 cannot prove it accepts
// -e), while the second is about to be created by this very binary.
type serverProbe struct {
	bound       bool
	identified  bool
	version     tmuxVersion
	utf8        bool
	localeKnown bool
}

// sessionLocaleDecision is the pure pin/do-not-pin rule, split out so every
// branch is unit-testable without a live tmux server.
func sessionLocaleDecision(locale string, server serverProbe, client tmuxVersion, clientKnown, clientUTF8 bool) []string {
	if locale == "" {
		return nil
	}
	pin := []string{"-e", "LC_ALL=", "-e", "LC_CTYPE=" + locale}
	if server.identified {
		if !server.version.atLeast(minSessionEnvVersion) {
			return nil
		}
		// Only a positively established ASCII server is pinned. An unreadable
		// environment is not evidence of ASCII, and blanking LC_ALL on a
		// guess would change collation, formatting and message language.
		if !server.localeKnown || server.utf8 {
			return nil
		}
		return pin
	}
	if server.bound {
		// Reachable but unidentifiable: pinning could fail every session on a
		// pre-3.2 server, so fail closed.
		return nil
	}
	// No server yet. AGM is about to start one from this binary, which will
	// inherit this process's environment, so the client answers both
	// questions: whether -e is supported, and whether a pin is needed at all.
	if !clientKnown || !client.atLeast(minSessionEnvVersion) || clientUTF8 {
		return nil
	}
	return pin
}

// SessionLocaleArgs returns the `new-session` arguments that pin a UTF-8
// LC_CTYPE into the session created on the server at socketPath, or nothing
// when pinning would be wrong or impossible. See sessionLocaleDecision.
func SessionLocaleArgs(socketPath string) []string {
	locale := pinnableLocale()
	if locale == "" {
		debug.Log("⚠️  no installed UTF-8 locale proven; leaving the session locale alone")
		return nil
	}
	ctx := context.Background()
	probe := serverProbe{}
	if version, ok := serverTmuxVersion(ctx, socketPath); ok {
		probe.bound = true
		probe.identified = true
		probe.version = version
		probe.utf8, probe.localeKnown = serverLocaleUsableUTF8(ctx, socketPath)
	} else {
		probe.bound = probeDialable(socketPath)
	}
	client, clientKnown := clientTmuxVersion()
	args := sessionLocaleDecision(locale, probe, client, clientKnown, localeIsUsableUTF8(os.LookupEnv))
	if len(args) == 0 {
		debug.Log("⚠️  not pinning a session locale (server bound=%v identified=%v utf8=%v)",
			probe.bound, probe.identified, probe.utf8)
	}
	return args
}

// clientTmuxVersion reports the version of the tmux binary AGM would exec.
func clientTmuxVersion() (tmuxVersion, bool) {
	raw, err := Version()
	if err != nil {
		return tmuxVersion{}, false
	}
	return parseTmuxVersion(raw)
}

// SessionLocalePinnable reports whether this host could pin a session locale
// at all. Tests whose subject is the pane environment skip when it is false.
func SessionLocalePinnable() bool {
	if pinnableLocale() == "" {
		return false
	}
	version, ok := clientTmuxVersion()
	return ok && version.atLeast(minSessionEnvVersion)
}

// newSessionArgs builds the argv for the `tmux new-session` command queue that
// creates an AGM session, including the session locale pin.
func newSessionArgs(socketPath string, identity SessionIdentity, workDir, sanitizedName string) []string {
	args := []string{"-S", socketPath,
		"new-session", "-d", "-P", "-F", "#{session_id}", "-s", identity.CreationName, "-c", workDir}
	args = append(args, SessionLocaleArgs(socketPath)...)
	return append(args,
		";", "set-option", "@agm_session_identity", identity.Token,
		";", "rename-session", sanitizedName)
}
