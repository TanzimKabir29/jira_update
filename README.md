# jira-update

Prints all Jira activity relevant to you since the last time you ran it — assignee changes, status changes, and comments on your tickets. Useful for daily standup prep.

---

## Quick start

**First time only** — run the interactive setup to create your config:

```bash
jira-update --init
```

This prompts for your Jira URL, email, and API token and saves them to `~/.jira_update/.env`.

To generate an API token: Atlassian Account Settings → Security → API tokens.

---

## Install

### macOS / Linux (recommended)

Download the binary for your platform from the [latest release](https://github.com/TanzimKabir29/jira_update/releases/latest):

| File | Platform |
|------|----------|
| `jira-update-darwin-arm64` | macOS Apple Silicon |
| `jira-update-darwin-amd64` | macOS Intel |
| `jira-update-linux-amd64` | Linux |
| `jira-update-windows-amd64.exe` | Windows |

Move it onto your PATH:

```bash
mv jira-update-darwin-arm64 jira-update
chmod +x jira-update
mv jira-update /usr/local/bin/
```

Verify the download (optional):

```bash
# Download checksums.txt from the same release, then:
sha256sum -c checksums.txt
```

### Windows

Download `jira-update-windows-amd64.exe`, rename it to `jira-update.exe`, and place it in a folder on your `PATH`.

### Python (alternative)

Requires Python 3.11+ and [pipx](https://pipx.pypa.io/stable/installation/).

```bash
pipx install "git+https://github.com/TanzimKabir29/jira_update@v1.0.0#subdirectory=python"
```

Replace `v1.0.0` with the version you want to install.

---

## Usage

```
jira-update [flags]

Flags:
  --since TIME     Override start time. Accepted formats:
                     yesterday, monday–sunday
                     9am, 14:30, 3pm
                     1h, 6h, 2d
                     "2026-05-30 14:00"
                     "2026-05-30 14:00+06:00"
  --project KEYS   Filter to specific projects, e.g. PROJ or PROJ,OTHER
  --unassigned-qa  Show only tickets where you moved a status from a QA
                   column (any status containing "qa") to another status
  --assigned-qa    Reserved for future use
  --pm             Project-wide summary: status transition counts, unique
                   tickets moved, completed tickets, and team activity.
                   Requires --project. Mutually exclusive with the other
                   mode flags above.
  --output FORMAT  Output format: json
  --log [N]        Show run history (default 20 entries)
  --log-n N        Show last N entries of run history (0 = all)
  --dry-run        Run normally but do not update state or history
  --reset          Delete the state file and exit
  --init           Interactive setup: create ~/.jira_update/.env
  --version        Print version and exit
```

### Examples

```bash
# Activity since last run
jira-update

# Activity since 9am today
jira-update --since 9am

# Activity since last Monday, filtered to one project
jira-update --since monday --project PROJ

# Check what would show without advancing the state
jira-update --dry-run

# Machine-readable output
jira-update --output json

# Tickets you moved out of a QA column today
jira-update --since 1d --unassigned-qa

# PM summary for a project this week
jira-update --since monday --project PROJ --pm
```

---

## How it works

On each run the tool fetches all Jira issues updated since the previous run, then filters activity down to only what involves you:

- Tickets assigned to or unassigned from you
- Status changes on tickets where you were the assignee at the time of the change
- Comments on tickets where you were the assignee at the time of the comment

The timestamp of each run is saved to `~/.jira_update/state.json`. On first run it defaults to the last 24 hours.

---

## Shell completions

First, clone or download the repo to get the completion files, then follow the steps for your shell.

### zsh (macOS default)

```bash
mkdir -p ~/.zsh/completions
cp completions/jira-update.zsh ~/.zsh/completions/_jira-update
```

Add to `~/.zshrc` if not already present:

```zsh
fpath=(~/.zsh/completions $fpath)
autoload -Uz compinit && compinit
```

Then reload: `exec zsh`

### bash

Add to `~/.bashrc`:

```bash
source /path/to/completions/jira-update.bash
```

Then reload: `source ~/.bashrc`

### PowerShell

Add to your `$PROFILE`:

```powershell
. /path/to/completions/jira-update.ps1
```
