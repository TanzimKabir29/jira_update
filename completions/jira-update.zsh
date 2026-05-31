#compdef jira-update
# zsh completion for jira-update
# Install: place this file somewhere in your $fpath (e.g. /usr/local/share/zsh/site-functions/)

_jira_update() {
    local -a args
    args=(
        '--since[Override start time]:time:(yesterday monday tuesday wednesday thursday friday saturday sunday 1h 2h 6h 1d 2d 7d)'
        '--project[Filter by project key(s)]:keys:'
        '--output[Output format]:format:(json)'
        '--log[Show run history (last 20 entries)]'
        '--log-n[Show last N entries of run history]:n:'
        '--dry-run[Run without updating state or history]'
        '--reset[Delete the state file and exit]'
        '--init[Interactive setup: create ~/.jira_update/.env]'
        '--version[Print version and exit]'
    )
    _arguments -s $args
}

_jira_update "$@"
