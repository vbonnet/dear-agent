package cmd

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/engram/internal/config"
	"github.com/vbonnet/dear-agent/pkg/vcs"
)

var memoryVcsCmd = &cobra.Command{
	Use:   "vcs",
	Short: "Version control for memory files",
	Long: `Version control commands for tracking .ai.md and .why.md memory files.

Every change is committed to a git repository for auditability, reversibility,
and remote backup. Pre-commit hooks enforce .ai.md <-> .why.md pairing.

COMMANDS
  init      - Initialize VCS tracking for memory files
  status    - Show tracked file status
  log       - Show commit history
  push      - Push commits to remote
  restore   - Restore a file to a previous version
  validate  - Run validation checks on memory files
  backfill  - Import existing memory files into VCS tracking`,
}

var vcsInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize VCS tracking for memory files",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadVCSConfig()
		if err != nil {
			return err
		}

		m, err := vcs.New(cfg)
		if err != nil {
			return fmt.Errorf("init failed: %w", err)
		}
		defer m.Close()

		fmt.Printf("VCS tracking initialized\n")
		fmt.Printf("  Repo: %s\n", cfg.RepoPath)
		fmt.Printf("  Push: %s\n", cfg.PushStrategy)
		if cfg.RemoteURL != "" {
			fmt.Printf("  Remote: %s\n", cfg.RemoteURL)
		}
		fmt.Printf("  Validation: require_why_file=%v lint_on_commit=%v\n",
			cfg.Validation.RequireWhyFile, cfg.Validation.LintOnCommit)
		return nil
	},
}

var vcsStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show tracked file status",
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := loadVCS()
		if err != nil {
			return err
		}
		defer m.Close()

		status, err := m.Status()
		if err != nil {
			return err
		}

		if strings.TrimSpace(status) == "" {
			fmt.Println("Clean — all memory files are tracked and committed")
		} else {
			fmt.Println(status)
		}
		return nil
	},
}

var (
	vcsLogLimit int
)

var vcsLogCmd = &cobra.Command{
	Use:   "log [file]",
	Short: "Show commit history for memory files",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := loadVCS()
		if err != nil {
			return err
		}
		defer m.Close()

		var path string
		if len(args) > 0 {
			path = args[0]
		}

		entries, err := m.Log(path, vcsLogLimit)
		if err != nil {
			return err
		}

		if len(entries) == 0 {
			fmt.Println("No commits found")
			return nil
		}

		for _, e := range entries {
			fmt.Printf("%s  %s  %s\n", e.Hash[:8], e.Date[:10], e.Message)
		}
		return nil
	},
}

var vcsPushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push commits to remote",
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := loadVCS()
		if err != nil {
			return err
		}
		defer m.Close()

		if err := m.Push(); err != nil {
			return fmt.Errorf("push failed: %w", err)
		}
		fmt.Println("Push complete")
		return nil
	},
}

var vcsRestoreCmd = &cobra.Command{
	Use:   "restore <file> <commit-hash>",
	Short: "Restore a file to a previous version",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := loadVCS()
		if err != nil {
			return err
		}
		defer m.Close()

		if err := m.Restore(args[0], args[1]); err != nil {
			return fmt.Errorf("restore failed: %w", err)
		}
		fmt.Printf("Restored %s to %s\n", args[0], args[1][:8])
		return nil
	},
}

var vcsValidateCmd = &cobra.Command{
	Use:   "validate [files...]",
	Short: "Run validation checks on memory files",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := loadVCSConfig()
		if err != nil {
			return err
		}

		repoPath := expandHomePath(cfg.RepoPath)

		var files []string
		if len(args) > 0 {
			files = args
		} else {
			// Validate all staged files
			m, err := vcs.New(cfg)
			if err != nil {
				return err
			}
			defer m.Close()

			repo := m.Repo()
			if repo == nil {
				return fmt.Errorf("VCS not initialized — run 'engram memory vcs init'")
			}
			status, err := repo.Status()
			if err != nil {
				return err
			}
			for _, line := range strings.Split(status, "\n") {
				if len(line) > 3 {
					files = append(files, strings.TrimSpace(line[3:]))
				}
			}
		}

		pairErrors := vcs.ValidateMemoryPair(repoPath, files)
		fmErrors := vcs.ValidateEngramFrontmatter(repoPath, files)

		allErrors := append(pairErrors, fmErrors...)
		if len(allErrors) == 0 {
			fmt.Println("All validations passed")
			return nil
		}

		for _, e := range allErrors {
			fmt.Fprintf(os.Stderr, "ERROR: %s\n", e.Error())
		}
		return fmt.Errorf("%d validation error(s)", len(allErrors))
	},
}

var vcsBackfillCmd = &cobra.Command{
	Use:   "backfill <source-dir>",
	Short: "Import existing memory files into VCS tracking",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		m, err := loadVCS()
		if err != nil {
			return err
		}
		defer m.Close()

		repo := m.Repo()
		if repo == nil {
			return fmt.Errorf("VCS not initialized — run 'engram memory vcs init'")
		}

		sourceDir := args[0]
		var imported int

		// Walk source directory for .ai.md files
		entries, err := os.ReadDir(sourceDir)
		if err != nil {
			return fmt.Errorf("read source dir: %w", err)
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !strings.HasSuffix(name, ".ai.md") {
				continue
			}

			// Copy file to repo
			srcPath := sourceDir + "/" + name
			data, err := os.ReadFile(srcPath)
			if err != nil {
				log.Printf("WARN: skip %s: %v", name, err)
				continue
			}

			dstPath := repo.Dir() + "/" + name
			if err := os.WriteFile(dstPath, data, 0644); err != nil {
				log.Printf("WARN: skip %s: %v", name, err)
				continue
			}

			// Also copy companion .why.md if it exists
			whyName := strings.TrimSuffix(name, ".ai.md") + ".why.md"
			whySrc := sourceDir + "/" + whyName
			if whyData, err := os.ReadFile(whySrc); err == nil {
				whyDst := repo.Dir() + "/" + whyName
				_ = os.WriteFile(whyDst, whyData, 0644)
			}

			imported++
		}

		if imported == 0 {
			fmt.Println("No .ai.md files found to import")
			return nil
		}

		// Stage and commit all imported files
		if err := repo.StageFiles("."); err != nil {
			return fmt.Errorf("stage: %w", err)
		}
		hash, err := repo.Commit(fmt.Sprintf("vcs: backfill %d memory files", imported))
		if err != nil {
			return fmt.Errorf("commit: %w", err)
		}

		fmt.Printf("Backfilled %d memory files (commit %s)\n", imported, hash[:8])
		return nil
	},
}

func init() {
	memoryCmd.AddCommand(memoryVcsCmd)
	memoryVcsCmd.AddCommand(vcsInitCmd)
	memoryVcsCmd.AddCommand(vcsStatusCmd)
	memoryVcsCmd.AddCommand(vcsLogCmd)
	memoryVcsCmd.AddCommand(vcsPushCmd)
	memoryVcsCmd.AddCommand(vcsRestoreCmd)
	memoryVcsCmd.AddCommand(vcsValidateCmd)
	memoryVcsCmd.AddCommand(vcsBackfillCmd)

	vcsLogCmd.Flags().IntVarP(&vcsLogLimit, "limit", "n", 20, "Maximum number of entries to show")
}

// loadVCSConfig loads VCS configuration from engram config
func loadVCSConfig() (*vcs.Config, error) {
	loader := config.NewLoader()
	cfg, err := loader.Load()
	if err != nil {
		// Fall back to defaults if config loading fails
		return vcs.DefaultConfig(), nil
	}

	return &vcs.Config{
		Enabled:       cfg.VCS.Enabled,
		RepoPath:      cfg.VCS.RepoPath,
		PushStrategy:  cfg.VCS.PushStrategy,
		BatchInterval: cfg.VCS.BatchInterval,
		RemoteURL:     cfg.VCS.RemoteURL,
		RemoteName:    cfg.VCS.RemoteName,
		Branch:        cfg.VCS.Branch,
		Validation: vcs.ValidationConfig{
			RequireWhyFile: cfg.VCS.Validation.RequireWhyFile,
			LintOnCommit:   cfg.VCS.Validation.LintOnCommit,
		},
		OptIn: vcs.OptInConfig{
			ErrorMemory:     cfg.VCS.OptIn.ErrorMemory,
			EcphoryMetadata: cfg.VCS.OptIn.EcphoryMetadata,
			Logs:            cfg.VCS.OptIn.Logs,
		},
	}, nil
}

// loadVCS creates a MemoryVCS instance from configuration
func loadVCS() (*vcs.MemoryVCS, error) {
	cfg, err := loadVCSConfig()
	if err != nil {
		return nil, err
	}

	m, err := vcs.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("VCS init failed: %w", err)
	}
	return m, nil
}

// expandHomePath expands ~ in paths
func expandHomePath(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return home + path[1:]
	}
	return path
}
