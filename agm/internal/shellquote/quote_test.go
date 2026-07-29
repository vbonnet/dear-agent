package shellquote

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

// Quote is the single shell-quoting primitive for the agm module. Before
// ce-93lw.1 three copies existed — here, in internal/agent, and in
// internal/session — and they had already drifted into two different escape
// styles ('"'"' versus '\''). The other two were deleted; this is the one that
// survives, so this is where its tests live.

func TestQuote(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", "simple", "'simple'"},
		{"with spaces", "with spaces", "'with spaces'"},
		{"single quote", "with'quote", `'with'"'"'quote'`},
		{"empty", "", "''"},
		{"path", "/home/user/work", "'/home/user/work'"},
		{"shell metacharacters are inert once quoted", "a;b|c&d$e`f", "'a;b|c&d$e`f'"},
		{"only a quote", "'", `''"'"''`},
		{"adjacent quotes", "''", `''"'"''"'"''`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := Quote(tt.input); got != tt.want {
				t.Errorf("Quote(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestQuoteRoundTripsThroughRealShell checks the property the golden
// strings above only approximate: whatever goes in must come back out as
// exactly one shell word, byte for byte. A hand-written expectation can encode
// a subtly wrong escape and still look right; /bin/sh cannot.
func TestQuoteRoundTripsThroughRealShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	t.Parallel()

	inputs := []string{
		"simple",
		"with spaces",
		"with'quote",
		"'",
		"''",
		`x'; touch pwned; echo '`,
		`$(id)`,
		"`id`",
		"a\nb",
		"back\\slash",
		"réunion — 会議",
		"",
	}

	for _, in := range inputs {
		t.Run(in, func(t *testing.T) {
			t.Parallel()

			// printf %s writes the argument with no interpretation, so stdout
			// is exactly the word the shell parsed.
			out, err := exec.Command("/bin/sh", "-c", "printf %s "+Quote(in)).Output()
			if err != nil {
				t.Fatalf("shell rejected Quote(%q) = %s: %v", in, Quote(in), err)
			}
			if string(out) != in {
				t.Errorf("round trip: shellquote.Quote(%q) parsed back as %q", in, string(out))
			}
		})
	}
}

// TestQuoteProducesOneWord guards the failure mode that matters most for
// callers: a value must never split into multiple arguments, however much
// whitespace or syntax it contains.
func TestQuoteProducesOneWord(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a POSIX shell")
	}
	t.Parallel()

	const hostile = `x'; touch pwned; echo '`

	// $# is the argument count after the shell has parsed the command line.
	out, err := exec.Command("/bin/sh", "-c", "set -- "+Quote(hostile)+`; printf %s "$#"`).Output()
	if err != nil {
		t.Fatalf("shell rejected quoted value: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != "1" {
		t.Errorf("Quote(%q) parsed as %s words, want 1", hostile, got)
	}

	if _, err := os.Stat("pwned"); err == nil {
		t.Error("INJECTION: quoted payload created ./pwned")
	}
}
