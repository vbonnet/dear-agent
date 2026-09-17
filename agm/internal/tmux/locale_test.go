package tmux

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func envLookup(pairs map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		value, ok := pairs[key]
		return value, ok
	}
}

// tmux documents the rule precisely: the FIRST of LC_ALL, LC_CTYPE, LANG that
// is *set* decides. A later UTF-8 variable does not rescue an earlier
// non-UTF-8 one, which is why the session pin empties LC_ALL instead of
// trusting LC_CTYPE to win.
func TestLocaleIsUTF8FollowsTmuxPrecedence(t *testing.T) {
	tests := map[string]struct {
		env  map[string]string
		want bool
	}{
		"nothing set":               {env: map[string]string{}, want: false},
		"LANG utf8":                 {env: map[string]string{"LANG": "en_US.UTF-8"}, want: true},
		"LANG lowercase utf-8":      {env: map[string]string{"LANG": "en_us.utf-8"}, want: true},
		"LANG utf8 no dash":         {env: map[string]string{"LANG": "en_US.UTF8"}, want: true},
		"LANG C":                    {env: map[string]string{"LANG": "C"}, want: false},
		"LC_CTYPE wins over LANG":   {env: map[string]string{"LC_CTYPE": "C", "LANG": "en_US.UTF-8"}, want: false},
		"LC_ALL wins over LC_CTYPE": {env: map[string]string{"LC_ALL": "C", "LC_CTYPE": "en_US.UTF-8"}, want: false},
		"LC_ALL utf8":               {env: map[string]string{"LC_ALL": "C.UTF-8", "LANG": "C"}, want: true},
		"empty LC_ALL is set":       {env: map[string]string{"LC_ALL": ""}, want: false},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.want, localeIsUTF8(envLookup(test.env)))
		})
	}
}

// The pane-side rule differs from tmux's on exactly one case, and the session
// pin depends on that difference: POSIX ignores an EMPTY LC_ALL and falls
// through to LC_CTYPE, while tmux's client check treats "set" as decisive.
func TestLocaleIsUTF8ForShellIgnoresEmptyLCALL(t *testing.T) {
	shadowed := map[string]string{"LC_ALL": "C", "LC_CTYPE": "en_US.UTF-8"}
	assert.False(t, localeIsUTF8ForShell(envLookup(shadowed)), "a non-empty LC_ALL=C still shadows LC_CTYPE")

	neutralised := map[string]string{"LC_ALL": "", "LC_CTYPE": "en_US.UTF-8"}
	assert.True(t, localeIsUTF8ForShell(envLookup(neutralised)),
		"an emptied LC_ALL must fall through to the pinned LC_CTYPE")
	assert.False(t, localeIsUTF8(envLookup(neutralised)),
		"tmux's own client rule stops at LC_ALL because it is set; the two rules are deliberately different")

	assert.False(t, localeIsUTF8ForShell(envLookup(map[string]string{"LC_ALL": "", "LC_CTYPE": ""})),
		"emptying everything leaves no UTF-8 guarantee")
}

// The codeset sits before an optional @modifier, so a name that does not end
// in the codeset can still be a UTF-8 locale.
func TestLocaleNameIsUTF8HandlesModifiers(t *testing.T) {
	for _, name := range []string{"en_US.UTF-8", "en_US.utf8", "sr_RS.UTF-8@latin", "ca_ES.UTF-8@valencia", "C.UTF-8"} {
		assert.True(t, localeNameIsUTF8(name), "expected %q to be UTF-8", name)
	}
	for _, name := range []string{"C", "POSIX", "en_US", "en_US.ISO8859-1", "", "@latin"} {
		assert.False(t, localeNameIsUTF8(name), "expected %q to not be UTF-8", name)
	}
}

func TestPickUTF8LocalePrefersPortableNames(t *testing.T) {
	assert.Equal(t, "C.UTF-8", pickUTF8Locale([]string{"C", "POSIX", "en_US.UTF-8", "C.UTF-8"}))
	assert.Equal(t, "en_US.UTF-8", pickUTF8Locale([]string{"C", "fr_FR.UTF-8", "en_US.UTF-8"}))
	assert.Equal(t, "fr_FR.UTF-8", pickUTF8Locale([]string{"C", "POSIX", "fr_FR.UTF-8"}))
	assert.Equal(t, "sr_RS.UTF-8@latin", pickUTF8Locale([]string{"C", "sr_RS.UTF-8@latin"}),
		"a modified UTF-8 locale is still usable for LC_CTYPE")
	assert.Equal(t, "", pickUTF8Locale([]string{"C", "POSIX"}), "no UTF-8 locale installed")
}

// Failing to enumerate locales is not proof that none exists, but it is also
// not licence to pin a guess: an unloadable name makes shells print "cannot
// change locale" into the pane, which is the corruption this code prevents.
// So an unenumerable host pins nothing, and so does one that enumerates
// without finding a UTF-8 locale.
func TestResolveSessionLocaleRequiresProofOfInstallation(t *testing.T) {
	assert.Equal(t, "", resolveSessionLocale(func() ([]string, bool) { return nil, false }),
		"an unenumerable host must not pin a guessed locale")
	assert.Equal(t, "", resolveSessionLocale(func() ([]string, bool) { return []string{"C", "POSIX"}, true }),
		"enumerated with no UTF-8 locale must stay empty")
	assert.Equal(t, "C.UTF-8", resolveSessionLocale(func() ([]string, bool) { return []string{"C", "C.UTF-8"}, true }))
}

// new-session -e first exists in tmux 3.2. Passing it to an older server fails
// session creation outright, so the version gate is load-bearing rather than
// cosmetic.
func TestParseTmuxVersionAndSessionEnvGate(t *testing.T) {
	tests := map[string]struct {
		raw       string
		want      tmuxVersion
		parses    bool
		supported bool
	}{
		"release with letter": {raw: "tmux 3.7c", want: tmuxVersion{3, 7}, parses: true, supported: true},
		"plain":               {raw: "tmux 3.2", want: tmuxVersion{3, 2}, parses: true, supported: true},
		"just below":          {raw: "tmux 3.1b", want: tmuxVersion{3, 1}, parses: true, supported: false},
		"documented floor":    {raw: "tmux 2.6", want: tmuxVersion{2, 6}, parses: true, supported: false},
		"next build":          {raw: "tmux next-3.4", want: tmuxVersion{3, 4}, parses: true, supported: true},
		"major bump":          {raw: "tmux 4.0", want: tmuxVersion{4, 0}, parses: true, supported: true},
		"garbage":             {raw: "tmux unknown", parses: false},
		"empty":               {raw: "", parses: false},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, ok := parseTmuxVersion(test.raw)
			assert.Equal(t, test.parses, ok)
			if !test.parses {
				return
			}
			assert.Equal(t, test.want, got)
			assert.Equal(t, test.supported, got.atLeast(minSessionEnvVersion))
		})
	}
}

// The pin decision has four ways to say "leave it alone" and only one way to
// say "pin", and three of the four were found by review rather than by these
// tests, so each branch is pinned explicitly here.
func TestSessionLocaleDecision(t *testing.T) {
	const locale = "C.UTF-8"
	pin := []string{"-e", "LC_ALL=", "-e", "LC_CTYPE=C.UTF-8"}
	modern := tmuxVersion{3, 2}
	old := tmuxVersion{3, 1}

	tests := map[string]struct {
		locale      string
		server      serverProbe
		client      tmuxVersion
		clientKnown bool
		clientUTF8  bool
		want        []string
	}{
		"identified modern ASCII server pins": {
			locale: locale, server: serverProbe{bound: true, identified: true, version: modern, localeKnown: true},
			client: modern, clientKnown: true, want: pin,
		},
		"identified server already UTF-8 is left alone": {
			locale: locale, server: serverProbe{bound: true, identified: true, version: modern, utf8: true, localeKnown: true},
			client: modern, clientKnown: true, want: nil,
		},
		"identified pre-3.2 server gets no -e": {
			locale: locale, server: serverProbe{bound: true, identified: true, version: old, localeKnown: true},
			client: modern, clientKnown: true, want: nil,
		},
		// A live server whose version cannot be read must not inherit the
		// client's version: after a client upgrade that would send -e to a
		// server that rejects it and fail every session.
		// An unreadable server environment is not evidence of ASCII; blanking
		// LC_ALL on that guess would change collation and message language.
		"identified server with unreadable locale fails closed": {
			locale: locale, server: serverProbe{bound: true, identified: true, version: modern, localeKnown: false},
			client: modern, clientKnown: true, want: nil,
		},
		"bound but unidentified server fails closed": {
			locale: locale, server: serverProbe{bound: true},
			client: modern, clientKnown: true, want: nil,
		},
		"no server pins using the client version": {
			locale: locale, server: serverProbe{},
			client: modern, clientKnown: true, want: pin,
		},
		"no server and an old client gets no -e": {
			locale: locale, server: serverProbe{},
			client: old, clientKnown: true, want: nil,
		},
		"no server and an unknown client fails closed": {
			locale: locale, server: serverProbe{}, clientKnown: false, want: nil,
		},
		// The server AGM is about to start inherits this environment, so an
		// already-UTF-8 AGM needs no pin and must not have LC_ALL blanked.
		"no server but AGM is already UTF-8 is left alone": {
			locale: locale, server: serverProbe{},
			client: modern, clientKnown: true, clientUTF8: true, want: nil,
		},
		"no installed locale never pins": {
			locale: "", server: serverProbe{bound: true, identified: true, version: modern, localeKnown: true},
			client: modern, clientKnown: true, want: nil,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := sessionLocaleDecision(test.locale, test.server, test.client, test.clientKnown, test.clientUTF8)
			assert.Equal(t, test.want, got)
		})
	}
}

// A locale name can be syntactically UTF-8 and still unloadable, which is the
// case where "leave it alone" would be exactly wrong: the pane inherits a
// locale the C library cannot select, prints setlocale warnings and behaves as
// ASCII. So the preserve-existing checks require the inherited name to be
// installed, not merely to look like UTF-8.
func TestLocaleIsUsableUTF8RequiresInstallation(t *testing.T) {
	installed, enumerated := installedLocales()
	if !enumerated {
		t.Skip("host cannot enumerate locales")
	}
	real := pickUTF8Locale(installed)
	if real == "" {
		t.Skip("host has no installed UTF-8 locale")
	}

	assert.True(t, localeIsUsableUTF8(envLookup(map[string]string{"LC_ALL": real})),
		"an installed UTF-8 locale is usable")
	assert.False(t, localeIsUsableUTF8(envLookup(map[string]string{"LC_ALL": "zz_ZZ.UTF-8"})),
		"a syntactically UTF-8 name that is not installed must not count as usable")
	assert.False(t, localeIsUsableUTF8(envLookup(map[string]string{"LC_ALL": "C"})),
		"a non-UTF-8 locale is not usable")
	assert.False(t, localeIsUsableUTF8(envLookup(map[string]string{})),
		"an empty environment is not usable")
	assert.True(t, localeIsUsableUTF8(envLookup(map[string]string{"LC_ALL": "", "LC_CTYPE": real})),
		"an emptied LC_ALL still falls through to an installed LC_CTYPE")
}

func TestEffectiveLocaleNameSkipsEmptyValues(t *testing.T) {
	assert.Equal(t, "en_US.UTF-8", effectiveLocaleName(envLookup(map[string]string{"LC_ALL": "", "LC_CTYPE": "en_US.UTF-8"})))
	assert.Equal(t, "C", effectiveLocaleName(envLookup(map[string]string{"LC_ALL": "C", "LC_CTYPE": "en_US.UTF-8"})))
	assert.Equal(t, "", effectiveLocaleName(envLookup(map[string]string{})))
}
