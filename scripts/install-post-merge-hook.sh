#!/usr/bin/env bash
#
# Install the post-merge worktree-sweep trigger into the hooks directory git
# actually consults — honouring core.hooksPath, and refusing to clobber a
# chezmoi-managed hooks dir.
#
# Why this is its own installer (not `agm admin install-hooks`): install-hooks
# manages Claude Code hooks under ~/.claude/hooks. This is a *git* hook, and on
# this host git's effective hooks dir is core.hooksPath (a repo-local
# .git/hooks would be silently bypassed). Getting that resolution right is the
# whole job — install to the wrong dir and the trigger never fires.
#
# Positive-guidance stance (.claude/CLAUDE.md principle 2): when this script
# cannot safely complete an install, it explains what it found and the exact
# right way to finish, rather than failing blindly or overwriting your work.

set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
hook_src="${repo_root}/scripts/git-hooks/post-merge"

if [ ! -f "${hook_src}" ]; then
  echo "Error: hook source not found at ${hook_src}" >&2
  exit 1
fi

# ── Resolve the hooks dir git will actually use ────────────────────────────
# Precedence mirrors git's own: core.hooksPath (if set) wins over .git/hooks.
hooks_dir="$(git config --get core.hooksPath || true)"
if [ -n "${hooks_dir}" ]; then
  # Expand a leading ~ and resolve a relative path against the repo root,
  # exactly as git interprets core.hooksPath.
  case "${hooks_dir}" in
    "~/"*) hooks_dir="${HOME}/${hooks_dir#\~/}" ;;
    /*) : ;;
    *) hooks_dir="${repo_root}/${hooks_dir}" ;;
  esac
  source_note="core.hooksPath = ${hooks_dir}"
else
  hooks_dir="$(git rev-parse --git-path hooks)"
  case "${hooks_dir}" in /*) : ;; *) hooks_dir="${repo_root}/${hooks_dir}" ;; esac
  source_note="repo-local hooks dir = ${hooks_dir}"
fi
hook_dst="${hooks_dir}/post-merge"

echo "Target: ${hook_dst}"
echo "        (${source_note})"
echo ""

# ── Refuse to clobber a chezmoi-managed hooks dir ──────────────────────────
# chezmoi owns the canonical copy of a managed file; a direct write here would
# silently drift from source and be reverted on the next `chezmoi apply`. Hand
# the install to chezmoi instead.
if command -v chezmoi >/dev/null 2>&1; then
  rel="${hooks_dir#"${HOME}"/}"
  if chezmoi managed 2>/dev/null | grep -qxF "${rel}" \
     || chezmoi managed 2>/dev/null | grep -qF "${rel}/post-merge"; then
    cat >&2 <<EOF
The effective hooks dir is managed by chezmoi:
  ${hooks_dir}

Not writing there directly — chezmoi would revert it on the next apply.
Finish the install through chezmoi so the hook is versioned in your dotfiles:

  chezmoi add --template=false "${hook_src}"   # or copy ${hook_src}
  cp "${hook_src}" "\$(chezmoi source-path "${hooks_dir}")/post-merge"
  chezmoi apply "${hook_dst}"

Then confirm it is executable:
  chmod +x "${hook_dst}"

(The canonical, reviewed copy lives in this repo at
 scripts/git-hooks/post-merge — keep the chezmoi copy in sync with it.)
EOF
    exit 3
  fi
fi

# ── Conflict check: never silently overwrite a foreign hook ────────────────
if [ -e "${hook_dst}" ]; then
  if cmp -s "${hook_src}" "${hook_dst}"; then
    echo "Already installed and up to date — nothing to do."
    exit 0
  fi
  cat >&2 <<EOF
A different post-merge hook already exists at:
  ${hook_dst}

Not overwriting it. To adopt this one, review the existing hook then either:
  • merge its logic with ${hook_src}, or
  • remove the existing hook and re-run this installer.

Tip: this repo's hook runs \`agm worktree sweep --execute\` only on the
default branch and exits 0 unconditionally, so it composes cleanly if you
append it to an existing post-merge hook.
EOF
  exit 1
fi

# ── Install ────────────────────────────────────────────────────────────────
mkdir -p "${hooks_dir}"
cp "${hook_src}" "${hook_dst}"
chmod +x "${hook_dst}"
echo "✓ Installed: ${hook_dst} -> agm worktree sweep --execute (default-branch merges only)"
echo ""
echo "It runs after \`git pull\`/\`git merge\` lands a PR on the default branch."
echo "Disable per-shell with: export AGM_POST_MERGE_SWEEP=0"
