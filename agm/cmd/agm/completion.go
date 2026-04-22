package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func init() {
	// Replace cobra's default completion command with our own so we can:
	//   1. Skip PersistentPreRunE (no DB, health-check, or workspace detection)
	//   2. Support --cache to write the script to ~/.cache/ for fast shell startup
	rootCmd.CompletionOptions.DisableDefaultCmd = true
	rootCmd.AddCommand(newCompletionCmd())
}

func newCompletionCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:   "completion [shell]",
		Short: "Generate shell completion scripts",
		Long: `Generate shell completion scripts for agm.

Load immediately (slow — spawns agm on every shell start):
  source <(agm completion zsh)

Recommended: cache to file for fast startup:
  agm completion zsh --cache
  # Then in .zshrc:
  [[ -f ~/.cache/agm-completion.zsh ]] && source ~/.cache/agm-completion.zsh
  # Regenerate after upgrading agm:
  agm completion zsh --cache`,

		ValidArgs: []string{"bash", "fish", "powershell", "zsh"},
	}

	type shellDef struct {
		gen       func(root *cobra.Command, w *bytes.Buffer) error
		cacheFile string
	}

	shells := map[string]shellDef{
		"bash": {
			gen:       func(root *cobra.Command, w *bytes.Buffer) error { return root.GenBashCompletion(w) },
			cacheFile: "agm-completion.bash",
		},
		"zsh": {
			gen:       func(root *cobra.Command, w *bytes.Buffer) error { return root.GenZshCompletion(w) },
			cacheFile: "agm-completion.zsh",
		},
		"fish": {
			gen:       func(root *cobra.Command, w *bytes.Buffer) error { return root.GenFishCompletion(w, true) },
			cacheFile: "agm-completion.fish",
		},
		"powershell": {
			gen:       func(root *cobra.Command, w *bytes.Buffer) error { return root.GenPowerShellCompletionWithDesc(w) },
			cacheFile: "agm-completion.ps1",
		},
	}

	for shell, def := range shells {
		shell, def := shell, def
		sub := &cobra.Command{
			Use:   shell,
			Short: fmt.Sprintf("Generate %s completion script", shell),

			// Override PersistentPreRunE so cobra does not walk up to the root's
			// heavy initializer (Dolt, health-check, workspace detection). Completion
			// is a pure in-process operation; it needs none of that.
			PersistentPreRunE: func(*cobra.Command, []string) error { return nil },
			// Matching noop for PersistentPostRunE (audit logger, usage tracker).
			PersistentPostRunE: func(*cobra.Command, []string) error { return nil },

			RunE: func(cmd *cobra.Command, _ []string) error {
				cache, err := cmd.Flags().GetBool("cache")
				if err != nil {
					return err
				}

				var buf bytes.Buffer
				if err := def.gen(cmd.Root(), &buf); err != nil {
					return fmt.Errorf("generate %s completion: %w", shell, err)
				}

				if cache {
					return writeCompletionCache(def.cacheFile, buf.Bytes())
				}
				_, err = os.Stdout.Write(buf.Bytes())
				return err
			},
		}
		sub.Flags().Bool("cache", false,
			fmt.Sprintf("write completion to ~/.cache/%s instead of stdout", def.cacheFile))
		parent.AddCommand(sub)
	}

	return parent
}

func writeCompletionCache(filename string, content []byte) error {
	// Use XDG_CACHE_HOME / ~/.cache for shell completion scripts so the path
	// is the same on Linux and macOS (the macOS default ~/Library/Caches is
	// not on the standard shell search path convention).
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("locate home dir: %w", err)
	}
	cacheDir := filepath.Join(home, ".cache")
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		cacheDir = xdg
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil { //nolint:gosec // cacheDir is user-controlled XDG path
		return fmt.Errorf("create cache dir: %w", err)
	}
	// filepath.Base prevents any path traversal in the filename argument.
	dest := filepath.Join(cacheDir, filepath.Base(filename)) //nolint:gosec // path cleaned above
	// 0644 so the script is readable by subshells (completion files are not sensitive).
	if err := os.WriteFile(dest, content, 0o644); err != nil { //nolint:gosec // 0644 intentional for shell scripts
		return fmt.Errorf("write %s: %w", dest, err)
	}
	fmt.Fprintf(os.Stderr, "Completion written to %s\n", dest)
	fmt.Fprintf(os.Stderr, "Source it from your shell config: source %s\n", dest)
	return nil
}
