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
[ ! -L "$1" ] || exit 73
umask 022
tmp=""
cleanup() {
	if [ -n "$tmp" ]; then
		/bin/rm -f -- "$tmp"
	fi
}
trap cleanup EXIT HUP INT TERM
tmp=$(/usr/bin/mktemp "$1.tmp.XXXXXX")
/usr/bin/tee "$tmp" >/dev/null
/bin/chmod 0644 "$tmp"
/bin/mv -f -- "$tmp" "$1"
tmp=""`

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
