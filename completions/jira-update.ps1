# PowerShell completion for jira-update
# Install: add the following line to your $PROFILE
#   . /path/to/completions/jira-update.ps1

Register-ArgumentCompleter -Native -CommandName jira-update -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)

    $flags = @(
        '--since',
        '--project',
        '--output',
        '--log',
        '--log-n',
        '--dry-run',
        '--reset',
        '--init',
        '--version'
    )

    $sinceValues = @(
        'yesterday', 'monday', 'tuesday', 'wednesday',
        'thursday', 'friday', 'saturday', 'sunday',
        '1h', '2h', '6h', '1d', '2d', '7d'
    )

    $tokens = $commandAst.CommandElements
    $prev = if ($tokens.Count -ge 2) { $tokens[$tokens.Count - 2].ToString() } else { '' }

    switch ($prev) {
        '--since' {
            $sinceValues | Where-Object { $_ -like "$wordToComplete*" } |
                ForEach-Object { [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_) }
            return
        }
        '--output' {
            [System.Management.Automation.CompletionResult]::new('json', 'json', 'ParameterValue', 'JSON output')
            return
        }
    }

    $flags | Where-Object { $_ -like "$wordToComplete*" } |
        ForEach-Object { [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterName', $_) }
}
