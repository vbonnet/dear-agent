# Shared install helper. Included by the root Makefile and agm/Makefile so a
# single definition covers every install target in the repository -- the agm
# sub-Makefile previously had its own `install -m` path that bypassed this
# hardening entirely (ce-77ip.8).

# install-go-bin: install a freshly built binary into ~/go/bin so that macOS
# will actually execute it (ce-77ip.8).
#
# A bare `cp` rewrites the EXISTING inode in place. If that binary has already
# been executed, the kernel may still hold the code-signing identity it cached
# for that vnode, so the new bytes fail validation and the process is SIGKILLed
# with OS_REASON_CODESIGNING *before main() runs* — no stderr, no log line.
#
# This is a RACE, not a certainty: whether the kill happens depends on the
# cached signature still being live for that vnode. Measured on this host, a
# bare cp over an already-executed binary was killed 1 time in 30; staging and
# renaming was killed 0 times in 30. Intermittency is what made the original
# incident so hard to attribute — a rebuild usually works, so the failure looks
# like something else entirely.
# For a launchd-run binary that has no terminal, the only visible symptom is
# that the job silently stops working. On 2026-07-19 this disabled the OAuth
# token-refresher for 17 hours and was misread as a dead token family.
#
# Staging to a temp path and renaming gives the binary a NEW inode, so no stale
# cdhash can be cached against it, and makes the swap atomic — a launchd tick
# firing mid-install can never observe a half-written file. The explicit
# ad-hoc codesign then covers the installed bytes rather than relying on the
# linker-signed signature surviving the copy.
#
# codesign is macOS-only; failures are tolerated so Linux builds stay green.
# Correctness rests on the rename, which is portable.
#
# The staging path is unique per invocation, via mktemp in the destination
# directory. Two worktrees or automation jobs installing the same binary at
# once would otherwise share one <name>.tmp: each could codesign or rename it
# while the other was still copying, installing a mixed binary — or the second
# mv would fail because the first already consumed the path.
#
# mktemp rather than $$ (the shell PID): $$ is NOT reliably unique, because in
# a subshell it expands to the PARENT shell's pid, so parallel installs driven
# from one shell would still collide. mktemp is unique by construction. It
# creates the file in the destination directory, which keeps the final mv on
# the same filesystem and therefore atomic. Copying into that freshly created
# inode is safe: it has never been executed, so no cdhash is cached for it.
#
# The whole sequence runs in ONE shell so the trap can remove the staging file
# if any step fails, rather than leaving debris in ~/go/bin.
#
# What NOT to write, and what to write instead:
#
# Retired form, shown so the failure mode is documented -- do NOT use it.
# RETIRED-EXAMPLE (this marker is what authorises the line below; deleting the
# warning deletes the exemption with it):
#   cp bin/token-refresher /usr/local/bin/token-refresher
# Correct form:
#   stage=$(mktemp /usr/local/bin/tr.XXXXXX) && cp bin/token-refresher "$stage" \
#     && chmod 755 "$stage" && mv -f "$stage" /usr/local/bin/token-refresher
#
# (The retired line above is allowlisted in
# internal/deploy/testdata/rawcopy-allowlist.txt so this warning can show the
# form it warns about.)
#
# Usage: $(call install-go-bin,bin/<name>[,<dest-dir>])
# dest-dir defaults to ~/go/bin; pass it for hooks and other install roots.
define install-go-bin
	@set -e; \
	dest='$(if $(2),$(2),$(HOME)/go/bin)/$(notdir $(1))'; \
	stage="$$(mktemp "$$dest.XXXXXX")"; \
	trap 'rm -f "$$stage"' EXIT; \
	cp '$(1)' "$$stage"; \
	chmod 755 "$$stage"; \
	codesign -f -s - "$$stage" 2>/dev/null || true; \
	mv -f "$$stage" "$$dest"; \
	echo "Installed: $$dest"
endef
