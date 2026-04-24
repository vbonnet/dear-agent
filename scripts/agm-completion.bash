# Bash completion for csm (Claude Session Manager)
# This completion script prevents file fallback and strips Cobra descriptions
#
# Installation:
#   1. Copy this file to: ~/.csm-completion.bash
#   2. Add to ~/.bashrc: source ~/.csm-completion.bash
#
# Or use the provided setup script:
#   ./scripts/setup-completion.sh

_csm_completion() {
    local cur="${COMP_WORDS[COMP_CWORD]}"
    local prev="${COMP_WORDS[COMP_CWORD-1]}"

    # Call csm's built-in Cobra completion
    local IFS=$'\n'
    local completions=($(csm __complete "${COMP_WORDS[@]:1}" 2>/dev/null))

    # Filter completions and strip descriptions
    COMPREPLY=()
    for comp in "${completions[@]}"; do
        # Skip directive lines (start with :)
        [[ $comp == :* ]] && continue
        # Skip debug/error lines (start with [)
        [[ $comp == \[* ]] && continue
        # Skip empty lines
        [[ -z $comp ]] && continue

        # Strip description (everything after tab character)
        # Cobra returns: "command\tDescription text"
        # We only want: "command"
        local completion="${comp%%$'\t'*}"

        # Only add completions that match the current prefix
        if [[ -z $cur ]] || [[ $completion == "$cur"* ]]; then
            COMPREPLY+=("$completion")
        fi
    done

    # Disable all forms of file completion
    # This prevents bash from suggesting files/directories when no matches exist
    compopt -o nospace 2>/dev/null || true
    compopt +o default 2>/dev/null || true
    compopt +o dirnames 2>/dev/null || true
    compopt +o filenames 2>/dev/null || true

    return 0
}

# Unregister any existing completion to avoid conflicts
complete -r csm 2>/dev/null || true

# Register completion function
complete -F _csm_completion -o nospace csm
