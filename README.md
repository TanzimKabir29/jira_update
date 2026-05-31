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

### Prerequisites
- Go 1.21+

### Install

```bash
cd go
go install .
```

This builds the binary and places it in `$GOPATH/bin`, which is on your PATH if you have a standard Go setup.

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

From a cloned repo:
```bash
pipx install ./python
```

From GitHub:
```bash
pipx install "git+<repo-url>#subdirectory=python"
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
