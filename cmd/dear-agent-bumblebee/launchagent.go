package main

// launchagent.go — install/uninstall/inspect the per-user macOS LaunchAgent
// at ~/Library/LaunchAgents/com.dear-agent.bumblebee.plist.
//
// Why LaunchAgent (per-user), not LaunchDaemon (root): Bumblebee inventories
// the developer endpoint — npm/PyPI lockfiles, MCP configs, IDE & browser
// extensions — and "per-user" is the right scope for that. Running as root
// would broaden scope unnecessarily and create a service we'd have to reason
// about under sudo.
//
// Idempotent. Re-running install refreshes the plist (path substitutions,
// schedule) and reloads the agent.

import (
	_ "embed"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// launchAgentLabel matches the plist <key>Label</key> string.
const launchAgentLabel = "com.dear-agent.bumblebee"

// plistTemplate is the source-of-truth LaunchAgent definition, embedded at
// build time. The template carries two placeholders, substituted at install:
//
//	__USER_HOME__         absolute path to $HOME
//	__BUMBLEBEE_BINARY__  absolute path to this binary (used for `scan`)
//
//go:embed templates/com.dear-agent.bumblebee.plist
var plistTemplate string

func runInstallLaunchAgent(args []string) error {
	fs := flag.NewFlagSet("install-launchagent", flag.ContinueOnError)
	uninstall := fs.Bool("uninstall", false, "remove the LaunchAgent")
	status := fs.Bool("status", false, "show launchctl state and exit")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if runtime.GOOS != "darwin" {
		return errors.New("LaunchAgent is macOS-only. On Linux, use systemd --user or cron")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve $HOME: %w", err)
	}
	target := filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist")
	domain := fmt.Sprintf("gui/%d", os.Getuid())

	switch {
	case *status:
		return statusLaunchAgent(domain)
	case *uninstall:
		return uninstallLaunchAgent(domain, target)
	default:
		return installLaunchAgent(home, domain, target)
	}
}

func statusLaunchAgent(domain string) error {
	cmd := exec.Command("launchctl", "print", domain+"/"+launchAgentLabel)
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("%s is not loaded in %s\n", launchAgentLabel, domain)
		return errors.New("not loaded")
	}
	// Match the bash precursor: show the first ~20 lines, which is the
	// header launchctl prints (Label/state/PID/etc.).
	lines := strings.SplitN(string(out), "\n", 21)
	if len(lines) == 21 {
		lines = lines[:20]
	}
	fmt.Println(strings.Join(lines, "\n"))
	return nil
}

func uninstallLaunchAgent(domain, target string) error {
	// bootout is the load-counterpart of bootstrap. Returns non-zero if the
	// service wasn't loaded, which is fine on uninstall — swallow it.
	_ = exec.Command("launchctl", "bootout", domain+"/"+launchAgentLabel).Run()

	if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
		fmt.Printf("Not installed at %s (nothing to do)\n", target)
		return nil
	}
	if err := os.Remove(target); err != nil {
		return fmt.Errorf("remove %s: %w", target, err)
	}
	fmt.Printf("Removed %s\n", target)
	return nil
}

func installLaunchAgent(home, domain, target string) error {
	binPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve self path: %w", err)
	}
	binPath, err = filepath.EvalSymlinks(binPath)
	if err != nil {
		// EvalSymlinks failure is non-fatal (e.g. binary served from a path
		// that isn't a symlink); fall back to the raw executable path.
		binPath, _ = os.Executable()
	}

	// Ensure log dir exists — launchd will not create intermediate directories
	// for StandardOutPath/StandardErrorPath.
	logDir := filepath.Join(home, "Library", "Logs", "dear-agent")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", logDir, err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(target), err)
	}

	rendered := renderPlist(plistTemplate, home, binPath)
	// 0o600: per-user LaunchAgent plist contains the absolute path to the
	// installed binary. Other local users have no need to read it; tightening
	// to user-only also clears gosec G306. launchd is happy at 0o600 — it
	// runs as the owning user and only requires read access for that uid.
	if err := os.WriteFile(target, []byte(rendered), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}

	// Reload: bootout the old (if any), bootstrap the new. bootstrap fails if
	// the label is already loaded, so the bootout-first sequence is required
	// for idempotent reinstall.
	_ = exec.Command("launchctl", "bootout", domain+"/"+launchAgentLabel).Run()
	if out, err := exec.Command("launchctl", "bootstrap", domain, target).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl bootstrap %s %s: %w\n%s", domain, target, err, out)
	}

	fmt.Printf("Installed %s\n", launchAgentLabel)
	fmt.Printf("  plist:   %s\n", target)
	fmt.Println("  runs:    daily at 04:00 local")
	fmt.Printf("  logs:    %s/bumblebee.log\n", logDir)
	fmt.Printf("  output:  %s/Library/Application Support/dear-agent/bumblebee/<date>.ndjson\n\n", home)
	fmt.Printf("Run on demand:   launchctl kickstart %s/%s\n", domain, launchAgentLabel)
	fmt.Println("Uninstall:       dear-agent-bumblebee install-launchagent --uninstall")
	return nil
}

// renderPlist substitutes the two placeholders in the embedded template.
// Pulled out for testability.
func renderPlist(template, userHome, binPath string) string {
	r := strings.NewReplacer(
		"__USER_HOME__", userHome,
		"__BUMBLEBEE_BINARY__", binPath,
	)
	return r.Replace(template)
}
