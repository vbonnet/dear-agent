package config

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// FuzzConfigUnmarshal feeds random bytes to the YAML config parser.
// The parser must never panic on any input -- it should return errors gracefully.
func FuzzConfigUnmarshal(f *testing.F) {
	// Valid YAML config seed
	f.Add([]byte(`sessions_dir: /tmp/sessions
log_level: info
storage:
  mode: dotfile
timeout:
  tmux_commands: 5s
  enabled: true
`))
	// Minimal
	f.Add([]byte(`log_level: debug`))
	// Empty
	f.Add([]byte(""))
	// Invalid YAML
	f.Add([]byte("{{{{"))
	// Deeply nested
	f.Add([]byte("a:\n  b:\n    c:\n      d: value\n"))
	// Large value
	f.Add([]byte("sessions_dir: " + string(make([]byte, 4096)) + "\n"))
	// Binary-like content
	f.Add([]byte{0xff, 0xfe, 0x00, 0x01, 0x02})

	f.Fuzz(func(t *testing.T, data []byte) {
		cfg := Default()
		// Must never panic -- errors are acceptable
		_ = yaml.Unmarshal(data, cfg)

		// Also exercise validate on whatever was parsed
		_ = validate(cfg)
	})
}

// FuzzExpandHome feeds random paths to the tilde-expansion function.
func FuzzExpandHome(f *testing.F) {
	f.Add("~/test/path")
	f.Add("~")
	f.Add("")
	f.Add("/absolute/path")
	f.Add("relative/path")
	f.Add("~user/path")
	f.Add(string([]byte{0x7e, 0x2f, 0x00})) // ~/\x00

	f.Fuzz(func(t *testing.T, path string) {
		// Must never panic
		_ = expandHome(path)
	})
}
