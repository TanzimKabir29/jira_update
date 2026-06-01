import os
import json
import re
import sys
import argparse
import requests
from datetime import datetime, timedelta, timezone
from pathlib import Path
from dotenv import load_dotenv

load_dotenv(Path.home() / ".jira_update" / ".env")

__version__ = "dev"

_RELATIVE_RE = re.compile(r'(\d+)(d|h|m)', re.IGNORECASE)

# =========================================================
# CONFIG
# =========================================================

JIRA_BASE_URL = os.environ.get("JIRA_BASE_URL", "").rstrip("/")
JIRA_EMAIL = os.environ.get("JIRA_EMAIL", "")
JIRA_API_TOKEN = os.environ.get("JIRA_API_TOKEN", "")

STATE_DIR = Path.home() / ".jira_update"
STATE_FILE = STATE_DIR / "state.json"

_MAX_RESULTS = 100


def validate_config():
    missing = [k for k, v in {
        "JIRA_BASE_URL": JIRA_BASE_URL,
        "JIRA_EMAIL": JIRA_EMAIL,
        "JIRA_API_TOKEN": JIRA_API_TOKEN,
    }.items() if not v]

    if missing:
        print("Error: the following required variables are not set in your .env file:", file=sys.stderr)
        for key in missing:
            print(f"  {key}", file=sys.stderr)
        print("\nCopy .env.example to .env and fill in your values.", file=sys.stderr)
        sys.exit(1)


# =========================================================
# STATE MANAGEMENT
# =========================================================

def load_last_run():
    """
    Returns datetime of previous successful run.
    Falls back to last 24h if no state file exists or is corrupt.
    """

    if not STATE_FILE.exists():
        return datetime.now(timezone.utc) - timedelta(days=1)

    try:
        with open(STATE_FILE, "r") as f:
            data = json.load(f)
        return datetime.fromisoformat(data["last_run"])
    except (json.JSONDecodeError, KeyError, ValueError):
        print("Warning: state file is corrupt, defaulting to last 24 hours.", file=sys.stderr)
        return datetime.now(timezone.utc) - timedelta(days=1)


def save_last_run(dt):
    STATE_DIR.mkdir(parents=True, exist_ok=True)
    with open(STATE_FILE, "w") as f:
        json.dump({"last_run": dt.isoformat()}, f)


# =========================================================
# JIRA API
# =========================================================

def jira_get(path, params=None):

    url = f"{JIRA_BASE_URL}{path}"

    response = requests.get(
        url,
        auth=(JIRA_EMAIL, JIRA_API_TOKEN),
        headers={
            "Accept": "application/json",
        },
        params=params,
        timeout=30,
    )

    if not response.ok:
        print("STATUS:", response.status_code, file=sys.stderr)
        print("RESPONSE:", file=sys.stderr)
        print(response.text, file=sys.stderr)
        response.raise_for_status()

    return response.json()


def fetch_my_account_id():
    data = jira_get("/rest/api/3/myself")
    account_id = data.get("accountId")
    if not account_id:
        print("Error: could not determine your Jira account ID.", file=sys.stderr)
        sys.exit(1)
    return account_id


def fetch_updated_issues(since, projects=None):

    since_str = since.strftime("%Y-%m-%d %H:%M")

    jql = f'assignee was currentUser() AND updated >= "{since_str}"'
    if projects:
        keys = ", ".join(projects)
        jql += f" AND project in ({keys})"
    jql += " ORDER BY updated ASC"

    issues = []

    next_page_token = None

    while True:

        params = {
            "jql": jql,
            "fields": "summary,status,assignee,comment",
            "maxResults": _MAX_RESULTS,
        }

        if next_page_token:
            params["nextPageToken"] = next_page_token

        data = jira_get(
            "/rest/api/3/search/jql",
            params=params,
        )

        issues.extend(data.get("issues", []))

        next_page_token = data.get("nextPageToken")

        if not next_page_token:
            break

    return issues


# =========================================================
# HELPERS
# =========================================================

def parse_jira_time(s):
    return datetime.fromisoformat(s.replace("Z", "+00:00"))


def format_time(dt):
    return dt.strftime("%Y-%m-%d %H:%M")

def fetch_issue_changelog(issue_key):
    all_values = []
    start_at = 0

    while True:
        data = jira_get(
            f"/rest/api/3/issue/{issue_key}/changelog",
            params={"startAt": start_at, "maxResults": _MAX_RESULTS},
        )
        values = data.get("values", [])
        all_values.extend(values)
        if data.get("isLast", True):
            break
        start_at += len(values)

    return all_values


# =========================================================
# ACTIVITY EXTRACTION
# =========================================================

def extract_relevant_activity(issue, since, account_id):
    """
    Extract only activity relevant to the current user.
    Shows events where the user was the assignee at the time of the event.
    """

    issue_key = issue["key"]
    summary = issue["fields"]["summary"]

    histories = fetch_issue_changelog(issue["key"])

    # Sort all changelog entries chronologically
    sorted_histories = sorted(histories, key=lambda h: parse_jira_time(h["created"]))

    # Build assignee timeline: list of (time, account_id_or_none)
    # Infer initial assignee from the 'from' field of the first assignee change.
    # If no changes exist, the current assignee has always been the assignee.
    assignee_timeline = []
    initial_assignee = None
    found_first_change = False

    for h in sorted_histories:
        for item in h.get("items", []):
            if item["field"] == "assignee":
                if not found_first_change:
                    initial_assignee = item.get("from")
                    found_first_change = True
                assignee_timeline.append((parse_jira_time(h["created"]), item.get("to")))

    if not found_first_change:
        current = issue["fields"]["assignee"]
        initial_assignee = current["accountId"] if current else None

    def was_assigned_at(t):
        current = initial_assignee
        for change_time, to_id in assignee_timeline:
            if change_time <= t:
                current = to_id
            else:
                break
        return current == account_id

    # ---------------------------------------------------------
    # CHANGELOG EVENTS
    # ---------------------------------------------------------

    relevant_events = []

    for history in sorted_histories:
        created = parse_jira_time(history["created"])

        if created < since:
            continue

        author = history["author"]["displayName"]

        for item in history.get("items", []):

            field = item["field"]

            if field == "assignee":
                from_account = item.get("from")
                to_account = item.get("to")

                if from_account == account_id or to_account == account_id:
                    from_name = item.get("fromString") or "Unassigned"
                    to_name = item.get("toString") or "Unassigned"
                    relevant_events.append({
                        "time": created,
                        "text": (
                            f"[{format_time(created)}] "
                            f"{author} changed assignee "
                            f"from '{from_name}' to '{to_name}'"
                        )
                    })

            elif field == "status":
                if was_assigned_at(created):
                    relevant_events.append({
                        "time": created,
                        "text": (
                            f"[{format_time(created)}] "
                            f"{author} changed status "
                            f"from '{item.get('fromString')}' to '{item.get('toString')}'"
                        )
                    })

    # ---------------------------------------------------------
    # COMMENTS
    # ---------------------------------------------------------

    for comment in issue["fields"].get("comment", {}).get("comments", []):
        created = parse_jira_time(comment["created"])
        if created < since:
            continue
        if was_assigned_at(created):
            relevant_events.append({
                "time": created,
                "text": (
                    f"[{format_time(created)}] "
                    f"{comment['author']['displayName']} commented"
                )
            })

    relevant_events.sort(key=lambda x: x["time"])

    return {
        "key": issue_key,
        "summary": summary,
        "events": relevant_events,
    }


# =========================================================
# HISTORY
# =========================================================

HISTORY_FILE = STATE_DIR / "history.json"


def append_history(source, since_type, since_value):
    STATE_DIR.mkdir(parents=True, exist_ok=True)
    entry = {
        "time": datetime.now(timezone.utc).isoformat(),
        "source": source,
        "since_type": since_type,
        "since_value": since_value,
    }
    with open(HISTORY_FILE, "a") as f:
        f.write(json.dumps(entry) + "\n")


def print_history(limit):
    if not HISTORY_FILE.exists():
        print("No run history found.")
        return

    with open(HISTORY_FILE, "r") as f:
        lines = [line.strip() for line in f if line.strip()]

    entries = []
    for line in lines:
        try:
            entries.append(json.loads(line))
        except json.JSONDecodeError:
            continue

    entries.reverse()  # most recent first

    if limit > 0:
        entries = entries[:limit]

    if not entries:
        print("No run history found.")
        return

    local_tz = datetime.now().astimezone().tzinfo

    print(f"{'#':>3}  {'Time':<16}  {'Source':<6}  Since")
    print(f"{'--':>3}  {'----------------':<16}  {'------':<6}  {'---------------------------'}")

    for i, entry in enumerate(entries, 1):
        run_time = datetime.fromisoformat(entry["time"]).astimezone(local_tz)
        time_str = run_time.strftime("%Y-%m-%d %H:%M")
        source = entry.get("source", "?")
        since_type = entry.get("since_type", "?")
        since_value = entry.get("since_value", "")

        if since_type == "arg":
            since_str = f"arg: {since_value}"
        else:
            try:
                state_time = datetime.fromisoformat(since_value).astimezone(local_tz)
                since_str = f"state: {state_time.strftime('%Y-%m-%d %H:%M')}"
            except (ValueError, TypeError):
                since_str = f"state: {since_value}"

        print(f"{i:>3}  {time_str:<16}  {source:<6}  {since_str}")


# =========================================================
# INIT
# =========================================================

def run_init():
    env_file = STATE_DIR / ".env"
    print("Jira Update — interactive setup")
    print(f"Values will be saved to {env_file}")
    print()

    base_url = input("JIRA_BASE_URL (e.g. https://your-company.atlassian.net): ").strip().rstrip("/")
    email = input("JIRA_EMAIL: ").strip()
    token = input("JIRA_API_TOKEN: ").strip()

    if not base_url or not email or not token:
        print("Error: all three values are required.", file=sys.stderr)
        sys.exit(1)

    STATE_DIR.mkdir(parents=True, exist_ok=True)
    with open(env_file, "w") as f:
        f.write(f"JIRA_BASE_URL={base_url}\nJIRA_EMAIL={email}\nJIRA_API_TOKEN={token}\n")
    env_file.chmod(0o600)

    print()
    print(f"Config saved to {env_file}")


# =========================================================
# ARGUMENT PARSING
# =========================================================

def parse_since_arg(value):
    """
    Parse a --since argument into a UTC-aware datetime.
    Accepted formats:
      Relative : 1d, 2d, 6h
      Time only: 9am, 9:30am, 14:30  (today, local time)
      Full date : 2026-05-30 14:00    (local time assumed)
      With tz   : 2026-05-30 14:00+06:00
    """
    local_tz = datetime.now().astimezone().tzinfo
    value = value.strip()

    # Natural language: yesterday, monday–sunday
    lower = value.lower()
    if lower == "yesterday":
        now = datetime.now(local_tz)
        return (now - timedelta(days=1)).replace(hour=0, minute=0, second=0, microsecond=0).astimezone(timezone.utc)

    _WEEKDAYS = ["monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"]
    if lower in _WEEKDAYS:
        target = _WEEKDAYS.index(lower)
        now = datetime.now(local_tz)
        today = now.weekday()
        days_back = (today - target) % 7
        if days_back == 0:
            days_back = 7  # last occurrence, not today
        return (now - timedelta(days=days_back)).replace(hour=0, minute=0, second=0, microsecond=0).astimezone(timezone.utc)

    # Relative: 1d / 2h / 30m (case-insensitive)
    m = _RELATIVE_RE.fullmatch(value)
    if m:
        n, unit = int(m.group(1)), m.group(2).lower()
        if n == 0:
            raise ValueError(f"'0{m.group(2)}' is not a valid duration. Use a value greater than 0.")
        delta = {'d': timedelta(days=n), 'h': timedelta(hours=n), 'm': timedelta(minutes=n)}[unit]
        return datetime.now(timezone.utc) - delta

    # Time only: 9am, 9:30am, 14:30
    for fmt in ('%I%p', '%I:%M%p', '%H:%M'):
        try:
            t = datetime.strptime(value.upper(), fmt)
            now = datetime.now(local_tz)
            result = now.replace(hour=t.hour, minute=t.minute, second=0, microsecond=0)
            if result > now:
                result -= timedelta(days=1)
            return result.astimezone(timezone.utc)
        except ValueError:
            continue

    # Full datetime — fromisoformat handles both with and without timezone
    for fmt in ('%Y-%m-%d %H:%M', '%Y-%m-%dT%H:%M'):
        try:
            dt = datetime.strptime(value, fmt)
            return dt.replace(tzinfo=local_tz).astimezone(timezone.utc)
        except ValueError:
            continue

    # ISO with timezone offset: 2026-05-30 14:00+06:00
    try:
        dt = datetime.fromisoformat(value.replace(' ', 'T'))
        if dt.tzinfo is None:
            dt = dt.replace(tzinfo=local_tz)
        return dt.astimezone(timezone.utc)
    except ValueError:
        pass

    raise ValueError(
        f"Unrecognized time format: {value!r}\n"
        "Accepted: yesterday, monday–sunday, 9am, 14:30, 30m, 2h, 1d, \"2026-05-30 14:00\", \"2026-05-30 14:00+06:00\""
    )


# =========================================================
# MAIN
# =========================================================

def main():

    parser = argparse.ArgumentParser(
        description="Show Jira activity since last run or a given time."
    )
    parser.add_argument(
        '--since',
        metavar='TIME',
        help='Override start time. Accepted: 9am, 14:30, 2h, 1d, "2026-05-30 14:00", "2026-05-30 14:00+06:00"',
    )
    parser.add_argument(
        '--project',
        metavar='KEYS',
        help='Comma-separated project keys to filter results (e.g. PROJ or PROJ,OTHER)',
    )
    parser.add_argument(
        '--log',
        nargs='?',
        const=20,
        type=int,
        metavar='N',
        help='Show run history. Optionally specify number of entries (default 20, 0 = all)',
    )
    parser.add_argument(
        '--dry-run',
        action='store_true',
        help='Run normally but do not update state or history',
    )
    parser.add_argument(
        '--reset',
        action='store_true',
        help='Delete the state file and exit',
    )
    parser.add_argument(
        '--output',
        metavar='FORMAT',
        help='Output format: "json" for machine-readable output',
    )
    parser.add_argument(
        '--version',
        action='version',
        version=__version__,
    )
    parser.add_argument(
        '--init',
        action='store_true',
        help='Interactive setup: create ~/.jira_update/.env',
    )
    args = parser.parse_args()

    if args.init:
        run_init()
        return

    if args.reset:
        if STATE_FILE.exists():
            STATE_FILE.unlink()
            print("State file removed.")
        else:
            print("No state file found.")
        return

    if args.log is not None:
        print_history(args.log)
        return

    validate_config()

    account_id = fetch_my_account_id()

    if args.since:
        try:
            since = parse_since_arg(args.since)
        except ValueError as e:
            print(f"Error: {e}", file=sys.stderr)
            sys.exit(1)
        if since > datetime.now(timezone.utc):
            print("Error: --since time cannot be in the future.", file=sys.stderr)
            sys.exit(1)
        since_type, since_value = "arg", args.since
    else:
        since = load_last_run()
        since_type, since_value = "state", since.isoformat()

    if args.output != "json":
        print("=" * 80)
        print(f"JIRA activity since {since.isoformat()}")
        print("=" * 80)
        print()

    projects = [k.strip().upper() for k in args.project.split(",")] if args.project else None
    issues = fetch_updated_issues(since, projects=projects)

    issue_activity = []
    has_error = False

    for issue in issues:
        try:
            activity = extract_relevant_activity(issue, since, account_id)
        except Exception as e:
            print(f"Warning: skipping {issue['key']}: {e}", file=sys.stderr)
            has_error = True
            continue

        if activity["events"]:
            issue_activity.append(activity)

    # ---------------------------------------------------------
    # OUTPUT
    # ---------------------------------------------------------

    if args.output == "json":
        out = [
            {
                "key": iss["key"],
                "summary": iss["summary"],
                "events": [e["text"] for e in iss["events"]],
            }
            for iss in issue_activity
        ]
        print(json.dumps(out, indent=2))

    else:
        if not issue_activity:
            print("No relevant activity found.")
        else:
            for issue in issue_activity:

                print(f"{issue['key']} - {issue['summary']}")
                print("-" * 80)

                for event in issue["events"]:
                    print(f"  • {event['text']}")

                print()

    if has_error:
        print(
            "Warning: some issues could not be processed. "
            "State not updated to avoid missing activity on next run.",
            file=sys.stderr,
        )
    elif args.dry_run:
        print("[dry-run] state and history not updated", file=sys.stderr)
    else:
        now = datetime.now(timezone.utc)
        save_last_run(now)
        append_history("python", since_type, since_value)


if __name__ == "__main__":
    main()
