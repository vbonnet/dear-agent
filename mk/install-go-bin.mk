# Shared install helper. Included by the root Makefile and agm/Makefile so a
# single definition covers every install target in the repository -- the agm
# sub-Makefile previously had its own `install -m` path that bypassed this
# hardening entirely (ce-77ip.8).

# install-go-bin: install a freshly built binary into ~/go/bin so that macOS
# will actually execute it (ce-77ip.8).
#
# A bare `cp` rewrites the EXISTING inode in place. If that binary has already
# been executed, the kernel still holds the code-signing identity it cached for
# that vnode, so the new bytes fail validation and the process is SIGKILLed with
# OS_REASON_CODESIGNING *before main() runs* — no stderr, no log line, nothing.
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
