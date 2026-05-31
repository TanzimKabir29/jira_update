package main

import (
	"strings"
	"testing"
	"time"
)

func approxEqual(t *testing.T, result, expected time.Time, deltaSeconds float64) {
	t.Helper()
	diff := result.Sub(expected).Seconds()
	if diff < 0 {
		diff = -diff
	}
	if diff > deltaSeconds {
		t.Errorf("got %v, want within %.0fs of %v (diff: %.2fs)", result, deltaSeconds, expected, diff)
	}
}

func localTodayOrYesterday(hour, minute int) time.Time {
	now := time.Now()
	result := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.Local)
	if result.After(now) {
		result = result.AddDate(0, 0, -1)
	}
	return result.UTC()
}

// ----------------------------------------------------------
// Relative — case insensitive
// ----------------------------------------------------------

func TestRelativeDaysLowercase(t *testing.T) {
	result, err := parseSinceArg("2d")
	if err != nil {
		t.Fatal(err)
	}
	approxEqual(t, result, time.Now().UTC().Add(-2*24*time.Hour), 2)
}

func TestRelativeDaysUppercase(t *testing.T) {
	result, err := parseSinceArg("2D")
	if err != nil {
		t.Fatal(err)
	}
	approxEqual(t, result, time.Now().UTC().Add(-2*24*time.Hour), 2)
}

func TestRelativeHoursLowercase(t *testing.T) {
	result, err := parseSinceArg("6h")
	if err != nil {
		t.Fatal(err)
	}
	approxEqual(t, result, time.Now().UTC().Add(-6*time.Hour), 2)
}

func TestRelativeHoursUppercase(t *testing.T) {
	result, err := parseSinceArg("6H")
	if err != nil {
		t.Fatal(err)
	}
	approxEqual(t, result, time.Now().UTC().Add(-6*time.Hour), 2)
}

func TestRelativeMinutesLowercase(t *testing.T) {
	result, err := parseSinceArg("30m")
	if err != nil {
		t.Fatal(err)
	}
	approxEqual(t, result, time.Now().UTC().Add(-30*time.Minute), 2)
}

func TestRelativeMinutesUppercase(t *testing.T) {
	result, err := parseSinceArg("30M")
	if err != nil {
		t.Fatal(err)
	}
	approxEqual(t, result, time.Now().UTC().Add(-30*time.Minute), 2)
}

// ----------------------------------------------------------
// Zero values — must error
// ----------------------------------------------------------

func TestZeroDaysErrors(t *testing.T) {
	_, err := parseSinceArg("0d")
	if err == nil {
		t.Fatal("expected error for 0d")
	}
	if !strings.Contains(err.Error(), "not a valid duration") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestZeroHoursErrors(t *testing.T) {
	_, err := parseSinceArg("0H")
	if err == nil {
		t.Fatal("expected error for 0H")
	}
	if !strings.Contains(err.Error(), "not a valid duration") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestZeroMinutesErrors(t *testing.T) {
	_, err := parseSinceArg("0m")
	if err == nil {
		t.Fatal("expected error for 0m")
	}
	if !strings.Contains(err.Error(), "not a valid duration") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ----------------------------------------------------------
// Time only — today in local time
// ----------------------------------------------------------

func TestTimeHourAM(t *testing.T) {
	result, err := parseSinceArg("9am")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Equal(localTodayOrYesterday(9, 0)) {
		t.Errorf("got %v, want %v", result, localTodayOrYesterday(9, 0))
	}
}

func TestTimeHourMinuteAM(t *testing.T) {
	result, err := parseSinceArg("9:30am")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Equal(localTodayOrYesterday(9, 30)) {
		t.Errorf("got %v, want %v", result, localTodayOrYesterday(9, 30))
	}
}

func TestTime24h(t *testing.T) {
	result, err := parseSinceArg("14:30")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Equal(localTodayOrYesterday(14, 30)) {
		t.Errorf("got %v, want %v", result, localTodayOrYesterday(14, 30))
	}
}

func TestTimePM(t *testing.T) {
	result, err := parseSinceArg("3pm")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Equal(localTodayOrYesterday(15, 0)) {
		t.Errorf("got %v, want %v", result, localTodayOrYesterday(15, 0))
	}
}

// ----------------------------------------------------------
// Absolute datetime — no timezone (local assumed)
// ----------------------------------------------------------

func TestAbsoluteNoTZ(t *testing.T) {
	result, err := parseSinceArg("2026-05-30 14:00")
	if err != nil {
		t.Fatal(err)
	}
	expected := time.Date(2026, 5, 30, 14, 0, 0, 0, time.Local).UTC()
	if !result.Equal(expected) {
		t.Errorf("got %v, want %v", result, expected)
	}
}

// ----------------------------------------------------------
// Absolute datetime — explicit timezone
// ----------------------------------------------------------

func TestAbsolutePositiveTZ(t *testing.T) {
	result, err := parseSinceArg("2026-05-30 14:00+06:00")
	if err != nil {
		t.Fatal(err)
	}
	expected := time.Date(2026, 5, 30, 8, 0, 0, 0, time.UTC)
	if !result.Equal(expected) {
		t.Errorf("got %v, want %v", result, expected)
	}
}

func TestAbsoluteNegativeTZ(t *testing.T) {
	result, err := parseSinceArg("2026-05-30 14:00-05:00")
	if err != nil {
		t.Fatal(err)
	}
	expected := time.Date(2026, 5, 30, 19, 0, 0, 0, time.UTC)
	if !result.Equal(expected) {
		t.Errorf("got %v, want %v", result, expected)
	}
}

func TestAbsoluteUTC(t *testing.T) {
	result, err := parseSinceArg("2026-05-30 14:00+00:00")
	if err != nil {
		t.Fatal(err)
	}
	expected := time.Date(2026, 5, 30, 14, 0, 0, 0, time.UTC)
	if !result.Equal(expected) {
		t.Errorf("got %v, want %v", result, expected)
	}
}

// ----------------------------------------------------------
// Natural language: yesterday, weekday names
// ----------------------------------------------------------

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local).UTC()
}

func TestYesterday(t *testing.T) {
	result, err := parseSinceArg("yesterday")
	if err != nil {
		t.Fatal(err)
	}
	expected := startOfDay(time.Now().AddDate(0, 0, -1))
	if !result.Equal(expected) {
		t.Errorf("got %v, want %v", result, expected)
	}
}

func TestYesterdayUppercase(t *testing.T) {
	result, err := parseSinceArg("Yesterday")
	if err != nil {
		t.Fatal(err)
	}
	expected := startOfDay(time.Now().AddDate(0, 0, -1))
	if !result.Equal(expected) {
		t.Errorf("got %v, want %v", result, expected)
	}
}

func TestWeekdayReturnsLastOccurrence(t *testing.T) {
	// Any weekday name should return a date in the past (1–7 days ago, start of day)
	for _, day := range weekdays {
		result, err := parseSinceArg(day)
		if err != nil {
			t.Fatalf("parseSinceArg(%q) error: %v", day, err)
		}
		now := time.Now().UTC()
		if !result.Before(now) {
			t.Errorf("%s: got %v which is not before now (%v)", day, result, now)
		}
		daysAgo := int(now.Sub(result).Hours() / 24)
		if daysAgo < 1 || daysAgo > 7 {
			t.Errorf("%s: expected 1–7 days ago, got %d days ago", day, daysAgo)
		}
		// Verify time is midnight local
		local := result.In(time.Local)
		if local.Hour() != 0 || local.Minute() != 0 || local.Second() != 0 {
			t.Errorf("%s: expected midnight local, got %v", day, local)
		}
	}
}

// ----------------------------------------------------------
// Invalid input
// ----------------------------------------------------------

func TestInvalidFormat(t *testing.T) {
	_, err := parseSinceArg("notadate")
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	if !strings.Contains(err.Error(), "unrecognized time format") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestEmptyString(t *testing.T) {
	_, err := parseSinceArg("")
	if err == nil {
		t.Fatal("expected error for empty string")
	}
}
