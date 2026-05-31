import unittest
from datetime import datetime, timedelta, timezone
from jira_proc import parse_since_arg


class TestParseSinceArg(unittest.TestCase):

    def _approx(self, result, expected, delta_seconds=2):
        diff = abs((result - expected).total_seconds())
        self.assertLessEqual(diff, delta_seconds,
            f"{result} not within {delta_seconds}s of {expected}")

    def _local_today_or_yesterday(self, hour, minute):
        local_tz = datetime.now().astimezone().tzinfo
        now = datetime.now(local_tz)
        result = now.replace(hour=hour, minute=minute, second=0, microsecond=0)
        if result > now:
            result -= timedelta(days=1)
        return result.astimezone(timezone.utc)

    # ----------------------------------------------------------
    # Relative — case insensitive
    # ----------------------------------------------------------

    def test_days_lowercase(self):
        self._approx(parse_since_arg("2d"), datetime.now(timezone.utc) - timedelta(days=2))

    def test_days_uppercase(self):
        self._approx(parse_since_arg("2D"), datetime.now(timezone.utc) - timedelta(days=2))

    def test_hours_lowercase(self):
        self._approx(parse_since_arg("6h"), datetime.now(timezone.utc) - timedelta(hours=6))

    def test_hours_uppercase(self):
        self._approx(parse_since_arg("6H"), datetime.now(timezone.utc) - timedelta(hours=6))

    def test_minutes_lowercase(self):
        self._approx(parse_since_arg("30m"), datetime.now(timezone.utc) - timedelta(minutes=30))

    def test_minutes_uppercase(self):
        self._approx(parse_since_arg("30M"), datetime.now(timezone.utc) - timedelta(minutes=30))

    # ----------------------------------------------------------
    # Zero values — must error
    # ----------------------------------------------------------

    def test_zero_days_errors(self):
        with self.assertRaisesRegex(ValueError, "not a valid duration"):
            parse_since_arg("0d")

    def test_zero_hours_errors(self):
        with self.assertRaisesRegex(ValueError, "not a valid duration"):
            parse_since_arg("0H")

    def test_zero_minutes_errors(self):
        with self.assertRaisesRegex(ValueError, "not a valid duration"):
            parse_since_arg("0m")

    # ----------------------------------------------------------
    # Time only — today in local time
    # ----------------------------------------------------------

    def test_time_hour_am(self):
        self.assertEqual(parse_since_arg("9am"), self._local_today_or_yesterday(9, 0))

    def test_time_hour_minute_am(self):
        self.assertEqual(parse_since_arg("9:30am"), self._local_today_or_yesterday(9, 30))

    def test_time_24h(self):
        self.assertEqual(parse_since_arg("14:30"), self._local_today_or_yesterday(14, 30))

    def test_time_pm(self):
        self.assertEqual(parse_since_arg("3pm"), self._local_today_or_yesterday(15, 0))

    # ----------------------------------------------------------
    # Absolute datetime — no timezone (local assumed)
    # ----------------------------------------------------------

    def test_absolute_no_tz(self):
        local_tz = datetime.now().astimezone().tzinfo
        expected = datetime(2026, 5, 30, 14, 0, tzinfo=local_tz).astimezone(timezone.utc)
        self.assertEqual(parse_since_arg("2026-05-30 14:00"), expected)

    # ----------------------------------------------------------
    # Absolute datetime — explicit timezone
    # ----------------------------------------------------------

    def test_absolute_positive_tz(self):
        expected = datetime(2026, 5, 30, 8, 0, tzinfo=timezone.utc)
        self.assertEqual(parse_since_arg("2026-05-30 14:00+06:00"), expected)

    def test_absolute_negative_tz(self):
        expected = datetime(2026, 5, 30, 19, 0, tzinfo=timezone.utc)
        self.assertEqual(parse_since_arg("2026-05-30 14:00-05:00"), expected)

    def test_absolute_utc(self):
        expected = datetime(2026, 5, 30, 14, 0, tzinfo=timezone.utc)
        self.assertEqual(parse_since_arg("2026-05-30 14:00+00:00"), expected)

    # ----------------------------------------------------------
    # Natural language: yesterday, weekday names
    # ----------------------------------------------------------

    def _start_of_day(self, dt):
        local_tz = datetime.now().astimezone().tzinfo
        return dt.astimezone(local_tz).replace(hour=0, minute=0, second=0, microsecond=0).astimezone(timezone.utc)

    def test_yesterday(self):
        result = parse_since_arg("yesterday")
        expected = self._start_of_day(datetime.now(timezone.utc) - timedelta(days=1))
        self.assertEqual(result, expected)

    def test_yesterday_uppercase(self):
        result = parse_since_arg("Yesterday")
        expected = self._start_of_day(datetime.now(timezone.utc) - timedelta(days=1))
        self.assertEqual(result, expected)

    def test_weekday_returns_last_occurrence(self):
        weekdays = ["monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"]
        now = datetime.now(timezone.utc)
        for day in weekdays:
            result = parse_since_arg(day)
            self.assertLess(result, now, f"{day}: result {result} should be in the past")
            days_ago = (now - result).days
            self.assertGreaterEqual(days_ago, 1, f"{day}: should be at least 1 day ago")
            self.assertLessEqual(days_ago, 7, f"{day}: should be at most 7 days ago")
            # Verify time is midnight local
            local_tz = datetime.now().astimezone().tzinfo
            local_result = result.astimezone(local_tz)
            self.assertEqual(local_result.hour, 0)
            self.assertEqual(local_result.minute, 0)
            self.assertEqual(local_result.second, 0)

    # ----------------------------------------------------------
    # Invalid input
    # ----------------------------------------------------------

    def test_invalid_format(self):
        with self.assertRaisesRegex(ValueError, "Unrecognized time format"):
            parse_since_arg("notadate")

    def test_empty_string(self):
        with self.assertRaisesRegex(ValueError, "Unrecognized time format"):
            parse_since_arg("")


if __name__ == "__main__":
    unittest.main(verbosity=2)
