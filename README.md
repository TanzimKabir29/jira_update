# jira-update

Prints all Jira activity relevant to you since the last time you ran it — assignee changes, status changes, and comments on your tickets. Useful for daily standup prep.

## Configuration

Create `~/.jira_update/.env` using the template below:

```
JIRA_BASE_URL=https://your-company.atlassian.net
JIRA_EMAIL=you@your-company.com
JIRA_API_TOKEN=your_api_token_here
```

To generate an API token, go to: Atlassian Account Settings → Security → API tokens.

Your account ID is discovered automatically — you don't need to set it.

---

## Go

No Go installation required.

### Install

1. Download the binary for your platform from the [latest release](../../releases/latest):
   - `jira-update-darwin-arm64` — macOS Apple Silicon
   - `jira-update-darwin-amd64` — macOS Intel
   - `jira-update-linux-amd64` — Linux
   - `jira-update-windows-amd64.exe` — Windows

2. Rename it to `jira-update` and move it onto your PATH:
   ```bash
   mv jira-update-darwin-arm64 jira-update
   chmod +x jira-update
   mv jira-update /usr/local/bin/
   ```

### Run

```bash
jira-update
```

---

## Python

### Prerequisites
- Python 3.9+
- [pipx](https://pipx.pypa.io/stable/installation/)

### Install

```bash
pipx install "git+https://github.com/TanzimKabir29/jira_update@<tag>#subdirectory=python"
```

For example:
```bash
pipx install "git+https://github.com/TanzimKabir29/jira_update@v1.0.0#subdirectory=python"
```

### Run

```bash
jira-update
```

---

## How it works

On each run the tool fetches all Jira issues updated since the previous run, then filters the activity down to only what involves you:

- Tickets assigned to or from you
- Status changes on tickets currently assigned to you
- Comments on tickets currently assigned to you

The timestamp of each run is saved to `~/.jira_update/state.json`. On first run it defaults to the last 24 hours.

---

## Cutting a release

```bash
git tag v1.0.0
git push origin v1.0.0
```

GitHub Actions will build binaries for all platforms and publish them as a GitHub Release automatically.
