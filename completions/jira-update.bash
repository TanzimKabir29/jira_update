# bash completion for jira-update
# Install: source this file in your ~/.bashrc or drop it in /etc/bash_completion.d/

_jira_update() {
    local cur prev
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    case "$prev" in
        --output)
            COMPREPLY=($(compgen -W "json" -- "$cur"))
            return
            ;;
        --since)
            COMPREPLY=($(compgen -W "yesterday monday tuesday wednesday thursday friday saturday sunday 1h 2h 6h 1d 2d 7d" -- "$cur"))
            return
            ;;
        --log-n)
            # numeric — no completions
            return
            ;;
    esac

    COMPREPLY=($(compgen -W "
        --since
        --project
        --output
        --log
        --log-n
        --dry-run
        --reset
        --init
        --version
    " -- "$cur"))
}

complete -F _jira_update jira-update
