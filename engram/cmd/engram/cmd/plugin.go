package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/engram/cmd/engram/internal/cli"
	"github.com/vbonnet/dear-agent/engram/internal/config"
	"github.com/vbonnet/dear-agent/engram/internal/plugin"
)

var pluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Manage plugins",
	Long:  `Manage Engram plugins (list, enable, disable, info)`,
}

var pluginListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all loaded plugins",
	Long: `List all loaded plugins from configured plugin paths.

Shows plugin name, pattern (guidance/tool/connector), and description.

Example:
  engram plugin list
  engram plugin list --verbose`,
	RunE: runPluginList,
}

type pluginListConfig struct {
	Verbose bool
}

var pluginListCfg pluginListConfig

func init() {
	rootCmd.AddCommand(pluginCmd)
	pluginCmd.AddCommand(pluginListCmd)

	pluginListCmd.Flags().BoolVarP(&pluginListCfg.Verbose, "verbose", "v", false, "Show full details including permissions")
}

func runPluginList(cmd *cobra.Command, args []string) error {
	// Load configuration
	loader := config.NewLoader()
	cfg, err := loader.Load()
	if err != nil {
		return cli.ConfigNotFoundError("", err)
	}

	// Load plugins
	pluginLoader := plugin.NewLoader(cfg.Plugins.Paths, cfg.Plugins.Disabled)
	plugins, err := pluginLoader.Load()
	if err != nil {
		return cli.PluginLoadError("", err)
	}

	// Display results
	if len(plugins) == 0 {
		fmt.Println("No plugins found.")
		fmt.Printf("\nPlugin paths:\n")
		for _, path := range cfg.Plugins.Paths {
			fmt.Printf("  - %s\n", path)
		}
		return nil
	}

	fmt.Printf("Found %d plugin(s):\n\n", len(plugins))

	for i, p := range plugins {
		fmt.Printf("%d. %s (%s)\n", i+1, p.Manifest.Name, p.Manifest.Pattern)
		fmt.Printf("   Description: %s\n", p.Manifest.Description)
		fmt.Printf("   Version: %s\n", p.Manifest.Version)
		fmt.Printf("   Path: %s\n", p.Path)

		// Show commands
		if len(p.Manifest.Commands) > 0 {
			var cmdNames []string
			for _, cmd := range p.Manifest.Commands {
				cmdNames = append(cmdNames, cmd.Name)
			}
			fmt.Printf("   Commands: %s\n", strings.Join(cmdNames, ", "))
		}

		// Show EventBus subscriptions
		if len(p.Manifest.EventBus.Subscribe) > 0 {
			fmt.Printf("   EventBus: subscribes to %s\n", strings.Join(p.Manifest.EventBus.Subscribe, ", "))
		}

		// Show permissions in verbose mode
		if pluginListCfg.Verbose {
			fmt.Printf("   Permissions:\n")
			if len(p.Manifest.Permissions.Filesystem) > 0 {
				fmt.Printf("     Filesystem: %s\n", strings.Join(p.Manifest.Permissions.Filesystem, ", "))
			}
			if len(p.Manifest.Permissions.Network) > 0 {
				fmt.Printf("     Network: %s\n", strings.Join(p.Manifest.Permissions.Network, ", "))
			}
			if len(p.Manifest.Permissions.Commands) > 0 {
				fmt.Printf("     Commands: %s\n", strings.Join(p.Manifest.Permissions.Commands, ", "))
			}
		}

		fmt.Println()
	}

	// Show disabled plugins if any
	if len(cfg.Plugins.Disabled) > 0 {
		fmt.Printf("Disabled plugins: %s\n", strings.Join(cfg.Plugins.Disabled, ", "))
	}

	return nil
}
