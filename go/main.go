package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// =========================================================
// CONFIG
// Config is loaded from ~/.jira_update/.env
// Copy .env.example from the repo root to that location.
// =========================================================

var (
	jiraBaseURL  string
	jiraEmail    string
	jiraAPIToken string
)

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
	defer f.Close()

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
		os.Setenv(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
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

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

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
	return me.AccountID, json.Unmarshal(body, &me)
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

func fetchUpdatedIssues(since time.Time) ([]Issue, error) {
	jql := fmt.Sprintf(`updated >= "%s" ORDER BY updated ASC`, since.Format("2006-01-02 15:04"))

	var issues []Issue
	nextPageToken := ""

	for {
		params := url.Values{
			"jql":        {jql},
			"fields":     {"summary,status,assignee,comment"},
			"maxResults": {"100"},
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

func fetchChangelog(issueKey string) ([]struct {
	Created string `json:"created"`
	Author  struct {
		DisplayName string `json:"displayName"`
	} `json:"author"`
	Items []struct {
		Field      string `json:"field"`
		From       string `json:"from"`
		FromString string `json:"fromString"`
		To         string `json:"to"`
		ToString   string `json:"toString"`
	} `json:"items"`
}, error) {
	body, err := jiraGet("/rest/api/3/issue/"+issueKey+"/changelog", nil)
	if err != nil {
		return nil, err
	}
	var result struct {
		Values []struct {
			Created string `json:"created"`
			Author  struct {
				DisplayName string `json:"displayName"`
			} `json:"author"`
			Items []struct {
				Field      string `json:"field"`
				From       string `json:"from"`
				FromString string `json:"fromString"`
				To         string `json:"to"`
				ToString   string `json:"toString"`
			} `json:"items"`
		} `json:"values"`
	}
	return result.Values, json.Unmarshal(body, &result)
}

// =========================================================
// HELPERS
// =========================================================

func parseJiraTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, strings.Replace(s, "Z", "+00:00", 1))
}

func fmtTime(t time.Time) string {
	return t.UTC().Format("2006-01-02 15:04")
}

// =========================================================
// ACTIVITY EXTRACTION
// =========================================================

type Event struct {
	Time time.Time
	Text string
}

func extractRelevantActivity(issue Issue, since time.Time, accountID string) ([]Event, error) {
	var events []Event

	assignedToMe := issue.Fields.Assignee != nil &&
		issue.Fields.Assignee.AccountID == accountID

	histories, err := fetchChangelog(issue.Key)
	if err != nil {
		return nil, err
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
				if assignedToMe {
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
	if assignedToMe {
		for _, c := range issue.Fields.Comment.Comments {
			created, err := parseJiraTime(c.Created)
			if err != nil || created.Before(since) {
				continue
			}
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
// MAIN
// =========================================================

func main() {
	loadEnv()
	validateConfig()

	accountID, err := fetchMyAccountID()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error fetching account ID:", err)
		os.Exit(1)
	}

	since := loadLastRun()

	fmt.Println(strings.Repeat("=", 80))
	fmt.Printf("JIRA activity since %s\n", since.Format(time.RFC3339))
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println()

	issues, err := fetchUpdatedIssues(since)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error fetching issues:", err)
		os.Exit(1)
	}

	var hasActivity bool
	for _, issue := range issues {
		events, err := extractRelevantActivity(issue, since, accountID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: skipping %s: %v\n", issue.Key, err)
			continue
		}
		if len(events) == 0 {
			continue
		}
		hasActivity = true
		fmt.Printf("%s - %s\n", issue.Key, issue.Fields.Summary)
		fmt.Println(strings.Repeat("-", 80))
		for _, e := range events {
			fmt.Printf("  • %s\n", e.Text)
		}
		fmt.Println()
	}

	if !hasActivity {
		fmt.Println("No relevant activity found.")
	}

	saveLastRun(time.Now().UTC())
}
