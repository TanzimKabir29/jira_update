package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// =========================================================
// CONFIG
// Config is loaded from ~/.jira_update/.env
// Copy .env.example from the repo root to that location.
// =========================================================

var version = "dev"

const maxResults = 100

var (
	jiraBaseURL  string
	jiraEmail    string
	jiraAPIToken string
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

func loadEnv() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	envFile := filepath.Join(home, ".jira_update", ".env")
	f, err := os.Open(envFile)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		_ = os.Setenv(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
	}
}

func validateConfig() {
	jiraBaseURL = strings.TrimRight(os.Getenv("JIRA_BASE_URL"), "/")
	jiraEmail = os.Getenv("JIRA_EMAIL")
	jiraAPIToken = os.Getenv("JIRA_API_TOKEN")

	var missing []string
	if jiraBaseURL == "" {
		missing = append(missing, "JIRA_BASE_URL")
	}
	if jiraEmail == "" {
		missing = append(missing, "JIRA_EMAIL")
	}
	if jiraAPIToken == "" {
		missing = append(missing, "JIRA_API_TOKEN")
	}

	if len(missing) > 0 {
		home, _ := os.UserHomeDir()
		fmt.Fprintf(os.Stderr, "Error: missing required config variables: %s\n", strings.Join(missing, ", "))
		fmt.Fprintf(os.Stderr, "Copy .env.example to %s and fill in your values.\n",
			filepath.Join(home, ".jira_update", ".env"))
		os.Exit(1)
	}
}

// =========================================================
// INIT
// =========================================================

func runInit() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error determining home directory:", err)
		os.Exit(1)
	}
	dir := filepath.Join(home, ".jira_update")
	envFile := filepath.Join(dir, ".env")

	reader := bufio.NewReader(os.Stdin)

	prompt := func(label, placeholder string) string {
		fmt.Printf("%s [%s]: ", label, placeholder)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			return placeholder
		}
		return line
	}

	fmt.Println("Jira Update — interactive setup")
	fmt.Println("Values will be saved to", envFile)
	fmt.Println()

	baseURL := prompt("JIRA_BASE_URL (e.g. https://your-company.atlassian.net)", "")
	email := prompt("JIRA_EMAIL", "")
	token := prompt("JIRA_API_TOKEN", "")

	if baseURL == "" || email == "" || token == "" {
		fmt.Fprintln(os.Stderr, "Error: all three values are required.")
		os.Exit(1)
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		fmt.Fprintln(os.Stderr, "Error creating config directory:", err)
		os.Exit(1)
	}

	contents := fmt.Sprintf("JIRA_BASE_URL=%s\nJIRA_EMAIL=%s\nJIRA_API_TOKEN=%s\n",
		strings.TrimRight(baseURL, "/"), email, token)

	if err := os.WriteFile(envFile, []byte(contents), 0600); err != nil {
		fmt.Fprintln(os.Stderr, "Error writing .env file:", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("Config saved to", envFile)
}

// =========================================================
// STATE MANAGEMENT
// =========================================================

func statePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".jira_update", "state.json")
}

func loadLastRun() time.Time {
	data, err := os.ReadFile(statePath())
	if err != nil {
		return time.Now().UTC().Add(-24 * time.Hour)
	}
	var s struct {
		LastRun time.Time `json:"last_run"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		fmt.Fprintln(os.Stderr, "Warning: state file is corrupt, defaulting to last 24 hours.")
		return time.Now().UTC().Add(-24 * time.Hour)
	}
	return s.LastRun
}

func saveLastRun(t time.Time) {
	path := statePath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		fmt.Fprintln(os.Stderr, "Warning: could not create state directory:", err)
		return
	}
	data, _ := json.Marshal(struct {
		LastRun time.Time `json:"last_run"`
	}{LastRun: t})
	if err := os.WriteFile(path, data, 0644); err != nil {
		fmt.Fprintln(os.Stderr, "Warning: could not save state:", err)
	}
}

// =========================================================
// JIRA API
// =========================================================

func jiraGet(path string, params url.Values) ([]byte, error) {
	req, err := http.NewRequest("GET", jiraBaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(jiraEmail, jiraAPIToken)
	req.Header.Set("Accept", "application/json")
	if params != nil {
		req.URL.RawQuery = params.Encode()
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("jira API %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func fetchMyAccountID() (string, error) {
	body, err := jiraGet("/rest/api/3/myself", nil)
	if err != nil {
		return "", err
	}
	var me struct {
		AccountID string `json:"accountId"`
	}
	if err := json.Unmarshal(body, &me); err != nil {
		return "", err
	}
	if me.AccountID == "" {
		return "", fmt.Errorf("could not determine account ID from /rest/api/3/myself response")
	}
	return me.AccountID, nil
}

type Issue struct {
	Key    string `json:"key"`
	Fields struct {
		Summary  string `json:"summary"`
		Assignee *struct {
			AccountID   string `json:"accountId"`
			DisplayName string `json:"displayName"`
		} `json:"assignee"`
		Comment struct {
			Comments []struct {
				Created string `json:"created"`
				Author  struct {
					DisplayName string `json:"displayName"`
				} `json:"author"`
			} `json:"comments"`
		} `json:"comment"`
	} `json:"fields"`
}

func fetchUpdatedIssues(since time.Time, projects []string, unassignedQA bool) ([]Issue, error) {
	var jql string
	if unassignedQA {
		jql = fmt.Sprintf(`status changed by currentUser() after "%s"`, since.Format("2006-01-02 15:04"))
	} else {
		jql = fmt.Sprintf(`assignee was currentUser() AND updated >= "%s"`, since.Format("2006-01-02 15:04"))
	}
	if len(projects) > 0 {
		jql += fmt.Sprintf(` AND project in (%s)`, strings.Join(projects, ", "))
	}
	jql += " ORDER BY updated ASC"

	var issues []Issue
	nextPageToken := ""

	for {
		params := url.Values{
			"jql":        {jql},
			"fields":     {"summary,status,assignee,comment"},
			"maxResults": {strconv.Itoa(maxResults)},
		}
		if nextPageToken != "" {
			params.Set("nextPageToken", nextPageToken)
		}

		body, err := jiraGet("/rest/api/3/search/jql", params)
		if err != nil {
			return nil, err
		}

		var result struct {
			Issues        []Issue `json:"issues"`
			NextPageToken string  `json:"nextPageToken"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, err
		}

		issues = append(issues, result.Issues...)
		if result.NextPageToken == "" {
			break
		}
		nextPageToken = result.NextPageToken
	}
	return issues, nil
}

type ChangelogItem struct {
	Field      string `json:"field"`
	From       string `json:"from"`
	FromString string `json:"fromString"`
	To         string `json:"to"`
	ToString   string `json:"toString"`
}

type ChangelogHistory struct {
	Created string `json:"created"`
	Author  struct {
		AccountID   string `json:"accountId"`
		DisplayName string `json:"displayName"`
	} `json:"author"`
	Items []ChangelogItem `json:"items"`
}

func fetchChangelog(issueKey string) ([]ChangelogHistory, error) {
	var all []ChangelogHistory
	startAt := 0

	for {
		params := url.Values{
			"startAt":    {strconv.Itoa(startAt)},
			"maxResults": {strconv.Itoa(maxResults)},
		}
		body, err := jiraGet("/rest/api/3/issue/"+issueKey+"/changelog", params)
		if err != nil {
			return nil, err
		}
		var result struct {
			Values []ChangelogHistory `json:"values"`
			IsLast bool               `json:"isLast"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, err
		}
		all = append(all, result.Values...)
		if result.IsLast {
			break
		}
		startAt += len(result.Values)
	}
	return all, nil
}

// =========================================================
// HISTORY
// =========================================================

type HistoryEntry struct {
	Time       time.Time `json:"time"`
	Source     string    `json:"source"`
	SinceType  string    `json:"since_type"`
	SinceValue string    `json:"since_value"`
}

func historyPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".jira_update", "history.json")
}

func appendHistory(source, sinceType, sinceValue string) {
	path := historyPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	entry := HistoryEntry{
		Time:       time.Now().UTC(),
		Source:     source,
		SinceType:  sinceType,
		SinceValue: sinceValue,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.Write(append(data, '\n'))
}

func printHistory(limit int) {
	data, err := os.ReadFile(historyPath())
	if err != nil {
		fmt.Println("No run history found.")
		return
	}

	var entries []HistoryEntry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var e HistoryEntry
		if err := json.Unmarshal([]byte(line), &e); err == nil {
			entries = append(entries, e)
		}
	}

	// Most recent first
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}

	// Apply limit (0 = all)
	if limit > 0 && limit < len(entries) {
		entries = entries[:limit]
	}

	if len(entries) == 0 {
		fmt.Println("No run history found.")
		return
	}

	fmt.Printf("%3s  %-16s  %-6s  %s\n", "#", "Time", "Source", "Since")
	fmt.Printf("%3s  %-16s  %-6s  %s\n", "--", "----------------", "------", "---------------------------")

	for i, e := range entries {
		timeStr := e.Time.In(time.Local).Format("2006-01-02 15:04")

		var sinceStr string
		if e.SinceType == "arg" {
			sinceStr = "arg: " + e.SinceValue
		} else {
			if t, err := time.Parse(time.RFC3339, e.SinceValue); err == nil {
				sinceStr = "state: " + t.In(time.Local).Format("2006-01-02 15:04")
			} else {
				sinceStr = "state: " + e.SinceValue
			}
		}

		fmt.Printf("%3d  %-16s  %-6s  %s\n", i+1, timeStr, e.Source, sinceStr)
	}
}

// =========================================================
// HELPERS
// =========================================================

func parseJiraTime(s string) (time.Time, error) {
	// Jira omits the colon in timezone offsets (e.g. +0200 instead of +02:00).
	// Insert it so the string is valid RFC3339.
	if n := len(s); n >= 5 {
		if c := s[n-5]; c == '+' || c == '-' {
			s = s[:n-2] + ":" + s[n-2:]
		}
	}
	s = strings.Replace(s, "Z", "+00:00", 1)
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}

func fmtTime(t time.Time) string {
	return t.Format("2006-01-02 15:04")
}

// =========================================================
// ACTIVITY EXTRACTION
// =========================================================

type Event struct {
	Time time.Time
	Text string
}

func extractRelevantActivity(issue Issue, since time.Time, accountID string, unassignedQA bool) ([]Event, error) {
	var events []Event

	histories, err := fetchChangelog(issue.Key)
	if err != nil {
		return nil, err
	}

	if unassignedQA {
		for _, h := range histories {
			created, err := parseJiraTime(h.Created)
			if err != nil || created.Before(since) {
				continue
			}
			if h.Author.AccountID != accountID {
				continue
			}
			for _, item := range h.Items {
				if item.Field == "status" && strings.Contains(strings.ToLower(item.FromString), "qa") {
					events = append(events, Event{
						Time: created,
						Text: fmt.Sprintf("[%s] %s changed status from '%s' to '%s'",
							fmtTime(created), h.Author.DisplayName, item.FromString, item.ToString),
					})
				}
			}
		}
		sort.Slice(events, func(i, j int) bool {
			return events[i].Time.Before(events[j].Time)
		})
		return events, nil
	}

	// Sort histories chronologically to build an accurate assignee timeline.
	sort.Slice(histories, func(i, j int) bool {
		ti, _ := parseJiraTime(histories[i].Created)
		tj, _ := parseJiraTime(histories[j].Created)
		return ti.Before(tj)
	})

	// Build assignee timeline: list of (changeTime, newAccountID) pairs.
	type assigneeChange struct {
		at      time.Time
		toID    string
	}
	var timeline []assigneeChange
	initialAssignee := ""
	foundFirstChange := false

	for _, h := range histories {
		for _, item := range h.Items {
			if item.Field == "assignee" {
				if !foundFirstChange {
					initialAssignee = item.From
					foundFirstChange = true
				}
				t, err := parseJiraTime(h.Created)
				if err != nil {
					continue
				}
				timeline = append(timeline, assigneeChange{at: t, toID: item.To})
			}
		}
	}

	if !foundFirstChange {
		if issue.Fields.Assignee != nil {
			initialAssignee = issue.Fields.Assignee.AccountID
		}
	}

	// wasAssignedAt returns true if accountID was the assignee at time t.
	wasAssignedAt := func(t time.Time) bool {
		current := initialAssignee
		for _, change := range timeline {
			if !change.at.After(t) {
				current = change.toID
			} else {
				break
			}
		}
		return current == accountID
	}

	for _, h := range histories {
		created, err := parseJiraTime(h.Created)
		if err != nil || created.Before(since) {
			continue
		}
		author := h.Author.DisplayName

		for _, item := range h.Items {
			switch item.Field {

			case "assignee":
				if item.From == accountID || item.To == accountID {
					from := item.FromString
					if from == "" {
						from = "Unassigned"
					}
					to := item.ToString
					if to == "" {
						to = "Unassigned"
					}
					events = append(events, Event{
						Time: created,
						Text: fmt.Sprintf("[%s] %s changed assignee from '%s' to '%s'",
							fmtTime(created), author, from, to),
					})
				}

			case "status":
				if wasAssignedAt(created) {
					events = append(events, Event{
						Time: created,
						Text: fmt.Sprintf("[%s] %s changed status from '%s' to '%s'",
							fmtTime(created), author, item.FromString, item.ToString),
					})
				}
			}
		}
	}

	// Comments
	for _, c := range issue.Fields.Comment.Comments {
		created, err := parseJiraTime(c.Created)
		if err != nil || created.Before(since) {
			continue
		}
		if wasAssignedAt(created) {
			events = append(events, Event{
				Time: created,
				Text: fmt.Sprintf("[%s] %s commented", fmtTime(created), c.Author.DisplayName),
			})
		}
	}

	sort.Slice(events, func(i, j int) bool {
		return events[i].Time.Before(events[j].Time)
	})
	return events, nil
}

// =========================================================
// ARGUMENT PARSING
// =========================================================

var relativeRe = regexp.MustCompile(`(?i)^(\d+)(d|h|m)$`)

// parseSinceArg parses a --since value into a UTC time.
// Accepted formats:
//
//	Relative : 1d, 2h
//	Time only: 9am, 9:30am, 14:30  (today, local time)
//	Full date : 2026-05-30 14:00    (local time assumed)
//	With tz   : 2026-05-30 14:00+06:00
var weekdays = []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}

func parseSinceArg(value string) (time.Time, error) {
	value = strings.TrimSpace(value)

	// Natural language: yesterday, monday–sunday
	lower := strings.ToLower(value)
	if lower == "yesterday" {
		now := time.Now()
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, -1).UTC(), nil
	}
	for target, name := range weekdays {
		if lower == name {
			now := time.Now()
			today := int(now.Weekday()+6) % 7 // convert Sunday=0 to Monday=0
			daysBack := (today - target + 7) % 7
			if daysBack == 0 {
				daysBack = 7 // last occurrence, not today
			}
			return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).AddDate(0, 0, -daysBack).UTC(), nil
		}
	}

	// Relative: 1d / 2h / 30m (case-insensitive)
	if m := relativeRe.FindStringSubmatch(value); m != nil {
		n, _ := strconv.Atoi(m[1])
		if n == 0 {
			return time.Time{}, fmt.Errorf("'%s' is not a valid duration. Use a value greater than 0", value)
		}
		switch strings.ToLower(m[2]) {
		case "d":
			return time.Now().UTC().Add(-time.Duration(n) * 24 * time.Hour), nil
		case "h":
			return time.Now().UTC().Add(-time.Duration(n) * time.Hour), nil
		case "m":
			return time.Now().UTC().Add(-time.Duration(n) * time.Minute), nil
		}
	}

	// Time only: 9am, 9:30am, 14:30
	upper := strings.ToUpper(value)
	for _, fmt := range []string{"3PM", "3:04PM", "15:04"} {
		if t, err := time.ParseInLocation(fmt, upper, time.Local); err == nil {
			now := time.Now()
			result := time.Date(now.Year(), now.Month(), now.Day(),
				t.Hour(), t.Minute(), 0, 0, time.Local)
			if result.After(now) {
				result = result.AddDate(0, 0, -1)
			}
			return result.UTC(), nil
		}
	}

	// Full datetime with explicit timezone offset
	for _, fmt := range []string{
		"2006-01-02 15:04-07:00",
		"2006-01-02T15:04-07:00",
		"2006-01-02 15:04Z07:00",
		time.RFC3339,
	} {
		if t, err := time.Parse(fmt, value); err == nil {
			return t.UTC(), nil
		}
	}

	// Full datetime without timezone — assume local
	for _, fmt := range []string{"2006-01-02 15:04", "2006-01-02T15:04"} {
		if t, err := time.ParseInLocation(fmt, value, time.Local); err == nil {
			return t.UTC(), nil
		}
	}

	return time.Time{}, fmt.Errorf(
		"unrecognized time format: %q\nAccepted: yesterday, monday–sunday, 9am, 14:30, 30m, 2h, 1d, \"2026-05-30 14:00\", \"2026-05-30 14:00+06:00\"",
		value,
	)
}

// =========================================================
// MAIN
// =========================================================

func main() {
	sinceFlag := flag.String("since", "", `Override start time. Accepted: 9am, 14:30, 2h, 1d, "2026-05-30 14:00", "2026-05-30 14:00+06:00"`)
	projectFlag := flag.String("project", "", "Comma-separated project keys to filter results (e.g. PROJ or PROJ,OTHER)")
	unassignedQA := flag.Bool("unassigned-qa", false, "Show tickets where you moved a status from a QA column to another")
	showLog := flag.Bool("log", false, "Show run history (last 20 entries)")
	logN := flag.Int("log-n", -1, "Show last N entries of run history (0 = all)")
	dryRun := flag.Bool("dry-run", false, "Run normally but do not update state or history")
	reset := flag.Bool("reset", false, "Delete the state file and exit")
	output := flag.String("output", "", `Output format: "json" for machine-readable output`)
	showVersion := flag.Bool("version", false, "Print version and exit")
	initFlag := flag.Bool("init", false, "Interactive setup: create ~/.jira_update/.env")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	if *initFlag {
		runInit()
		return
	}

	if *reset {
		err := os.Remove(statePath())
		if err != nil && !os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "Error removing state file:", err)
			os.Exit(1)
		}
		if os.IsNotExist(err) {
			fmt.Println("No state file found.")
		} else {
			fmt.Println("State file removed.")
		}
		return
	}

	if *showLog || *logN >= 0 {
		limit := 20
		if *logN >= 0 {
			limit = *logN
		}
		printHistory(limit)
		return
	}

	loadEnv()
	validateConfig()

	accountID, err := fetchMyAccountID()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error fetching account ID:", err)
		os.Exit(1)
	}

	var since time.Time
	var sinceType, sinceValue string
	if *sinceFlag != "" {
		var err error
		since, err = parseSinceArg(*sinceFlag)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		if since.After(time.Now().UTC()) {
			fmt.Fprintln(os.Stderr, "Error: --since time cannot be in the future.")
			os.Exit(1)
		}
		sinceType, sinceValue = "arg", *sinceFlag
	} else {
		since = loadLastRun()
		sinceType, sinceValue = "state", since.Format(time.RFC3339)
	}

	if *output != "json" {
		fmt.Println(strings.Repeat("=", 80))
		fmt.Printf("JIRA activity since %s\n", since.Format(time.RFC3339))
		fmt.Println(strings.Repeat("=", 80))
		fmt.Println()
	}

	var projects []string
	if *projectFlag != "" {
		for _, k := range strings.Split(*projectFlag, ",") {
			if k := strings.TrimSpace(strings.ToUpper(k)); k != "" {
				projects = append(projects, k)
			}
		}
	}

	issues, err := fetchUpdatedIssues(since, projects, *unassignedQA)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error fetching issues:", err)
		os.Exit(1)
	}

	type result struct {
		issue  Issue
		events []Event
		err    error
	}

	results := make([]result, len(issues))
	var wg sync.WaitGroup
	for i, issue := range issues {
		wg.Add(1)
		go func(i int, issue Issue) {
			defer wg.Done()
			events, err := extractRelevantActivity(issue, since, accountID, *unassignedQA)
			results[i] = result{issue: issue, events: events, err: err}
		}(i, issue)
	}
	wg.Wait()

	type issueOutput struct {
		Key     string   `json:"key"`
		Summary string   `json:"summary"`
		Events  []string `json:"events"`
	}

	var hasError bool
	var issueActivity []issueOutput
	for _, r := range results {
		if r.err != nil {
			fmt.Fprintf(os.Stderr, "Warning: skipping %s: %v\n", r.issue.Key, r.err)
			hasError = true
			continue
		}
		if len(r.events) == 0 {
			continue
		}
		texts := make([]string, len(r.events))
		for i, e := range r.events {
			texts[i] = e.Text
		}
		issueActivity = append(issueActivity, issueOutput{
			Key:     r.issue.Key,
			Summary: r.issue.Fields.Summary,
			Events:  texts,
		})
	}

	if *output == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if issueActivity == nil {
			issueActivity = []issueOutput{}
		}
		enc.Encode(issueActivity) //nolint
	} else {
		if len(issueActivity) == 0 {
			fmt.Println("No relevant activity found.")
		} else {
			for _, iss := range issueActivity {
				fmt.Printf("%s - %s\n", iss.Key, iss.Summary)
				fmt.Println(strings.Repeat("-", 80))
				for _, e := range iss.Events {
					fmt.Printf("  • %s\n", e)
				}
				fmt.Println()
			}
		}
	}

	if hasError {
		fmt.Fprintln(os.Stderr, "Warning: some issues could not be processed. State not updated to avoid missing activity on next run.")
	} else if *dryRun {
		fmt.Fprintln(os.Stderr, "[dry-run] state and history not updated")
	} else {
		saveLastRun(time.Now().UTC())
		appendHistory("go", sinceType, sinceValue)
	}
}
