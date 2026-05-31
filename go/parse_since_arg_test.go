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

func localToday(hour, minute int) time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.Local).UTC()
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
	if !result.Equal(localToday(9, 0)) {
		t.Errorf("got %v, want %v", result, localToday(9, 0))
	}
}

func TestTimeHourMinuteAM(t *testing.T) {
	result, err := parseSinceArg("9:30am")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Equal(localToday(9, 30)) {
		t.Errorf("got %v, want %v", result, localToday(9, 30))
	}
}

func TestTime24h(t *testing.T) {
	result, err := parseSinceArg("14:30")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Equal(localToday(14, 30)) {
		t.Errorf("got %v, want %v", result, localToday(14, 30))
	}
}

func TestTimePM(t *testing.T) {
	result, err := parseSinceArg("3pm")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Equal(localToday(15, 0)) {
		t.Errorf("got %v, want %v", result, localToday(15, 0))
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
