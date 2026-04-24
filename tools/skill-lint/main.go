// Command skill-lint validates model/effort frontmatter on Claude Code
// skill markdown files. Exits with status 1 if any file is non-compliant.
//
// Usage:
//
//	skill-lint <dir> [<dir>...]
//	skill-lint -file <path>
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/vbonnet/dear-agent/pkg/skilllint"
)

func main() {
	var (
		singleFile string
	)
	flag.StringVar(&singleFile, "file", "", "lint a single file instead of directories")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: skill-lint <dir> [<dir>...]")
		fmt.Fprintln(os.Stderr, "       skill-lint -file <path>")
		flag.PrintDefaults()
	}
	flag.Parse()

	var violations []skilllint.Violation

	if singleFile != "" {
		vs, err := skilllint.CheckFile(singleFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "skill-lint:", err)
			os.Exit(2)
		}
		violations = append(violations, vs...)
	} else {
		dirs := flag.Args()
		if len(dirs) == 0 {
			flag.Usage()
			os.Exit(2)
		}
		for _, d := range dirs {
			vs, err := skilllint.CheckDir(d)
			if err != nil {
				fmt.Fprintf(os.Stderr, "skill-lint: %s: %v\n", d, err)
				os.Exit(2)
			}
			violations = append(violations, vs...)
		}
	}

	if len(violations) == 0 {
		return
	}
	for _, v := range violations {
		fmt.Fprintln(os.Stderr, v)
	}
	fmt.Fprintf(os.Stderr, "\n%d violation(s)\n", len(violations))
	os.Exit(1)
}
