// Command workflow-roles inspects the role registry. Three subcommands
// matching the ROADMAP.md "roles list / describe / validate" surface:
//
//	workflow-roles list                         # one role per line
//	workflow-roles describe research            # full JSON dump of one role
//	workflow-roles validate ./roles.yaml        # parse + validate
//
// Resolution order (when the user does not pass --file): env var
// DEAR_AGENT_ROLES → ./.dear-agent/roles.yaml → ~/.config/dear-agent/roles.yaml
// → built-in defaults.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/vbonnet/dear-agent/pkg/workflow/roles"
)

func main() {
	os.Exit(run())
}

func run() int {
	if len(os.Args) < 2 {
		usage()
		return 2
	}
	sub := os.Args[1]
	args := os.Args[2:]
	switch sub {
	case "list":
		return cmdList(args)
	case "describe":
		return cmdDescribe(args)
	case "validate":
		return cmdValidate(args)
	default:
		usage()
		return 2
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: %s <subcommand> [flags] [args]

Subcommands:
  list                    list role names from the resolved registry
  describe <name>         dump one role as JSON
  validate <path>         parse and validate a roles.yaml file
`, os.Args[0])
}

func cmdList(args []string) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	file := fs.String("file", "", "path to roles.yaml (default: $DEAR_AGENT_ROLES → ./.dear-agent/roles.yaml → ~/.config/dear-agent/roles.yaml → built-in)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	reg, src, err := loadResolved(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load: %v\n", err)
		return 1
	}
	fmt.Printf("# source: %s\n", src)
	for _, name := range reg.RoleNames() {
		role := reg.Roles[name]
		fmt.Printf("%-15s %s\n", name, role.Description)
	}
	return 0
}

func cmdDescribe(args []string) int {
	fs := flag.NewFlagSet("describe", flag.ContinueOnError)
	file := fs.String("file", "", "path to roles.yaml")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "describe requires exactly one role name")
		return 2
	}
	reg, _, err := loadResolved(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load: %v\n", err)
		return 1
	}
	role, ok := reg.Lookup(fs.Arg(0))
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown role %q\n", fs.Arg(0))
		return 1
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(role); err != nil {
		fmt.Fprintf(os.Stderr, "encode: %v\n", err)
		return 1
	}
	return 0
}

func cmdValidate(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "validate requires exactly one path")
		return 2
	}
	if _, err := roles.LoadFile(args[0]); err != nil {
		fmt.Fprintf(os.Stderr, "INVALID  %s: %v\n", args[0], err)
		return 1
	}
	fmt.Printf("OK       %s\n", args[0])
	return 0
}

// loadResolved walks the standard resolution order. envPath is the
// explicit --file flag value (empty falls through to the env var).
func loadResolved(filePath string) (*roles.Registry, string, error) {
	if filePath != "" {
		reg, err := roles.LoadFile(filePath)
		if err != nil {
			return nil, filePath, err
		}
		return reg, filePath, nil
	}
	envPath := os.Getenv("DEAR_AGENT_ROLES")
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	return roles.AutoLoad(envPath, cwd, home)
}

