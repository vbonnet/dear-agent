package main

const unixOperatorGrantInstaller = "/bin/sh"
const unixOperatorGrantProbeInput = "AGM_AUTHENTICATION_PROBE\n"
const unixOperatorGrantInstallInput = "AGM_INSTALL_GRANT\n"
const unixOperatorGrantProbeExitCode = 42
const unixOperatorGrantInstallScript = `set -eu
IFS= read -r mode
if [ "$mode" = AGM_AUTHENTICATION_PROBE ]; then
	exit 42
fi
[ "$mode" = AGM_INSTALL_GRANT ] || exit 64
umask 022
/usr/bin/tee "$1" >/dev/null
/bin/chmod 0644 "$1"`

func operatorGrantInstallArgs(path string, nonInteractive bool) []string {
	args := []string{"-k"}
	if nonInteractive {
		args = append(args, "-n")
	}
	return append(args,
		unixOperatorGrantInstaller,
		"-c",
		unixOperatorGrantInstallScript,
		"dear-agent-override-grant-installer",
		path,
	)
}
