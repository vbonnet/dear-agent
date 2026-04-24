// Package workspace provides workspace detection and configuration management.
//
// It implements a 6-priority detection algorithm to determine the active
// workspace from flags, environment variables, git repository roots, or
// configuration file defaults. Workspaces carry tool-specific settings
// and output directory paths used by other packages.
package workspace
