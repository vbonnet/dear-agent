# Launchd audit installer contract

**LDA-01** When the macOS override-audit installer is invoked through the approved privileged bootstrap, the system shall require root-owned non-writable non-symlink destination ancestry, stage and hash the executable and rendered LaunchDaemon, validate the exact staged plist, back up the complete live set, atomically activate the staged files, and restore the prior set on any failure or cancellation before completion.
