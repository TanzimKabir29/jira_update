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

# =========================================================
# CONFIG
# =========================================================

JIRA_BASE_URL = os.environ.get("JIRA_BASE_URL", "").rstrip("/")
JIRA_EMAIL = os.environ.get("JIRA_EMAIL", "")
JIRA_API_TOKEN = os.environ.get("JIRA_API_TOKEN", "")

STATE_DIR = Path.home() / ".jira_update"
STATE_FILE = STATE_DIR / "state.json"


def validate_config():
    missing = [k for k, v in {
        "JIRA_BASE_URL": JIRA_BASE_URL,
        "JIRA_EMAIL": JIRA_EMAIL,
        "JIRA_API_TOKEN": JIRA_API_TOKEN,
    }.items() if not v]

    if missing:
        print("Error: the following required variables are not set in your .env file:")
        for key in missing:
            print(f"  {key}")
        print("\nCopy .env.example to .env and fill in your values.")
        sys.exit(1)


# =========================================================
# STATE MANAGEMENT
# =========================================================

def load_last_run():
    """
    Returns datetime of previous successful run.
    Falls back to last 24h if no state file exists.
    """

    if not STATE_FILE.exists():
        return datetime.now(timezone.utc) - timedelta(days=1)

    with open(STATE_FILE, "r") as f:
        data = json.load(f)

    return datetime.fromisoformat(data["last_run"])


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
    )

    if not response.ok:
        print("STATUS:", response.status_code)
        print("RESPONSE:")
        print(response.text)
        response.raise_for_status()

    return response.json()


def fetch_my_account_id():
    data = jira_get("/rest/api/3/myself")
    return data["accountId"]


def fetch_updated_issues(since):

    since_str = since.strftime("%Y-%m-%d %H:%M")

    jql = f'updated >= "{since_str}" ORDER BY updated ASC'

    issues = []

    next_page_token = None

    while True:

        params = {
            "jql": jql,
            "fields": "summary,status,assignee,comment",
            "maxResults": 100,
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


def format_time(s):
    dt = parse_jira_time(s)
    return dt.strftime("%Y-%m-%d %H:%M")

def fetch_issue_changelog(issue_key):

    data = jira_get(
        f"/rest/api/3/issue/{issue_key}/changelog"
    )

    return data.get("values", [])


# =========================================================
# ACTIVITY EXTRACTION
# =========================================================

def extract_relevant_activity(issue, since, account_id):
    """
    Extract only activity relevant to the current user.
    """

    issue_key = issue["key"]
    summary = issue["fields"]["summary"]

    relevant_events = []

    histories = fetch_issue_changelog(issue["key"])

    for history in histories:
        created = parse_jira_time(history["created"])

        items = history.get("items", [])

        if created < since:
            continue

        author = history["author"]["displayName"]

        for item in items:

            field = item["field"]

            # -------------------------------------------------
            # ASSIGNEE CHANGES
            # -------------------------------------------------

            if field == "assignee":

                from_account = item.get("from")
                to_account = item.get("to")

                if (
                    from_account == account_id
                    or to_account == account_id
                ):

                    from_name = item.get("fromString") or "Unassigned"
                    to_name = item.get("toString") or "Unassigned"

                    relevant_events.append({
                        "time": created,
                        "text": (
                            f"[{format_time(history['created'])}] "
                            f"{author} changed assignee "
                            f"from '{from_name}' to '{to_name}'"
                        )
                    })

            # -------------------------------------------------
            # STATUS CHANGES
            # Only include if currently assigned to you
            # OR was assigned to you during change
            # -------------------------------------------------

            elif field == "status":

                current_assignee = issue["fields"]["assignee"]

                assigned_to_me = (
                    current_assignee
                    and current_assignee["accountId"] == account_id
                )

                if assigned_to_me:

                    from_status = item.get("fromString")
                    to_status = item.get("toString")

                    relevant_events.append({
                        "time": created,
                        "text": (
                            f"[{format_time(history['created'])}] "
                            f"{author} changed status "
                            f"from '{from_status}' to '{to_status}'"
                        )
                    })

    # ---------------------------------------------------------
    # COMMENTS
    # ---------------------------------------------------------

    comments = issue["fields"]["comment"]["comments"]

    current_assignee = issue["fields"]["assignee"]

    assigned_to_me = (
        current_assignee
        and current_assignee["accountId"] == account_id
    )

    if assigned_to_me:

        for comment in comments:

            created = parse_jira_time(comment["created"])

            if created < since:
                continue

            author = comment["author"]["displayName"]

            relevant_events.append({
                "time": created,
                "text": (
                    f"[{format_time(comment['created'])}] "
                    f"{author} commented"
                )
            })

    # ---------------------------------------------------------
    # SORT EVENTS
    # ---------------------------------------------------------

    relevant_events.sort(key=lambda x: x["time"])

    return {
        "key": issue_key,
        "summary": summary,
        "events": relevant_events,
    }


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

    # Relative: 1d / 2h / 30m (case-insensitive)
    m = re.fullmatch(r'(\d+)(d|h|m)', value, re.IGNORECASE)
    if m:
        n, unit = int(m.group(1)), m.group(2).lower()
        if n == 0:
            raise ValueError(f"'0{m.group(2)}' is not a valid duration. Use a value greater than 0.")
        delta = {'d': timedelta(days=n), 'h': timedelta(hours=n), 'm': timedelta(minutes=n)}[unit]
        return datetime.now(timezone.utc) - delta

    # Time only: 9am, 9:30am, 14:30
    for fmt in ('%I%p', '%I:%M%p', '%H:%M'):
        try:
            t = datetime.strptime(value.upper(), fmt.upper())
            now = datetime.now(local_tz)
            result = now.replace(hour=t.hour, minute=t.minute, second=0, microsecond=0)
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
        "Accepted: 9am, 14:30, 30m, 2h, 1d, \"2026-05-30 14:00\", \"2026-05-30 14:00+06:00\""
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
    args = parser.parse_args()

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
    else:
        since = load_last_run()

    print("=" * 80)
    print(f"JIRA activity since {since.isoformat()}")
    print("=" * 80)
    print()

    issues = fetch_updated_issues(since)

    issue_activity = []

    for issue in issues:

        activity = extract_relevant_activity(issue, since, account_id)

        if activity["events"]:
            issue_activity.append(activity)

    # ---------------------------------------------------------
    # OUTPUT
    # ---------------------------------------------------------

    if not issue_activity:
        print("No relevant activity found.")

    else:
        for issue in issue_activity:

            print(f"{issue['key']} - {issue['summary']}")
            print("-" * 80)

            for event in issue["events"]:
                print(f"  • {event['text']}")

            print()

    save_last_run(datetime.now(timezone.utc))


if __name__ == "__main__":
    main()
