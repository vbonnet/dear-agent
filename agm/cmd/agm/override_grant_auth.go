package main

const unixFreshAuthenticationCommand = "/usr/bin/true"
const unixOperatorGrantInstaller = "/bin/sh"
const unixOperatorGrantInstallScript = `set -eu
umask 022
/usr/bin/tee "$1" >/dev/null
/bin/chmod 0644 "$1"`

func freshUnixAuthenticationArgs(nonInteractive bool) []string {
	// With a command, sudo -k ignores cached credentials and does not update
	// the credential cache after successful authentication.
	args := []string{"-k"}
	if nonInteractive {
		args = append(args, "-n")
	}
	return append(args, unixFreshAuthenticationCommand)
}

func operatorGrantInstallArgs(path string) []string {
	return []string{
		"-k",
		unixOperatorGrantInstaller,
		"-c",
		unixOperatorGrantInstallScript,
		"dear-agent-override-grant-installer",
		path,
	}
}
